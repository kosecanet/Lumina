package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"mime"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

// ---------- modelos persistentes ----------

type Library struct {
	ID   string   `json:"id"`
	Name string   `json:"name"`
	Type string   `json:"type"` // movies | series | cloud
	Path string   `json:"path"`
	Icon string   `json:"icon,omitempty"` // icono de la barra lateral (ver libIcons)
	Cats []string `json:"cats,omitempty"` // solo Type "cloud": categorías de series del Xtream
	Sers []int    `json:"sers,omitempty"` // solo Type "cloud": series sueltas
}

type XInfo struct {
	Status  string `json:"status"`
	Exp     int64  `json:"exp"`
	Active  int    `json:"active"`
	Max     int    `json:"max"`
	Checked int64  `json:"checked"`
}

type XCfg struct {
	URL     string `json:"url"`
	User    string `json:"user"`
	Pass    string `json:"pass"`
	MaxConn int    `json:"maxConn"` // 0 = el que indique el proveedor
	UA      string `json:"ua"`
	Info    XInfo  `json:"info"`
}

type Config struct {
	Libraries []Library `json:"libraries"`
	Xtream    XCfg      `json:"xtream"`
	Selected  []int     `json:"selected"` // ids de canal, en el orden elegido
	Favs      []int     `json:"favs"`
	Update    UpdCfg    `json:"update"`
	Profiles  []Profile `json:"profiles"`
	Users     []User    `json:"users,omitempty"`
	Secret    string    `json:"secret,omitempty"`
	TMDBKey   string    `json:"tmdb,omitempty"` // clave de TMDB para la información en castellano
}

type Item struct {
	ID         string  `json:"id"`
	Lib        string  `json:"lib"`
	Kind       string  `json:"kind"` // movie | episode
	Title      string  `json:"title"`
	Year       int     `json:"year,omitempty"`
	Series     string  `json:"series,omitempty"`
	SID        string  `json:"sid,omitempty"`
	Season     int     `json:"season,omitempty"`
	Episode    int     `json:"ep,omitempty"`
	Collection string  `json:"col,omitempty"`
	Path       string  `json:"path"`
	Size       int64   `json:"size"`
	Mod        int64   `json:"mod"`
	Added      int64   `json:"added"`
	Dur        float64 `json:"dur,omitempty"`
	VC         string  `json:"vc,omitempty"`
	AC         string  `json:"ac,omitempty"`
	Probed     bool    `json:"probed,omitempty"`
	PV         int     `json:"pv,omitempty"` // versión del análisis (para re-analizar al ampliar datos)
	Tracks     []Track `json:"tr,omitempty"`
	Art        string  `json:"art,omitempty"`  // imagen local propia
	SArt       string  `json:"sart,omitempty"` // imagen local de la serie
}

// Track: pista de audio (T="a") o subtítulos incrustados (T="s"); I es el índice dentro de su tipo.
type Track struct {
	T     string `json:"t"`
	I     int    `json:"i"`
	Lang  string `json:"l,omitempty"`
	Title string `json:"n,omitempty"`
	Codec string `json:"c,omitempty"`
	Ch    int    `json:"ch,omitempty"`
	Def   bool   `json:"d,omitempty"`
}

const probeVersion = 2

// repositorio de GitHub por defecto para las actualizaciones (editable en Ajustes → Sistema)
const officialRepo = "kosecanet/Lumina"

type Progress struct {
	Pos float64 `json:"p"`
	Dur float64 `json:"d"`
	At  int64   `json:"at"`
}

// ---------- aplicación ----------

type App struct {
	dir   string
	pass  string
	token string

	mu    sync.RWMutex
	cfg   Config
	items map[string]*Item
	progs map[string]map[string]Progress // progreso por perfil
	cat   Catalog
	chans map[int]*Channel
	epg   map[string][]Prog
	epgAt int64

	// derivados (se reconstruyen con rebuild)
	all      []*Item
	bySeries map[string][]*Item

	hub     *Hub
	upd     *Updater
	ffmpeg  string
	ffprobe string
	scan    ScanState
	aq      chan string

	tmu    sync.Mutex
	timers map[string]*time.Timer

	epgRun   sync.Mutex
	thumbMu  sync.Map
	thumbSem chan struct{}
	logoSem  chan struct{}
	logoNeg  sync.Map

	started time.Time
}

