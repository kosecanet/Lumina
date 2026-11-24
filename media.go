package main

import (
	"context"
	"crypto/subtle"
	"fmt"
	"io"
	"log"
	"math"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

type Card struct {
	T  string  `json:"t"` // m película, e episodio, s serie
	ID string  `json:"id"`
	Th string  `json:"th"`
	SA bool    `json:"sa,omitempty"`
	Ti string  `json:"ti"`
	Su string  `json:"su,omitempty"`
	P  float64 `json:"p,omitempty"`
	Y  int     `json:"y,omitempty"`
	A  int64   `json:"a,omitempty"`
	D  float64 `json:"d,omitempty"`
	Se int     `json:"se,omitempty"`
	E  int     `json:"e,omitempty"`
	Co string  `json:"co,omitempty"`
}

type Row struct {
	Key   string `json:"key"`
	Title string `json:"title"`
	Link  string `json:"link,omitempty"`
	Cards []Card `json:"cards"`
}

// --- construcción de tarjetas (el llamador debe tener a.mu en lectura) ---

func (a V) frac(it *Item) float64 {
	p, ok := a.pr()[it.ID]
	if !ok {
		return 0
	}
	d := p.Dur
	if d <= 0 {
		d = it.Dur
	}
	if d <= 0 {
		return 0
	}
	f := p.Pos / d
	if f > 1 {
		f = 1
	}
	return f
}

func epTag(it *Item) string {
	if it.Season > 0 {
		return fmt.Sprintf("T%d · E%d", it.Season, it.Episode)
	}
	if it.Episode > 0 {
		return fmt.Sprintf("E%d", it.Episode)
	}
	return ""
}

func (a V) cardItem(it *Item, withProg bool) Card {
	c := Card{ID: it.ID, Th: it.ID, Y: it.Year, A: it.Added, D: it.Dur}
	if it.Kind == "episode" {
		c.T, c.Ti = "e", it.Series
		c.Su = strings.TrimSpace(epTag(it) + " — " + it.Title)
		c.Se, c.E = it.Season, it.Episode
	} else {
		c.T, c.Ti, c.Co = "m", it.Title, it.Collection
		if it.Year > 0 {
			c.Su = strconv.Itoa(it.Year)
		}
	}
	if withProg {
		c.P = a.frac(it)
	}
	return c
}

func (a V) cardSeries(sid string) (Card, bool) {
	eps := a.bySeries[sid]
	if len(eps) == 0 {
		return Card{}, false
	}
	seasons := map[int]bool{}
	var newest int64
	for _, e := range eps {
		seasons[e.Season] = true
		if e.Added > newest {
			newest = e.Added
		}
	}
	su := fmt.Sprintf("%d episodios", len(eps))
	if len(seasons) > 1 {
		su = fmt.Sprintf("%d temporadas", len(seasons))
	} else if len(eps) == 1 {
		su = "1 episodio"
	}
	first := eps[0]
	return Card{T: "s", ID: sid, Th: first.ID, SA: true, Ti: first.Series, Su: su, Y: first.Year, A: newest}, true
}

func (a *App) nextEp(it *Item) *Item {
	eps := a.bySeries[it.SID]
	for i, e := range eps {
		if e.ID == it.ID && i+1 < len(eps) {
			return eps[i+1]
		}
	}
	return nil
}

// ---------- portada ----------

func (a *App) hHome(w http.ResponseWriter, r *http.Request) {
	v := a.V(r)
	res := v.buildHome()
	if v.u.liveOK() {
		res["live"] = v.chanViews(true, 24)
	} else {
		res["live"] = []ChanView{}
	}
	writeJSON(w, r, res)
}

func (a V) buildHome() map[string]any {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var rows []Row
	// seguir viendo + siguiente episodio
	type pe struct {
		id string
		p  Progress
	}
	var pl []pe
	for id, p := range a.pr() {
		pl = append(pl, pe{id, p})
	}
	sort.Slice(pl, func(i, j int) bool { return pl[i].p.At > pl[j].p.At })
	var cont []Card
	seenS := map[string]bool{}
	for _, e := range pl {
		it := a.items[e.id]
		if it == nil || len(cont) >= 20 || !a.libOK(it.Lib) {
			continue
		}
		if it.Kind == "episode" {
			if seenS[it.SID] {
				continue
			}
			seenS[it.SID] = true
		}
		if !finished(e.p) {
			if e.p.Pos >= 10 {
				cont = append(cont, a.cardItem(it, true))
			}
		} else if it.Kind == "episode" {
			if n := a.nextEp(it); n != nil {
				if _, started := a.pr()[n.ID]; !started {
					c := a.cardItem(n, false)
					c.Su = "Siguiente · " + c.Su
					cont = append(cont, c)
				}
			}
		}
	}
	if len(cont) > 0 {
		rows = append(rows, Row{Key: "continue", Title: "Seguir viendo", Cards: cont})
	}

	// añadido recientemente
	var recent []Card
	seenR := map[string]bool{}
	for _, it := range a.all {
		if len(recent) >= 24 {
			break
		}
		if !a.libOK(it.Lib) {
			continue
		}
		if it.Kind == "episode" {
			if seenR[it.SID] {
				continue
			}
			seenR[it.SID] = true
			if c, ok := a.cardSeries(it.SID); ok {
				recent = append(recent, c)
			}
		} else {
			recent = append(recent, a.cardItem(it, true))
		}
	}
	hero := []Card{}
	for _, c := range recent {
		if len(hero) < 5 {
			hero = append(hero, c)
		}
	}
	if len(recent) > 0 {
		rows = append(rows, Row{Key: "recent", Title: "Añadido recientemente", Cards: recent})
	}

	// una fila por biblioteca (y por colección en películas)
	libs := []Library{}
	for _, l := range a.cfg.Libraries {
		if !a.libOK(l.ID) {
			continue
		}
		if !a.u.isAdmin() {
			l.Path = ""
		}
		libs = append(libs, l)
		if l.Type == "series" || l.Type == "cloud" {
			var cs []Card
			seen := map[string]bool{}
			for _, it := range a.all {
				if it.Lib != l.ID || it.Kind != "episode" || seen[it.SID] {
					continue
				}
				seen[it.SID] = true
				if c, ok := a.cardSeries(it.SID); ok {
					cs = append(cs, c)
				}
				if len(cs) >= 30 {
					break
				}
			}
			if len(cs) > 0 {
				rows = append(rows, Row{Key: "lib" + l.ID, Title: l.Name, Link: "#/lib/" + l.ID, Cards: cs})
			}
			continue
		}
		cols := map[string][]*Item{}
		var plain []*Item
		for _, it := range a.all {
			if it.Lib != l.ID {
				continue
			}
			if it.Collection != "" {
				cols[it.Collection] = append(cols[it.Collection], it)
			} else {
				plain = append(plain, it)
			}
		}
		if len(plain) > 0 {
			var cs []Card
			for _, it := range plain {
				if len(cs) >= 30 {
					break
				}
				cs = append(cs, a.cardItem(it, true))
			}
			rows = append(rows, Row{Key: "lib" + l.ID, Title: l.Name, Link: "#/lib/" + l.ID, Cards: cs})
		}
		names := make([]string, 0, len(cols))
		for n := range cols {
			names = append(names, n)
		}
		sort.Strings(names)
		for i, n := range names {
			if i >= 12 {
				break
			}
			list := cols[n]
			sort.Slice(list, func(i, j int) bool { return strings.ToLower(list[i].Title) < strings.ToLower(list[j].Title) })
			var cs []Card
			for _, it := range list {
				if len(cs) >= 30 {
					break
				}
				cs = append(cs, a.cardItem(it, true))
			}
			rows = append(rows, Row{Key: "col" + l.ID + n, Title: n, Link: "#/lib/" + l.ID + "?col=" + urlEsc(n), Cards: cs})
		}
	}
	return map[string]any{
		"hero": hero, "rows": rows, "libs": libs,
		"empty": len(a.cfg.Libraries) == 0 && a.cfg.Xtream.URL == "",
	}
}

func urlEsc(s string) string {
	r := strings.NewReplacer("%", "%25", "&", "%26", "#", "%23", "?", "%3F", "+", "%2B", " ", "%20")
	return r.Replace(s)
}

// ---------- biblioteca, serie, ficha ----------

func (app *App) hLibrary(w http.ResponseWriter, r *http.Request) {
	a := app.V(r)
	id := r.PathValue("id")
	col := r.URL.Query().Get("col")
	l, ok := a.lib(id)
	if !ok {
		jsonErr(w, 404, "biblioteca no encontrada")
		return
	}
	if l.Type == "cloud" {
		cards, pending, err := a.cloudLibCards(l)
		if err != nil {
			jsonErr(w, 502, "no se pudo leer el catálogo del proveedor: "+err.Error())
			return
		}
		seen := map[string]bool{}
		cols := []string{}
		for _, c := range cards {
			if c.Co != "" && !seen[c.Co] {
				seen[c.Co] = true
				cols = append(cols, c.Co)
			}
		}
		sort.Slice(cols, func(i, j int) bool { return strings.ToLower(cols[i]) < strings.ToLower(cols[j]) })
		writeJSON(w, r, map[string]any{"lib": l, "cards": cards, "cols": cols, "pending": pending})
		return
	}
	a.mu.RLock()
	cards := []Card{}
	cols := map[string]bool{}
	seen := map[string]bool{}
	for _, it := range a.all {
		if it.Lib != id {
			continue
		}
		if l.Type == "series" {
			if seen[it.SID] {
				continue
			}
			seen[it.SID] = true
			if c, ok := a.cardSeries(it.SID); ok {
				cards = append(cards, c)
			}
			continue
		}
		if it.Collection != "" {
			cols[it.Collection] = true
		}
		if col != "" && it.Collection != col {
			continue
		}
		cards = append(cards, a.cardItem(it, true))
	}
	a.mu.RUnlock()
	cn := make([]string, 0, len(cols))
	for c := range cols {
		cn = append(cn, c)
	}
	sort.Strings(cn)
	writeJSON(w, r, map[string]any{"lib": l, "cards": cards, "cols": cn})
}

func (app *App) hSeries(w http.ResponseWriter, r *http.Request) {
	a := app.V(r)
	sid := r.PathValue("sid")
	if strings.HasPrefix(sid, "x") {
		if err := a.cloudOpen(sid); err != nil {
			jsonErr(w, 502, "no se pudieron leer los episodios: "+err.Error())
			return
		}
	}
	a.mu.RLock()
	eps := a.bySeries[sid]
	if len(eps) == 0 {
		a.mu.RUnlock()
		jsonErr(w, 404, "serie no encontrada")
		return
	}
	type season struct {
		N   int    `json:"n"`
		Eps []Card `json:"eps"`
	}
	var seasons []*season
	byN := map[int]*season{}
	resume := ""
	var resumeAt int64
	for _, e := range eps {
		s := byN[e.Season]
		if s == nil {
			s = &season{N: e.Season}
			byN[e.Season] = s
			seasons = append(seasons, s)
		}
		c := a.cardItem(e, true)
		c.Ti = e.Title
		c.Su = ""
		s.Eps = append(s.Eps, c)
		if p, ok := a.pr()[e.ID]; ok && p.At > resumeAt {
			resumeAt = p.At
			resume = e.ID
			if finished(p) {
				if n := a.nextEp(e); n != nil {
					resume = n.ID
				} else {
					resume = ""
				}
			}
		}
	}
	if resume == "" {
		resume = eps[0].ID
	}
	first := eps[0]
	res := map[string]any{"sid": sid, "name": first.Series, "lib": first.Lib, "year": first.Year,
		"th": first.ID, "seasons": seasons, "resume": resume, "count": len(eps)}
	a.mu.RUnlock()
	writeJSON(w, r, res)
}

type SubInfo struct {
	N int    `json:"n"`
	L string `json:"l"`
}

func subFiles(it *Item) []string {
	if isCloud(it) {
		return nil
	}
	dir := filepath.Dir(it.Path)
	base := strings.ToLower(strings.TrimSuffix(filepath.Base(it.Path), filepath.Ext(it.Path)))
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range ents {
		n := strings.ToLower(e.Name())
		ext := filepath.Ext(n)
		if (ext == ".srt" || ext == ".vtt") && strings.HasPrefix(n, base) {
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(out)
	return out
}

// textSub: códecs de subtítulos de texto que ffmpeg convierte a WebVTT (los de imagen —PGS, DVD— no se pueden mostrar en la web).
func textSub(c string) bool {
	switch c {
	case "subrip", "srt", "ass", "ssa", "mov_text", "webvtt", "text":
		return true
	}
	return false
}

func plan(it *Item, hasFFmpeg bool) string {
	if !hasFFmpeg || isCloud(it) {
		return "direct"
	}
	ext := strings.ToLower(filepath.Ext(it.Path))
	okC := ext == ".mp4" || ext == ".m4v" || ext == ".mov" || ext == ".webm" || ext == ".mkv"
	switch it.AC {
	case "", "aac", "mp3", "opus", "vorbis", "flac":
	default:
		okC = false
	}
	if okC {
		return "direct"
	}
	if it.VC == "h264" || it.VC == "hevc" || it.VC == "" {
		return "remux"
	}
	return "x264"
}

func (app *App) hItem(w http.ResponseWriter, r *http.Request) {
	a := app.V(r)
	it := a.item(r.PathValue("id"))
	if it == nil {
		jsonErr(w, 404, "no encontrado")
		return
	}
	a.mu.RLock()
	c := a.cardItem(it, true)
	res := map[string]any{
		"card": c, "title": it.Title, "series": it.Series, "sid": it.SID, "season": it.Season, "ep": it.Episode,
		"year": it.Year, "dur": it.Dur, "plan": plan(it, a.ffmpeg != ""), "lib": it.Lib, "kind": it.Kind,
		"pos": a.pr()[it.ID].Pos, "vc": it.VC, "ac": it.AC, "ffmpeg": a.ffmpeg != "", "cloud": isCloud(it),
		"ext": strings.ToLower(filepath.Ext(it.Path)),
	}
	if it.Kind == "episode" {
		if n := a.nextEp(it); n != nil {
			res["next"] = n.ID
		}
	}
	a.mu.RUnlock()
	var subs []SubInfo
	base := strings.ToLower(strings.TrimSuffix(filepath.Base(it.Path), filepath.Ext(it.Path)))
	for i, p := range subFiles(it) {
		lbl := strings.ToLower(strings.TrimSuffix(filepath.Base(p), filepath.Ext(p)))
		lbl = strings.Trim(strings.TrimPrefix(lbl, base), ". _-")
		if lbl == "" {
			lbl = "Subtítulos"
		}
		subs = append(subs, SubInfo{i, lbl})
	}
	res["subs"] = subs
	// pistas incrustadas: audio (cambiar exige conversión → ffmpeg) y subtítulos de texto (se extraen a WebVTT)
	var aud, esub []Track
	a.mu.RLock()
	for _, t := range it.Tracks {
		switch {
		case t.T == "a":
			aud = append(aud, t)
		case t.T == "s" && textSub(t.Codec):
			esub = append(esub, t)
		}
	}
	a.mu.RUnlock()
	if a.ffmpeg == "" {
		aud = nil
		esub = nil
	}
	res["audio"], res["esubs"] = aud, esub
	writeJSON(w, r, res)
}

func (app *App) hSearch(w http.ResponseWriter, r *http.Request) {
	a := app.V(r)
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	cards := []Card{}
	if len(q) >= 2 {
		a.mu.RLock()
		seen := map[string]bool{}
		for _, it := range a.all {
			if len(cards) >= 80 {
				break
			}
			if !a.libOK(it.Lib) {
				continue
			}
			if !strings.Contains(strings.ToLower(it.Title), q) && !strings.Contains(strings.ToLower(it.Series), q) {
				continue
			}
			if it.Kind == "episode" {
				if seen[it.SID] {
					continue
				}
				seen[it.SID] = true
				if c, ok := a.cardSeries(it.SID); ok {
					cards = append(cards, c)
				}
			} else {
				cards = append(cards, a.cardItem(it, true))
			}
		}
		a.mu.RUnlock()
	}
	chans := a.searchChannels(q, 40)
	if !a.u.liveOK() {
		chans = nil
	}
	writeJSON(w, r, map[string]any{"cards": cards, "chans": chans})
}

// ---------- bibliotecas (alta/baja/escaneo) ----------

func (a *App) hLibraries(w http.ResponseWriter, r *http.Request) {
	a.mu.RLock()
	counts := map[string]int{}
	for _, it := range a.items {
		counts[it.Lib]++
	}
	libs := append([]Library(nil), a.cfg.Libraries...)
	sc := a.scan
	a.mu.RUnlock()
	sc.Pending = len(a.aq)
	type lv struct {
		Library
		Count int `json:"count"`
	}
	out := []lv{}
	for _, l := range libs {
		out = append(out, lv{l, counts[l.ID]})
	}
	writeJSON(w, r, map[string]any{"libs": out, "scan": sc})
}

func (a *App) hLibAdd(w http.ResponseWriter, r *http.Request) {
	var in Library
	if err := readJSON(r, &in); err != nil {
		jsonErr(w, 400, "datos no válidos")
		return
	}
	in.Name, in.Path = strings.TrimSpace(in.Name), strings.TrimSpace(in.Path)
	if in.Type == "cloud" {
		in.Path = ""
		if in.Name == "" {
			in.Name = "Series en la nube"
		}
		if len(in.Cats) == 0 && len(in.Sers) == 0 {
			jsonErr(w, 400, "elige al menos una categoría o una serie")
			return
		}
		in.ID = shortHash(fmt.Sprintf("cloud|%s|%d", in.Name, time.Now().UnixNano()))
		a.mu.Lock()
		a.cfg.Libraries = append(a.cfg.Libraries, in)
		a.mu.Unlock()
		a.saveConfig()
		writeJSON(w, r, in)
		return
	}
	if in.Type != "series" {
		in.Type = "movies"
	}
	if st, err := os.Stat(in.Path); err != nil || !st.IsDir() {
		jsonErr(w, 400, "esa carpeta no existe en el servidor")
		return
	}
	if in.Name == "" {
		in.Name = filepath.Base(in.Path)
	}
	in.ID = shortHash(fmt.Sprintf("%s|%s|%d", in.Name, in.Path, time.Now().UnixNano()))
	a.mu.Lock()
	a.cfg.Libraries = append(a.cfg.Libraries, in)
	a.mu.Unlock()
	a.saveConfig()
	a.startScan(in.ID)
	writeJSON(w, r, in)
}

var libIcons = map[string]bool{"film": true, "series": true, "kids": true, "heart": true, "star": true, "music": true, "bolt": true, "globe": true,
	"camera": true, "cloud": true, "folder": true, "play": true, "live": true, "home": true}

func (a *App) hLibUpdate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Icon *string  `json:"icon"`
		Name *string  `json:"name"`
		Cats []string `json:"cats"`
		Sers []int    `json:"sers"`
		Set  bool     `json:"setCloud"` // true: cats/sers sustituyen a lo anterior (biblioteca en la nube)
	}
	if err := readJSON(r, &in); err != nil || (in.Icon != nil && *in.Icon != "" && !libIcons[*in.Icon]) {
		jsonErr(w, 400, "datos no válidos")
		return
	}
	id := r.PathValue("id")
	found := false
	a.mu.Lock()
	for i := range a.cfg.Libraries {
		l := &a.cfg.Libraries[i]
		if l.ID != id {
			continue
		}
		found = true
		if in.Icon != nil {
			l.Icon = *in.Icon
		}
		if in.Name != nil && strings.TrimSpace(*in.Name) != "" {
			l.Name = strings.TrimSpace(*in.Name)
		}
		if in.Set && l.Type == "cloud" {
			l.Cats, l.Sers = in.Cats, in.Sers
		}
	}
	a.mu.Unlock()
	if !found {
		jsonErr(w, 404, "biblioteca no encontrada")
		return
	}
	a.saveConfig()
	w.WriteHeader(204)
}

func (a *App) hLibDel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	a.mu.Lock()
	libs := a.cfg.Libraries[:0:0]
	for _, l := range a.cfg.Libraries {
		if l.ID != id {
			libs = append(libs, l)
		}
	}
	a.cfg.Libraries = libs
	for k, it := range a.items {
		if it.Lib == id {
			delete(a.items, k)
		}
	}
	a.mu.Unlock()
	a.rebuild()
	a.saveConfig()
	a.saveIndex()
	w.WriteHeader(204)
}

