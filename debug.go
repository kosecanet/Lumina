package main

// Depuración (solo administradores): registro reciente del servidor, conversiones en marcha,
// errores que envían los reproductores y análisis de un vídeo con ffprobe.

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ---------- registro en memoria ----------

const logKeep = 1500

type logLine struct {
	T   int64  `json:"t"` // unix ms
	Msg string `json:"m"`
}

type ringLog struct {
	mu    sync.Mutex
	lines []logLine
	next  int
	full  bool
	part  []byte
}

var logs = &ringLog{lines: make([]logLine, logKeep)}

// credenciales que nunca deben quedar en el registro: rutas Xtream /live|series|movie/usuario/clave/ y parámetros de URL
var redactRe = []struct {
	re  *regexp.Regexp
	rep string
}{
	{regexp.MustCompile(`/(live|series|movie|timeshift)/[^/\s]+/[^/\s]+/`), "/$1/***/***/"},
	{regexp.MustCompile(`(?i)\b(password|pass|username|api_key|token)=[^&\s"]+`), "$1=***"},
}

func redact(s string) string {
	for _, r := range redactRe {
		s = r.re.ReplaceAllString(s, r.rep)
	}
	return s
}

// Write recibe la salida de log (una o varias líneas terminadas en \n).
func (l *ringLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.part = append(l.part, p...)
	for {
		i := bytes.IndexByte(l.part, '\n')
		if i < 0 {
			break
		}
		line := string(l.part[:i])
		l.part = l.part[i+1:]
		// log ya antepone fecha y hora; se quita para no duplicarla en la vista
		if len(line) > 20 && line[4] == '/' && line[13] == ':' {
			line = line[20:]
		}
		l.lines[l.next] = logLine{T: time.Now().UnixMilli(), Msg: redact(line)}
		l.next = (l.next + 1) % logKeep
		if l.next == 0 {
			l.full = true
		}
	}
	if len(l.part) > 8192 { // línea anómala sin salto
		l.part = l.part[:0]
	}
	return len(p), nil
}

// tail devuelve las últimas n líneas en orden cronológico.
func (l *ringLog) tail(n int) []logLine {
	l.mu.Lock()
	defer l.mu.Unlock()
	cnt := l.next
	if l.full {
		cnt = logKeep
	}
	if n <= 0 || n > cnt {
		n = cnt
	}
	out := make([]logLine, 0, n)
	for i := cnt - n; i < cnt; i++ {
		idx := i
		if l.full {
			idx = (l.next + i) % logKeep
		}
		out = append(out, l.lines[idx])
	}
	return out
}

func installLog() {
	log.SetOutput(io.MultiWriter(os.Stderr, logs))
}

// ---------- conversiones en marcha ----------

type convInfo struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Mode  string `json:"mode"`
	User  string `json:"user"`
	Since int64  `json:"since"`
}

var (
	convMu  sync.Mutex
	convs   = map[int64]convInfo{}
	convSeq atomic.Int64
)

func convStart(c convInfo) func() {
	id := convSeq.Add(1)
	c.Since = time.Now().Unix()
	convMu.Lock()
	convs[id] = c
	convMu.Unlock()
	return func() {
		convMu.Lock()
		delete(convs, id)
		convMu.Unlock()
	}
}

func convList() []convInfo {
	convMu.Lock()
	defer convMu.Unlock()
	out := make([]convInfo, 0, len(convs))
	for _, c := range convs {
		out = append(out, c)
	}
	return out
}

// ---------- API ----------

var ffVer struct {
	once sync.Once
	v    string
}

func (a *App) ffmpegVersion() string {
	ffVer.once.Do(func() {
		if a.ffmpeg == "" {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, a.ffmpeg, "-hide_banner", "-version").Output()
		if err == nil {
			ffVer.v = strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
		}
	})
	return ffVer.v
}

// GET /api/system/debug?n=400
func (a *App) hDebug(w http.ResponseWriter, r *http.Request) {
	n, _ := strconv.Atoi(r.URL.Query().Get("n"))
	if n <= 0 || n > logKeep {
		n = 400
	}
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	host, _ := os.Hostname()
	writeJSON(w, r, map[string]any{
		"version": version, "go": runtime.Version(), "os": runtime.GOOS + "/" + runtime.GOARCH, "cpus": runtime.NumCPU(), "host": host,
		"uptime": int64(time.Since(a.started).Seconds()), "dir": a.dir, "exe": startExe,
		"ffmpeg": a.ffmpeg, "ffprobe": a.ffprobe, "ffver": a.ffmpegVersion(),
		"heapMB": float64(ms.HeapAlloc) / 1048576, "sysMB": float64(ms.Sys) / 1048576, "goroutines": runtime.NumGoroutine(),
		"remux": len(remuxSem), "remuxMax": cap(remuxSem), "x264": len(x264Sem), "x264Max": cap(x264Sem),
		"convs": convList(), "live": a.hub.status(), "logs": logs.tail(n),
	})
}