func NewApp(dir, pass string) (*App, error) {
	for _, d := range []string{"", "thumbs", "subs", "logos"} {
		if err := os.MkdirAll(filepath.Join(dir, d), 0o755); err != nil {
			return nil, err
		}
	}
	a := &App{
		dir: dir, pass: pass,
		items:    map[string]*Item{},
		chans:    map[int]*Channel{},
		epg:      map[string][]Prog{},
		bySeries: map[string][]*Item{},
		timers:   map[string]*time.Timer{},
		aq:       make(chan string, 1<<16),
		thumbSem: make(chan struct{}, 2),
		logoSem:  make(chan struct{}, 6),
		started:  time.Now(),
	}
	a.load("config.json", &a.cfg)
	var idx []*Item
	a.load("index.json", &idx)
	for _, it := range idx {
		if isCloud(it) { // índices guardados con versiones anteriores: sin etiqueta de idioma
			it.Series, it.Title = stripLang(it.Series), stripLang(it.Title)
		}
		a.items[it.ID] = it
	}
	a.initProfiles()
	a.load("xlive.json", &a.cat)
	var e epgFile
	a.load("epg.json", &e)
	if e.Progs != nil {
		a.epg = e.Progs
	}
	a.epgAt = e.At
	a.reindexChannels()
	a.rebuild()

	a.ffmpeg = findBin("ffmpeg")
	a.ffprobe = findBin("ffprobe")
	if pass != "" {
		m := hmac.New(sha256.New, []byte(pass))
		m.Write([]byte("lumina-token"))
		a.token = hex.EncodeToString(m.Sum(nil))
	}
	a.hub = newHub(a)
	a.upd = newUpdater(a)
	go a.analysisLoop()
	go a.background()
	// re-analizar lo que quedó pendiente
	a.mu.RLock()
	for id, it := range a.items {
		if !it.Probed || it.PV < probeVersion {
			a.enqueue(id)
		}
	}
	a.mu.RUnlock()
	return a, nil
}

func findBin(name string) string {
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if exe, err := os.Executable(); err == nil {
		d := filepath.Dir(exe)
		for _, p := range []string{filepath.Join(d, name), filepath.Join(d, "bin", name)} {
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				return p
			}
		}
	}
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	return ""
}

// ---------- persistencia ----------

func (a *App) load(name string, v any) {
	if b, err := os.ReadFile(filepath.Join(a.dir, name)); err == nil {
		_ = json.Unmarshal(b, v)
	}
}

func (a *App) writeFile(name string, b []byte) {
	p := filepath.Join(a.dir, name)
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err == nil {
		_ = os.Rename(tmp, p)
	}
}

func (a *App) saveConfig() {
	a.mu.RLock()
	b, _ := json.Marshal(a.cfg)
	a.mu.RUnlock()
	a.writeFile("config.json", b)
}

// saveConfigLocked guarda la configuración; el llamador ya tiene a.mu.
func (a *App) saveConfigLocked() {
	b, _ := json.Marshal(a.cfg)
	a.writeFile("config.json", b)
}

func (a *App) saveIndex() {
	a.mu.RLock()
	list := make([]*Item, 0, len(a.items))
	for _, it := range a.items {
		list = append(list, it)
	}
	b, _ := json.Marshal(list)
	a.mu.RUnlock()
	a.writeFile("index.json", b)
}

func (a *App) saveProgress() {
	a.mu.RLock()
	b, _ := json.Marshal(a.progs)
	a.mu.RUnlock()
	a.writeFile("progress2.json", b)
}

// later ejecuta f una sola vez tras d; nuevas llamadas con la misma clave reinician el temporizador.
func (a *App) later(key string, d time.Duration, f func()) {
	a.tmu.Lock()
	defer a.tmu.Unlock()
	if t := a.timers[key]; t != nil {
		t.Stop()
	}
	a.timers[key] = time.AfterFunc(d, f)
}