func (a *App) hLibScan(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "all" {
		id = ""
	}
	started := a.startScan(id)
	writeJSON(w, r, map[string]any{"started": started})
}

// ---------- archivos: stream, remux, miniaturas, subtítulos ----------

func (a *App) hStream(w http.ResponseWriter, r *http.Request) {
	it := a.item(r.PathValue("id"))
	if it == nil {
		http.NotFound(w, r)
		return
	}
	if isCloud(it) {
		a.cloudStream(w, r, it)
		return
	}
	f, err := os.Open(it.Path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		http.NotFound(w, r)
		return
	}
	ct := map[string]string{".mkv": "video/x-matroska", ".mp4": "video/mp4", ".m4v": "video/mp4", ".webm": "video/webm",
		".mov": "video/quicktime", ".avi": "video/x-msvideo", ".ts": "video/mp2t", ".m2ts": "video/mp2t"}[strings.ToLower(filepath.Ext(it.Path))]
	if ct == "" {
		ct = mime.TypeByExtension(filepath.Ext(it.Path))
	}
	if ct == "" {
		ct = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "private, no-cache")
	http.ServeContent(w, r, "", st.ModTime(), f)
}

// Límites de procesos ffmpeg simultáneos: el remux (copia) es barato; la recodificación x264 no.
var (
	remuxSem = make(chan struct{}, 8)
	x264Sem  = make(chan struct{}, max(1, runtime.NumCPU()/4))
)