// GET /api/system/debug/log: registro completo como texto, para descargarlo o compartirlo.
func (a *App) hDebugLog(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="lumina-registro.txt"`)
	var b strings.Builder
	for _, l := range logs.tail(0) {
		b.WriteString(time.UnixMilli(l.T).Format("2006-01-02 15:04:05 "))
		b.WriteString(l.Msg)
		b.WriteByte('\n')
	}
	_, _ = io.WriteString(w, b.String())
}

// GET /api/system/probe/{id}: pistas de un vídeo según ffprobe (códecs, perfil, nivel…).
func (a *App) hProbe(w http.ResponseWriter, r *http.Request) {
	it := a.item(r.PathValue("id"))
	if it == nil {
		jsonErr(w, 404, "no encontrado")
		return
	}
	res := map[string]any{"title": it.Title, "series": it.Series, "path": it.Path, "plan": plan(it, a.ffmpeg != ""), "vc": it.VC, "ac": it.AC, "dur": it.Dur}
	if isCloud(it) {
		res["path"] = "nube:" + strings.TrimPrefix(it.Path, "cloud:")
	}
	if a.ffprobe == "" {
		res["error"] = "ffprobe no está instalado"
		writeJSON(w, r, res)
		return
	}
	in := it.Path
	var pre []string
	if isCloud(it) {
		// ocupa una conexión del proveedor unos segundos: se cuenta como una reproducción más
		u, c, err := a.cloudURL(it.Path)
		if err != nil {
			jsonErr(w, 502, err.Error())
			return
		}
		ls, sub, busy := a.hub.vodAcquire(cloudKey(it), "análisis «"+it.Series+"»", false)
		if busy != nil {
			res["error"] = "el proveedor no tiene conexiones libres ahora (hay otra reproducción en marcha)"
			writeJSON(w, r, res)
			return
		}
		defer a.hub.release(ls, sub)
		in, pre = u, []string{"-user_agent", c.ua}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	args := append([]string{"-v", "error", "-print_format", "json", "-show_format", "-show_streams"}, pre...)
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, a.ffprobe, append(args, in)...)
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		res["error"] = redact(strings.ReplaceAll(strings.TrimSpace(stderr.String()+" "+err.Error()), in, "<vídeo>"))
		writeJSON(w, r, res)
		return
	}
	var pr struct {
		Streams []map[string]any `json:"streams"`
		Format  map[string]any   `json:"format"`
	}
	_ = json.Unmarshal(out, &pr)
	keep := []string{"index", "codec_type", "codec_name", "profile", "level", "pix_fmt", "width", "height", "r_frame_rate", "channels", "channel_layout", "sample_rate", "bit_rate", "tags", "disposition"}
	streams := make([]map[string]any, 0, len(pr.Streams))
	for _, s := range pr.Streams {
		m := map[string]any{}
		for _, k := range keep {
			if v, ok := s[k]; ok {
				if k == "tags" { // solo idioma y título de la pista
					if t, ok := v.(map[string]any); ok {
						v = map[string]any{"language": t["language"], "title": t["title"]}
					}
				}
				if k == "disposition" {
					if d, ok := v.(map[string]any); ok {
						v = map[string]any{"default": d["default"], "attached_pic": d["attached_pic"]}
					}
				}
				m[k] = v
			}
		}
		streams = append(streams, m)
	}
	res["streams"] = streams
	if pr.Format != nil {
		res["format"] = map[string]any{"format_name": pr.Format["format_name"], "duration": pr.Format["duration"], "bit_rate": pr.Format["bit_rate"], "size": pr.Format["size"]}
	}
	writeJSON(w, r, res)
}

// POST /api/clientlog: los reproductores informan de errores y cambios de modo (aparecen en el registro).
var (
	clMu   sync.Mutex
	clSeen = map[string][]time.Time{}
)

func (a *App) hClientLog(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Msg string `json:"msg"`
	}
	if err := readJSON(r, &in); err != nil || strings.TrimSpace(in.Msg) == "" {
		w.WriteHeader(204)
		return
	}
	ip := clientIP(r)
	clMu.Lock()
	cut := time.Now().Add(-time.Minute)
	l := clSeen[ip][:0]
	for _, t := range clSeen[ip] {
		if t.After(cut) {
			l = append(l, t)
		}
	}
	ok := len(l) < 30
	if ok {
		l = append(l, time.Now())
	}
	clSeen[ip] = l
	if len(clSeen) > 500 { // no crecer sin límite con muchas IP distintas
		clSeen = map[string][]time.Time{}
	}
	clMu.Unlock()
	if !ok {
		w.WriteHeader(429)
		return
	}
	msg := strings.Map(func(c rune) rune {
		if c < 32 {
			return ' '
		}
		return c
	}, in.Msg)
	if len(msg) > 400 {
		msg = msg[:400] + "…"
	}
	who := "?"
	if u := userFrom(r); u != nil {
		who = u.Name
	}
	log.Printf("cliente %s [%s, %s]: %s", ip, who, clientKind(r.UserAgent()), msg)
	w.WriteHeader(204)
}

func clientKind(ua string) string {
	switch {
	case strings.Contains(ua, "Web0S") || strings.Contains(ua, "webOS"):
		return "LG webOS"
	case strings.Contains(ua, "LumTV"):
		return "Android TV"
	case strings.Contains(ua, "LumApp"):
		return "app Android"
	case strings.Contains(ua, "Android"):
		return "Android"
	case strings.Contains(ua, "iPhone") || strings.Contains(ua, "iPad"):
		return "iOS"
	case strings.Contains(ua, "Firefox"):
		return "Firefox"
	case strings.Contains(ua, "Edg/"):
		return "Edge"
	case strings.Contains(ua, "Chrome"):
		return "Chrome"
	case strings.Contains(ua, "Safari"):
		return "Safari"
	}
	return "navegador"
}