func (a *App) background() {
	time.Sleep(4 * time.Second)
	for {
		a.mu.RLock()
		configured := a.cfg.Xtream.URL != ""
		catOld := time.Now().Unix()-a.cat.Updated > 24*3600
		epgOld := time.Now().Unix()-a.epgAt > 6*3600
		a.mu.RUnlock()
		if configured {
			if catOld {
				_ = a.refreshCatalog()
			}
			if epgOld {
				_ = a.refreshEPG()
			}
		}
		time.Sleep(30 * time.Minute)
	}
}

// ---------- índices derivados ----------

func shortHash(s string) string {
	h := sha1.Sum([]byte(s))
	return hex.EncodeToString(h[:])[:10]
}

func (a *App) rebuild() {
	a.mu.Lock()
	defer a.mu.Unlock()
	all := make([]*Item, 0, len(a.items))
	bs := map[string][]*Item{}
	for _, it := range a.items {
		all = append(all, it)
		if it.Kind == "episode" {
			bs[it.SID] = append(bs[it.SID], it)
		}
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].Added != all[j].Added {
			return all[i].Added > all[j].Added
		}
		return strings.ToLower(all[i].Title) < strings.ToLower(all[j].Title)
	})
	for _, eps := range bs {
		sort.Slice(eps, func(i, j int) bool {
			if eps[i].Season != eps[j].Season {
				return eps[i].Season < eps[j].Season
			}
			if eps[i].Episode != eps[j].Episode {
				return eps[i].Episode < eps[j].Episode
			}
			return eps[i].Title < eps[j].Title
		})
	}
	a.all, a.bySeries = all, bs
}

func (a *App) item(id string) *Item {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.items[id]
}

func (a *App) lib(id string) (Library, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	for _, l := range a.cfg.Libraries {
		if l.ID == id {
			return l, true
		}
	}
	return Library{}, false
}

func (a *App) maxConn() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.cfg.Xtream.MaxConn > 0 {
		return a.cfg.Xtream.MaxConn
	}
	if a.cfg.Xtream.Info.Max > 0 {
		return a.cfg.Xtream.Info.Max
	}
	return 1
}

// ---------- progreso ----------

func (a *App) setProgress(pid, id string, pos, dur float64) {
	a.mu.Lock()
	if a.progs[pid] == nil {
		a.progs[pid] = map[string]Progress{}
	}
	prog := a.progs[pid]
	switch {
	case pos <= 0:
		delete(prog, id)
	case dur > 0 && pos/dur >= 0.95:
		prog[id] = Progress{Pos: dur, Dur: dur, At: time.Now().Unix()}
	default:
		prog[id] = Progress{Pos: pos, Dur: dur, At: time.Now().Unix()}
	}
	a.mu.Unlock()
	a.later("progress", 3*time.Second, a.saveProgress)
}

func finished(p Progress) bool { return p.Dur > 0 && p.Pos/p.Dur >= 0.95 }

// ---------- autenticación opcional ----------

// sameOrigin rechaza peticiones que modifican datos desde otra web (CSRF): una página de internet
// no debe poder añadir bibliotecas ni cambiar ajustes en tu servidor de casa.
func sameOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" && r.Method != "HEAD" && r.Method != "OPTIONS" {
			if o := r.Header.Get("Origin"); o != "" {
				if u, err := url.Parse(o); err != nil || u.Host != r.Host {
					jsonErr(w, 403, "origen no permitido")
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (a *App) auth(next http.Handler) http.Handler {
	next = sameOrigin(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		protected := strings.HasPrefix(p, "/api/") || strings.HasPrefix(p, "/stream/") ||
			strings.HasPrefix(p, "/remux/") || strings.HasPrefix(p, "/live/") || strings.HasPrefix(p, "/sub/")
		public := !protected || p == "/api/login" || p == "/api/auth" || p == "/api/logout"
		if a.accounts() {
			if public {
				next.ServeHTTP(w, r)
				return
			}
			u := a.sessionUser(r)
			if u == nil {
				jsonErr(w, 401, "no autorizado")
				return
			}
			if !a.allowed(u, r) {
				jsonErr(w, 403, "no tienes permiso para esto")
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, u)))
			return
		}
		if a.pass == "" || public || a.authed(r) {
			next.ServeHTTP(w, r)
			return
		}
		jsonErr(w, 401, "no autorizado")
	})
}