// hRemux: MP4 fragmentado al vuelo. m=copy (vídeo tal cual, audio→AAC) o m=x264 (recodifica).
func (a *App) hRemux(w http.ResponseWriter, r *http.Request) {
	it := a.item(r.PathValue("id"))
	if it == nil {
		http.NotFound(w, r)
		return
	}
	if a.ffmpeg == "" {
		jsonErr(w, 501, "ffmpeg no está instalado en el servidor")
		return
	}
	enc := r.URL.Query().Get("m") == "x264"
	sem := remuxSem
	if enc {
		sem = x264Sem
	}
	select {
	case sem <- struct{}{}:
		defer func() { <-sem }()
	case <-time.After(4 * time.Second):
		jsonErr(w, 503, "el servidor está ocupado convirtiendo otros vídeos; inténtalo en unos segundos")
		return
	case <-r.Context().Done():
		return
	}
	ss, _ := strconv.ParseFloat(r.URL.Query().Get("ss"), 64)
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-probesize", "10M", "-analyzeduration", "5M"}
	if ss > 0 {
		args = append(args, "-ss", fmt.Sprintf("%.3f", ss))
	}
	ai, _ := strconv.Atoi(r.URL.Query().Get("a"))
	if ai < 0 || ai > 63 {
		ai = 0
	}
	args = append(args, "-fflags", "+genpts", "-i", it.Path, "-map", "0:v:0", "-map", fmt.Sprintf("0:a:%d?", ai), "-sn", "-dn", "-map_chapters", "-1", "-map_metadata", "-1")
	if enc {
		args = append(args, "-c:v", "libx264", "-preset", "veryfast", "-crf", "23", "-maxrate", "10M", "-bufsize", "20M",
			"-profile:v", "high", "-level", "4.1", "-pix_fmt", "yuv420p", "-vf", "scale='min(1920,iw)':-2", "-g", "48", "-sc_threshold", "0")
	} else {
		args = append(args, "-c:v", "copy", "-avoid_negative_ts", "make_zero")
		if it.VC == "hevc" {
			args = append(args, "-tag:v", "hvc1")
		}
	}
	// aresample async mantiene el audio sincronizado tras cortes o timestamps irregulares
	args = append(args, "-c:a", "aac", "-b:a", "192k", "-ac", "2", "-af", "aresample=async=1:first_pts=0",
		"-max_muxing_queue_size", "2048",
		"-movflags", "frag_keyframe+empty_moov+default_base_moof", "-f", "mp4", "pipe:1")
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	cmd := exec.CommandContext(ctx, a.ffmpeg, args...)
	var stderr limitBuf
	cmd.Stderr = &stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		jsonErr(w, 500, "no se pudo iniciar ffmpeg")
		return
	}
	if err := cmd.Start(); err != nil {
		jsonErr(w, 500, "no se pudo iniciar ffmpeg")
		return
	}
	// primer bloque antes de enviar cabeceras: si ffmpeg falla enseguida devolvemos un error real (el cliente escala a x264)
	buf := make([]byte, 128<<10)
	n, rerr := io.ReadAtLeast(out, buf, 1)
	if n == 0 {
		_ = cmd.Wait()
		log.Printf("ffmpeg (%s) sin salida: %s", it.Path, stderr.String())
		jsonErr(w, 500, "no se pudo convertir este vídeo")
		return
	}
	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Cache-Control", "no-store")
	rc := http.NewResponseController(w)
	write := func(b []byte) bool {
		_ = rc.SetWriteDeadline(time.Now().Add(30 * time.Second)) // un cliente colgado no retiene ffmpeg
		if _, err := w.Write(b); err != nil {
			return false
		}
		return rc.Flush() == nil
	}
	ok := write(buf[:n])
	for ok && rerr == nil {
		n, rerr = out.Read(buf)
		if n > 0 {
			ok = write(buf[:n])
		}
	}
	cancel()
	_ = cmd.Wait()
}

// limitBuf guarda solo el final de la salida de error de ffmpeg (máx. 2 KB).
type limitBuf struct{ b []byte }

func (l *limitBuf) Write(p []byte) (int, error) {
	l.b = append(l.b, p...)
	if len(l.b) > 2048 {
		l.b = l.b[len(l.b)-2048:]
	}
	return len(p), nil
}
func (l *limitBuf) String() string { return strings.TrimSpace(string(l.b)) }

func (a *App) hThumb(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if strings.HasPrefix(id, "x") { // carátula de una serie de la nube
		if u := cloudCover(id); u != "" {
			w.Header().Set("Cache-Control", "public, max-age=86400")
			http.Redirect(w, r, u, http.StatusFound)
			return
		}
		http.NotFound(w, r)
		return
	}
	it := a.item(id)
	if it == nil {
		http.NotFound(w, r)
		return
	}
	if isCloud(it) {
		if u := it.Art; u != "" {
			w.Header().Set("Cache-Control", "public, max-age=86400")
			http.Redirect(w, r, u, http.StatusFound)
			return
		}
		http.NotFound(w, r)
		return
	}
	wd := 640
	if r.URL.Query().Get("w") == "1280" {
		wd = 1280
	}
	series := r.URL.Query().Get("s") == "1"
	w.Header().Set("Cache-Control", "public, max-age=2592000")
	if p := a.ensureThumb(it, wd, series); p != "" {
		http.ServeFile(w, r, p)
		return
	}
	art := it.Art
	if series && it.SArt != "" {
		art = it.SArt
	}
	if art != "" {
		http.ServeFile(w, r, art)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	http.NotFound(w, r)
}

func (a *App) hSub(w http.ResponseWriter, r *http.Request) {
	it := a.item(r.PathValue("id"))
	if it == nil {
		http.NotFound(w, r)
		return
	}
	name := strings.TrimSuffix(r.PathValue("n"), ".vtt")
	var s string
	if strings.HasPrefix(name, "e") { // subtítulo incrustado en el vídeo
		n, err := strconv.Atoi(name[1:])
		if err != nil || n < 0 || n > 63 || a.ffmpeg == "" {
			http.NotFound(w, r)
			return
		}
		b, err := a.embeddedSub(it, n)
		if err != nil {
			jsonErr(w, 502, "no se pudieron extraer los subtítulos")
			return
		}
		s = string(b)
	} else {
		n, err := strconv.Atoi(name)
		files := subFiles(it)
		if err != nil || n < 0 || n >= len(files) {
			http.NotFound(w, r)
			return
		}
		b, err := os.ReadFile(files[n])
		if err != nil {
			http.NotFound(w, r)
			return
		}
		b = []byte(strings.TrimPrefix(string(b), "\xef\xbb\xbf"))
		s = string(b)
		if !utf8.Valid(b) { // latin-1 / windows-1252
			rs := make([]rune, len(b))
			for i, c := range b {
				rs[i] = rune(c)
			}
			s = string(rs)
		}
		s = strings.ReplaceAll(s, "\r\n", "\n")
		if !strings.HasPrefix(strings.TrimSpace(s), "WEBVTT") {
			lines := strings.Split(s, "\n")
			for i, ln := range lines {
				if strings.Contains(ln, "-->") {
					lines[i] = strings.ReplaceAll(ln, ",", ".")
				}
			}
			s = "WEBVTT\n\n" + strings.Join(lines, "\n")
		}
	}
	// en modo conversión el vídeo empieza en 0 tras cada salto: se desplazan los tiempos
	if off, _ := strconv.ParseFloat(r.URL.Query().Get("off"), 64); off > 0.5 {
		s = shiftVTT(s, off)
	}
	w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
	w.Header().Set("Cache-Control", "private, max-age=600")
	_, _ = io.WriteString(w, s)
}

var subLocks sync.Map

// embeddedSub extrae (una sola vez, con caché en disco) el subtítulo n del contenedor a WebVTT.
func (a *App) embeddedSub(it *Item, n int) ([]byte, error) {
	out := filepath.Join(a.dir, "subs", fmt.Sprintf("%s_%d.vtt", it.ID, n))
	if b, err := os.ReadFile(out); err == nil {
		return b, nil
	}
	mi, _ := subLocks.LoadOrStore(out, new(sync.Mutex))
	l := mi.(*sync.Mutex)
	l.Lock()
	defer l.Unlock()
	if b, err := os.ReadFile(out); err == nil {
		return b, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	b, err := exec.CommandContext(ctx, a.ffmpeg, "-hide_banner", "-loglevel", "error", "-nostdin", "-i", it.Path,
		"-map", fmt.Sprintf("0:s:%d", n), "-f", "webvtt", "pipe:1").Output()
	if err != nil || len(b) == 0 {
		return nil, fmt.Errorf("ffmpeg: %v", err)
	}
	tmp := out + ".tmp"
	if os.WriteFile(tmp, b, 0o644) == nil {
		_ = os.Rename(tmp, out)
	}
	return b, nil
}

// shiftVTT resta off segundos a todos los tiempos de un WebVTT y descarta lo que quede antes de 0.
func shiftVTT(s string, off float64) string {
	parse := func(t string) (float64, bool) {
		t = strings.TrimSpace(t)
		parts := strings.Split(strings.ReplaceAll(t, ",", "."), ":")
		if len(parts) < 2 || len(parts) > 3 {
			return 0, false
		}
		v := 0.0
		for _, p := range parts {
			f, err := strconv.ParseFloat(p, 64)
			if err != nil {
				return 0, false
			}
			v = v*60 + f
		}
		return v, true
	}
	format := func(v float64) string {
		ms := int(v*1000 + .5)
		return fmt.Sprintf("%02d:%02d:%02d.%03d", ms/3600000, ms/60000%60, ms/1000%60, ms%1000)
	}
	blocks := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n\n")
	var out []string
	for i, bl := range blocks {
		if i == 0 && strings.HasPrefix(strings.TrimSpace(bl), "WEBVTT") {
			out = append(out, bl)
			continue
		}
		lines := strings.Split(bl, "\n")
		keep := true
		for j, ln := range lines {
			k := strings.Index(ln, "-->")
			if k < 0 {
				continue
			}
			rest := strings.TrimSpace(ln[k+3:])
			end, extra := rest, ""
			if sp := strings.IndexAny(rest, " \t"); sp > 0 {
				end, extra = rest[:sp], rest[sp:]
			}
			t0, ok1 := parse(ln[:k])
			t1, ok2 := parse(end)
			if !ok1 || !ok2 {
				break
			}
			if t1-off <= 0 {
				keep = false
				break
			}
			lines[j] = format(math.Max(0, t0-off)) + " --> " + format(t1-off) + extra
			break
		}
		if keep {
			out = append(out, strings.Join(lines, "\n"))
		}
	}
	return strings.Join(out, "\n\n")
}

func (a *App) hProgress(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID  string  `json:"id"`
		Pos float64 `json:"pos"`
		Dur float64 `json:"dur"`
	}
	if err := readJSON(r, &in); err != nil || a.item(in.ID) == nil {
		w.WriteHeader(400)
		return
	}
	if !userFrom(r).libOK(a.item(in.ID).Lib) {
		w.WriteHeader(403)
		return
	}
	a.setProgress(a.pidOf(r), in.ID, in.Pos, in.Dur)
	w.WriteHeader(204)
}

// ---------- explorador de carpetas del servidor y sistema ----------

func (a *App) hFS(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Query().Get("path")
	type dir struct {
		N string `json:"n"`
		P string `json:"p"`
	}
	dirs := []dir{}
	if p == "" && runtime.GOOS == "windows" {
		for c := 'A'; c <= 'Z'; c++ {
			d := string(c) + ":\\"
			if _, err := os.Stat(d); err == nil {
				dirs = append(dirs, dir{d, d})
			}
		}
		writeJSON(w, r, map[string]any{"path": "", "parent": "", "dirs": dirs})
		return
	}
	if p == "" {
		p = "/"
		if h, err := os.UserHomeDir(); err == nil {
			p = h
		}
	}
	p = filepath.Clean(p)
	ents, err := os.ReadDir(p)
	if err != nil {
		jsonErr(w, 400, "no se puede leer esa carpeta")
		return
	}
	for _, e := range ents {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			dirs = append(dirs, dir{e.Name(), filepath.Join(p, e.Name())})
		}
	}
	sort.Slice(dirs, func(i, j int) bool { return strings.ToLower(dirs[i].N) < strings.ToLower(dirs[j].N) })
	parent := filepath.Dir(p)
	if parent == p {
		parent = ""
		if runtime.GOOS == "windows" {
			parent = ""
		}
	}
	writeJSON(w, r, map[string]any{"path": p, "parent": parent, "dirs": dirs})
}