func (a *App) authed(r *http.Request) bool {
	if a.pass == "" {
		return true
	}
	t := ""
	if c, err := r.Cookie("lum"); err == nil {
		t = c.Value
	} else if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		t = strings.TrimPrefix(h, "Bearer ")
	}
	return subtle.ConstantTimeCompare([]byte(t), []byte(a.token)) == 1
}

// ---------- JSON y estáticos ----------

func writeJSON(w http.ResponseWriter, r *http.Request, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if len(b) > 1024 && strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Add("Vary", "Accept-Encoding")
		zw := gzip.NewWriter(w)
		_, _ = zw.Write(b)
		_ = zw.Close()
		return
	}
	_, _ = w.Write(b)
}

func jsonErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	b, _ := json.Marshal(map[string]any{"error": msg})
	_, _ = w.Write(b)
}

func readJSON(r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 4<<20)
	return json.NewDecoder(r.Body).Decode(v)
}

type asset struct {
	raw, gz []byte
	etag    string
	ctype   string
}

func (a *App) static() http.Handler {
	assets := map[string]*asset{}
	sub, _ := fs.Sub(webFS, "web")
	_ = fs.WalkDir(sub, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		b, _ := fs.ReadFile(sub, p)
		as := &asset{raw: b}
		h := sha1.Sum(b)
		as.etag = `"` + hex.EncodeToString(h[:8]) + `"`
		as.ctype = mime.TypeByExtension(filepath.Ext(p))
		switch filepath.Ext(p) {
		case ".js":
			as.ctype = "text/javascript; charset=utf-8"
		case ".css":
			as.ctype = "text/css; charset=utf-8"
		case ".html":
			as.ctype = "text/html; charset=utf-8"
		}
		if len(b) > 1024 {
			var buf bytes.Buffer
			zw, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
			_, _ = zw.Write(b)
			_ = zw.Close()
			as.gz = buf.Bytes()
		}
		assets["/"+p] = as
		return nil
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if p == "/" || !strings.Contains(p[1:], ".") { // rutas de la SPA
			p = "/index.html"
		}
		as := assets[p]
		if legacyUA(r) {
			if l := assets["/legacy"+p]; l != nil {
				as = l
			}
		}
		if as == nil || strings.HasPrefix(p, "/legacy/") && !legacyUA(r) && r.URL.Query().Get("legacy") == "" {
			http.NotFound(w, r)
			return
		}
		w.Header().Add("Vary", "User-Agent")
		w.Header().Set("ETag", as.etag)
		w.Header().Set("Content-Type", as.ctype)
		w.Header().Set("Cache-Control", "no-cache")
		if r.Header.Get("If-None-Match") == as.etag {
			w.WriteHeader(304)
			return
		}
		body := as.raw
		if as.gz != nil && strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			w.Header().Set("Content-Encoding", "gzip")
			w.Header().Add("Vary", "Accept-Encoding")
			body = as.gz
		}
		_, _ = w.Write(body)
	})
}

// legacyUA: LG webOS y Chromium < 90 (53-89) reciben la versión rebajada de la web (web/legacy).
func legacyUA(r *http.Request) bool {
	ua := r.Header.Get("User-Agent")
	if strings.Contains(ua, "Web0S") || strings.Contains(ua, "webOS") || r.URL.Query().Get("legacy") == "1" {
		return true
	}
	// cualquier navegador/WebView con Chromium anterior a la v90 (p. ej. Fire TV antiguos)
	if i := strings.Index(ua, "Chrome/"); i >= 0 {
		v := 0
		for _, c := range ua[i+7:] {
			if c < '0' || c > '9' {
				break
			}
			v = v*10 + int(c-'0')
		}
		return v > 0 && v < 90
	}
	return false
}