func (a *App) hSystem(w http.ResponseWriter, r *http.Request) {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	a.mu.RLock()
	res := map[string]any{
		"version": version, "items": len(a.items), "channels": len(a.cat.Channels), "selected": len(a.cfg.Selected),
		"ffmpeg": a.ffmpeg != "", "ffprobe": a.ffprobe != "",
		"heapMB": float64(ms.HeapAlloc) / 1048576, "sysMB": float64(ms.Sys) / 1048576,
		"goroutines": runtime.NumGoroutine(), "uptime": int64(time.Since(a.started).Seconds()),
		"scan": a.scan, "pending": len(a.aq), "epgAt": a.epgAt, "epgChannels": len(a.epg), "catAt": a.cat.Updated,
		"password": a.pass != "" || len(a.cfg.Users) > 0,
	}
	a.mu.RUnlock()
	res["live"] = a.hub.status()
	writeJSON(w, r, res)
}

func (a *App) hAuth(w http.ResponseWriter, r *http.Request) {
	if a.accounts() {
		writeJSON(w, r, map[string]any{"required": true, "ok": a.sessionUser(r) != nil, "accounts": true})
		return
	}
	writeJSON(w, r, map[string]any{"required": a.pass != "", "ok": a.authed(r), "accounts": false})
}

func (a *App) hLogin(w http.ResponseWriter, r *http.Request) {
	var in struct {
		User     string `json:"user"`
		Password string `json:"password"`
	}
	_ = readJSON(r, &in)
	ip := clientIP(r)
	if !loginAllowed(ip) {
		jsonErr(w, 429, "demasiados intentos; espera un minuto")
		return
	}
	if a.accounts() {
		u := a.loginAccount(in.User, in.Password)
		if u == nil {
			loginFailed(ip)
			log.Printf("intento de acceso fallido desde %s", r.RemoteAddr)
			jsonErr(w, 401, "usuario o contraseña incorrectos")
			return
		}
		setSessionCookie(w, a.newSession(u.ID))
		writeJSON(w, r, map[string]any{"ok": true})
		return
	}
	if a.pass == "" || subtle.ConstantTimeCompare([]byte(in.Password), []byte(a.pass)) != 1 {
		loginFailed(ip)
		log.Printf("intento de acceso fallido desde %s", r.RemoteAddr)
		jsonErr(w, 401, "contraseña incorrecta")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "lum", Value: a.token, Path: "/", HttpOnly: true,
		SameSite: http.SameSiteLaxMode, MaxAge: 365 * 24 * 3600})
	writeJSON(w, r, map[string]any{"ok": true})
}

// ---------- rutas ----------

func (a *App) routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/auth", a.hAuth)
	mux.HandleFunc("GET /api/me", a.hMe)
	mux.HandleFunc("POST /api/logout", a.hLogout)
	mux.HandleFunc("GET /api/users", a.hUsers)
	mux.HandleFunc("POST /api/users", a.hUserCreate)
	mux.HandleFunc("PUT /api/users/{id}", a.hUserUpdate)
	mux.HandleFunc("DELETE /api/users/{id}", a.hUserDelete)
	mux.HandleFunc("GET /api/profiles", a.hProfiles)
	mux.HandleFunc("POST /api/profiles", a.hProfileCreate)
	mux.HandleFunc("PUT /api/profiles/{id}", a.hProfileUpdate)
	mux.HandleFunc("DELETE /api/profiles/{id}", a.hProfileDelete)
	mux.HandleFunc("POST /api/profiles/{id}/select", a.hProfileSelect)
	mux.HandleFunc("GET /api/update", a.hUpdate)
	mux.HandleFunc("POST /api/update/config", a.hUpdateCfg)
	mux.HandleFunc("POST /api/update/apply", a.hUpdateApply)
	mux.HandleFunc("POST /api/login", a.hLogin)
	mux.HandleFunc("GET /api/home", a.hHome)
	mux.HandleFunc("GET /api/library/{id}", a.hLibrary)
	mux.HandleFunc("GET /api/series/{sid}", a.hSeries)
	mux.HandleFunc("GET /api/item/{id}", a.hItem)
	mux.HandleFunc("GET /api/meta/{id}", a.hMeta)
	mux.HandleFunc("GET /api/cloud/cats", a.hCloudCats)
	mux.HandleFunc("GET /api/cloud/cat/{id}", a.hCloudCat)
	mux.HandleFunc("POST /api/vodtune", a.hVodTune)
	mux.HandleFunc("GET /api/system/meta", a.hMetaCfg)
	mux.HandleFunc("POST /api/system/meta", a.hMetaCfg)
	mux.HandleFunc("GET /api/search", a.hSearch)
	mux.HandleFunc("GET /api/libraries", a.hLibraries)
	mux.HandleFunc("POST /api/libraries", a.hLibAdd)
	mux.HandleFunc("PUT /api/libraries/{id}", a.hLibUpdate)
	mux.HandleFunc("DELETE /api/libraries/{id}", a.hLibDel)
	mux.HandleFunc("POST /api/libraries/{id}/scan", a.hLibScan)
	mux.HandleFunc("GET /api/thumb/{id}", a.hThumb)
	mux.HandleFunc("POST /api/progress", a.hProgress)
	mux.HandleFunc("GET /api/fs", a.hFS)
	mux.HandleFunc("GET /api/system", a.hSystem)
	mux.HandleFunc("GET /stream/{id}", a.hStream)
	mux.HandleFunc("GET /remux/{id}", a.hRemux)
	mux.HandleFunc("GET /sub/{id}/{n}", a.hSub)

	// Xtream y TV en directo
	mux.HandleFunc("GET /api/xtream/config", a.hXtreamGet)
	mux.HandleFunc("POST /api/xtream/config", a.hXtreamSave)
	mux.HandleFunc("POST /api/xtream/refresh", a.hXtreamRefresh)
	mux.HandleFunc("GET /api/xtream/categories", a.hXtreamCats)
	mux.HandleFunc("GET /api/xtream/channels", a.hXtreamChannels)
	mux.HandleFunc("POST /api/xtream/select", a.hXtreamSelect)
	mux.HandleFunc("POST /api/xtream/select-query", a.hXtreamSelectQuery)
	mux.HandleFunc("POST /api/xtream/move", a.hXtreamMove)
	mux.HandleFunc("POST /api/xtream/fav", a.hXtreamFav)
	mux.HandleFunc("GET /api/live/channels", a.hLiveChannels)
	mux.HandleFunc("GET /api/live/epg/{id}", a.hLiveEPG)
	mux.HandleFunc("POST /api/live/tune", a.hLiveTune)
	mux.HandleFunc("POST /api/live/warm", a.hLiveWarm)
	mux.HandleFunc("GET /api/live/status", a.hLiveStatus)
	mux.HandleFunc("GET /api/logo/{id}", a.hLogo)
	mux.HandleFunc("GET /live/{id}", a.hLiveStream)

	mux.Handle("/", a.static())
}

// Límite de intentos de login: 8 fallos por IP y minuto.
var (
	loginMu   sync.Mutex
	loginFail = map[string][]time.Time{}
)

func loginAllowed(ip string) bool {
	loginMu.Lock()
	defer loginMu.Unlock()
	cut := time.Now().Add(-time.Minute)
	l := loginFail[ip][:0]
	for _, t := range loginFail[ip] {
		if t.After(cut) {
			l = append(l, t)
		}
	}
	if len(l) == 0 {
		delete(loginFail, ip)
	} else {
		loginFail[ip] = l
	}
	return len(l) < 8
}

func loginFailed(ip string) {
	loginMu.Lock()
	loginFail[ip] = append(loginFail[ip], time.Now())
	loginMu.Unlock()
}
