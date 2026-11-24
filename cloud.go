package main

// Series de la nube: el catálogo de series (no en directo) de tu servicio Xtream, como si fuera una biblioteca.
//  - Una "biblioteca en la nube" es una Library con Type "cloud" y las categorías / series elegidas.
//  - El catálogo se lee por categorías (get_series entero pesa decenas de MB) y se guarda en memoria.
//  - Al abrir una serie se leen sus episodios y se registran como elementos normales (Path "cloud:<id>.<ext>"),
//    así funcionan el progreso, "siguiente episodio", la búsqueda, etc.
//  - La reproducción pasa por Lumina y ocupa la conexión del proveedor igual que un canal en directo.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type cCat struct {
	ID string `json:"id"`
	N  string `json:"n"`
}

type cSer struct {
	ID     int     `json:"id"`
	Name   string  `json:"n"`
	Cover  string  `json:"-"`
	Plot   string  `json:"-"`
	Year   int     `json:"y,omitempty"`
	Rating float64 `json:"r,omitempty"`
	Cat    string  `json:"-"`
}

type cEp struct {
	ID     string
	Num    int
	Season int
	Title  string
	Ext    string
	Dur    float64
	Image  string
	VC, AC string
}

type cInfo struct {
	Name, Cover string
	Year        int
	Eps         []cEp
	At          time.Time
}

var cloud struct {
	mu    sync.Mutex
	cats  []cCat
	catAt time.Time
	byCat map[string][]*cSer
	catTm map[string]time.Time
	sers  map[int]*cSer
	info  map[int]*cInfo
}

var cloudOpenMu sync.Mutex

// vodClient: reproducción desde el proveedor (sin plazo total: un episodio dura horas).
var vodClient = &http.Client{Transport: liveClient.Transport}

const cloudTTL = 6 * time.Hour

func cloudInit() {
	if cloud.byCat == nil {
		cloud.byCat = map[string][]*cSer{}
		cloud.catTm = map[string]time.Time{}
		cloud.sers = map[int]*cSer{}
		cloud.info = map[int]*cInfo{}
	}
}

var langPrefix = regexp.MustCompile(`^\s*[A-Z]{2,3}\s*[-–—|:]\s+`)

// stripLang quita la etiqueta de idioma que muchos proveedores ponen delante («ES - », «EN - », «LAT | »…).
func stripLang(s string) string {
	s = strings.TrimSpace(s)
	if t := strings.TrimSpace(langPrefix.ReplaceAllString(s, "")); t != "" {
		return t
	}
	return s
}

func yearOf(s string) int {
	s = strings.TrimSpace(s)
	if len(s) >= 4 {
		if n, err := strconv.Atoi(s[:4]); err == nil && n > 1800 && n < 2200 {
			return n
		}
	}
	return 0
}

func (a *App) cloudCats(refresh bool) ([]cCat, error) {
	a.cloudDiskLoad()
	cloud.mu.Lock()
	cloudInit()
	if !refresh && len(cloud.cats) > 0 && time.Since(cloud.catAt) < 12*time.Hour {
		c := cloud.cats
		cloud.mu.Unlock()
		return c, nil
	}
	cloud.mu.Unlock()
	c, err := a.xclient()
	if err != nil {
		return nil, err
	}
	var raw []struct {
		ID   flexStr `json:"category_id"`
		Name flexStr `json:"category_name"`
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := c.getJSON(ctx, c.apiURL("get_series_categories"), &raw); err != nil {
		return nil, err
	}
	out := make([]cCat, 0, len(raw))
	for _, r := range raw {
		out = append(out, cCat{ID: string(r.ID), N: strings.TrimSpace(string(r.Name))})
	}
	sort.SliceStable(out, func(i, j int) bool { return strings.ToLower(out[i].N) < strings.ToLower(out[j].N) })
	cloud.mu.Lock()
	cloud.cats, cloud.catAt = out, time.Now()
	cloud.mu.Unlock()
	return out, nil
}

// cloudCat: series de una categoría. Con caché (en memoria o en disco) responde al instante aunque esté caducada y la
// renueva en segundo plano; solo espera al proveedor si nunca se ha leído.
func (a *App) cloudCat(id string, refresh bool) ([]*cSer, error) {
	a.cloudDiskLoad()
	cloud.mu.Lock()
	cloudInit()
	l, ok := cloud.byCat[id]
	old := time.Since(cloud.catTm[id]) >= cloudTTL
	cloud.mu.Unlock()
	if ok && !refresh {
		if old {
			go a.cloudSingle(id)
		}
		return l, nil
	}
	return a.cloudSingle(id)
}

var cloudFlight struct {
	mu sync.Mutex
	m  map[string]chan struct{}
}
var cloudFail sync.Map                // categoría -> último error (se borra al leerse bien)
var cloudSem = make(chan struct{}, 4) // peticiones simultáneas al proveedor para el catálogo

// cloudSingle: una sola lectura por categoría a la vez; las demás esperan a que termine.
func (a *App) cloudSingle(id string) ([]*cSer, error) {
	cloudFlight.mu.Lock()
	if cloudFlight.m == nil {
		cloudFlight.m = map[string]chan struct{}{}
	}
	if ch, ok := cloudFlight.m[id]; ok {
		cloudFlight.mu.Unlock()
		<-ch
		cloud.mu.Lock()
		l, ok := cloud.byCat[id]
		cloud.mu.Unlock()
		if ok {
			return l, nil
		}
		return nil, errors.New("no se pudo leer la categoría")
	}
	ch := make(chan struct{})
	cloudFlight.m[id] = ch
	cloudFlight.mu.Unlock()
	cloudSem <- struct{}{}
	l, err := a.cloudFetchCat(id)
	<-cloudSem
	cloudFlight.mu.Lock()
	delete(cloudFlight.m, id)
	cloudFlight.mu.Unlock()
	close(ch)
	if err == nil {
		cloudFail.Delete(id)
		a.cloudDiskSave()
	}
	return l, err
}

type cDisk struct {
	Cats  []cCat               `json:"cats"`
	CatAt int64                `json:"catAt"`
	By    map[string]cDiskList `json:"by"`
}
type cDiskList struct {
	At   int64      `json:"at"`
	List []cDiskSer `json:"l"`
}
type cDiskSer struct {
	ID    int     `json:"id"`
	N     string  `json:"n"`
	Cover string  `json:"c,omitempty"`
	Plot  string  `json:"p,omitempty"`
	Y     int     `json:"y,omitempty"`
	R     float64 `json:"r,omitempty"`
	Cat   string  `json:"k,omitempty"`
}

var cloudDiskOnce sync.Once
var cloudSaveMu sync.Mutex

// cloudDiskLoad: recupera el catálogo guardado, para que tras reiniciar la biblioteca salga al instante.
func (a *App) cloudDiskLoad() {
	cloudDiskOnce.Do(func() {
		var d cDisk
		a.load("cloudcache.json", &d)
		cloud.mu.Lock()
		defer cloud.mu.Unlock()
		cloudInit()
		if len(cloud.cats) == 0 && len(d.Cats) > 0 {
			cloud.cats, cloud.catAt = d.Cats, time.Unix(d.CatAt, 0)
		}
		for id, dl := range d.By {
			if _, ok := cloud.byCat[id]; ok {
				continue
			}
			out := make([]*cSer, 0, len(dl.List))
			for _, x := range dl.List {
				s := &cSer{ID: x.ID, Name: stripLang(x.N), Cover: x.Cover, Plot: x.Plot, Year: x.Y, Rating: x.R, Cat: x.Cat}
				out = append(out, s)
				cloud.sers[s.ID] = s
			}
			cloud.byCat[id], cloud.catTm[id] = out, time.Unix(dl.At, 0)
		}
	})
}

func (a *App) cloudDiskSave() {
	cloudSaveMu.Lock()
	defer cloudSaveMu.Unlock()
	d := cDisk{By: map[string]cDiskList{}}
	cloud.mu.Lock()
	d.Cats, d.CatAt = cloud.cats, cloud.catAt.Unix()
	for id, l := range cloud.byCat {
		dl := cDiskList{At: cloud.catTm[id].Unix()}
		for _, s := range l {
			dl.List = append(dl.List, cDiskSer{ID: s.ID, N: s.Name, Cover: s.Cover, Plot: s.Plot, Y: s.Year, R: s.Rating, Cat: s.Cat})
		}
		d.By[id] = dl
	}
	cloud.mu.Unlock()
	if b, err := json.Marshal(d); err == nil {
		a.writeFile("cloudcache.json", b)
	}
}

func (a *App) cloudFetchCat(id string) ([]*cSer, error) {
	c, err := a.xclient()
	if err != nil {
		return nil, err
	}
	var raw []struct {
		ID      flexInt `json:"series_id"`
		Name    flexStr `json:"name"`
		Cover   flexStr `json:"cover"`
		Plot    flexStr `json:"plot"`
		Release flexStr `json:"releaseDate"`
		Rel2    flexStr `json:"release_date"`
		Rating  flexStr `json:"rating"`
	}
	ctx, cancel := context.WithTimeout(context.Background(), 85*time.Second)
	defer cancel()
	if err := c.getJSON(ctx, c.apiURL("get_series")+"&category_id="+url.QueryEscape(id), &raw); err != nil {
		return nil, err
	}
	out := make([]*cSer, 0, len(raw))
	for _, r := range raw {
		if r.ID == 0 {
			continue
		}
		rel := string(r.Release)
		if rel == "" {
			rel = string(r.Rel2)
		}
		rt, _ := strconv.ParseFloat(strings.TrimSpace(string(r.Rating)), 64)
		out = append(out, &cSer{ID: int(r.ID), Name: stripLang(string(r.Name)), Cover: string(r.Cover), Plot: string(r.Plot), Year: yearOf(rel), Rating: rt, Cat: id})
	}
	sort.SliceStable(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	cloud.mu.Lock()
	cloud.byCat[id], cloud.catTm[id] = out, time.Now()
	for _, s := range out {
		cloud.sers[s.ID] = s
	}
	cloud.mu.Unlock()
	return out, nil
}

// cloudSeriesInfo lee los episodios de una serie (caché de 1 h).
func (a *App) cloudSeriesInfo(sid int) (*cInfo, error) {
	cloud.mu.Lock()
	cloudInit()
	if i := cloud.info[sid]; i != nil && time.Since(i.At) < time.Hour {
		cloud.mu.Unlock()
		return i, nil
	}
	cloud.mu.Unlock()
	c, err := a.xclient()
	if err != nil {
		return nil, err
	}
	var raw struct {
		Info struct {
			Name    flexStr `json:"name"`
			Cover   flexStr `json:"cover"`
			Release flexStr `json:"releaseDate"`
			Rel2    flexStr `json:"release_date"`
		} `json:"info"`
		Episodes json.RawMessage `json:"episodes"`
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := c.getJSON(ctx, c.apiURL("get_series_info")+"&series_id="+strconv.Itoa(sid), &raw); err != nil {
		return nil, err
	}
	type epJ struct {
		ID     flexStr         `json:"id"`
		Num    flexInt         `json:"episode_num"`
		Title  flexStr         `json:"title"`
		Ext    flexStr         `json:"container_extension"`
		Season flexInt         `json:"season"`
		Info   json.RawMessage `json:"info"`
	}
	var groups = map[string][]epJ{}
	eb := strings.TrimSpace(string(raw.Episodes))
	switch {
	case strings.HasPrefix(eb, "{"):
		_ = json.Unmarshal(raw.Episodes, &groups)
	case strings.HasPrefix(eb, "["):
		var arr [][]epJ
		if json.Unmarshal(raw.Episodes, &arr) == nil {
			for i, g := range arr {
				groups[strconv.Itoa(i+1)] = g
			}
		}
	}
	rel := string(raw.Info.Release)
	if rel == "" {
		rel = string(raw.Info.Rel2)
	}
	inf := &cInfo{Name: stripLang(string(raw.Info.Name)), Cover: string(raw.Info.Cover), Year: yearOf(rel), At: time.Now()}
	for sk, g := range groups {
		sn, _ := strconv.Atoi(sk)
		for _, e := range g {
			if e.ID == "" {
				continue
			}
			ep := cEp{ID: string(e.ID), Num: int(e.Num), Season: int(e.Season), Title: strings.TrimSpace(string(e.Title)), Ext: strings.TrimPrefix(strings.ToLower(string(e.Ext)), ".")}
			if ep.Season == 0 {
				ep.Season = sn
			}
			if ep.Ext == "" {
				ep.Ext = "mp4"
			}
			var ei struct {
				Secs  flexInt         `json:"duration_secs"`
				Image flexStr         `json:"movie_image"`
				Video json.RawMessage `json:"video"`
				Audio json.RawMessage `json:"audio"`
			}
			if len(e.Info) > 0 && e.Info[0] == '{' {
				_ = json.Unmarshal(e.Info, &ei)
			}
			ep.Dur, ep.Image = float64(ei.Secs), string(ei.Image)
			codec := func(b json.RawMessage) string {
				var c struct {
					N flexStr `json:"codec_name"`
				}
				if len(b) > 0 && b[0] == '{' && json.Unmarshal(b, &c) == nil {
					return strings.ToLower(string(c.N))
				}
				return ""
			}
			ep.VC, ep.AC = codec(ei.Video), codec(ei.Audio)
			inf.Eps = append(inf.Eps, ep)
		}
	}
	sort.Slice(inf.Eps, func(i, j int) bool {
		if inf.Eps[i].Season != inf.Eps[j].Season {
			return inf.Eps[i].Season < inf.Eps[j].Season
		}
		return inf.Eps[i].Num < inf.Eps[j].Num
	})
	cloud.mu.Lock()
	cloud.info[sid] = inf
	cloud.mu.Unlock()
	return inf, nil
}

// cloudLibFor devuelve la biblioteca en la nube a la que pertenece una serie ("" si no se sabe).
func (a *App) cloudLibFor(sid int) string {
	cloud.mu.Lock()
	cloudInit()
	cat := ""
	if s := cloud.sers[sid]; s != nil {
		cat = s.Cat
	}
	cloud.mu.Unlock()
	a.mu.RLock()
	defer a.mu.RUnlock()
	first := ""
	for _, l := range a.cfg.Libraries {
		if l.Type != "cloud" {
			continue
		}
		if first == "" {
			first = l.ID
		}
		for _, id := range l.Sers {
			if id == sid {
				return l.ID
			}
		}
		for _, c := range l.Cats {
			if c == cat && cat != "" {
				return l.ID
			}
		}
	}
	return first
}

// cloudOpen registra como elementos los episodios de una serie (id "x<serie>") para poder reproducirlos.
func (a *App) cloudOpen(sidStr string) error {
	sid, err := strconv.Atoi(strings.TrimPrefix(sidStr, "x"))
	if err != nil || sid <= 0 {
		return errors.New("serie no válida")
	}
	lib := a.cloudLibFor(sid)
	if lib == "" {
		return errors.New("esa serie no está en ninguna biblioteca de la nube")
	}
	cloudOpenMu.Lock()
	defer cloudOpenMu.Unlock()
	inf, err := a.cloudSeriesInfo(sid)
	if err != nil {
		return err
	}
	cloud.mu.Lock()
	cs := cloud.sers[sid]
	cloud.mu.Unlock()
	name := stripLang(inf.Name)
	if name == "" && cs != nil {
		name = stripLang(cs.Name)
	}
	cover := inf.Cover
	if cover == "" && cs != nil {
		cover = cs.Cover
	}
	year := inf.Year
	if year == 0 && cs != nil {
		year = cs.Year
	}
	now := time.Now().Unix()
	a.mu.Lock()
	for _, e := range inf.Eps {
		id := "c" + e.ID
		title := stripLang(e.Title)
		if title == "" {
			title = fmt.Sprintf("Episodio %d", e.Num)
		}
		art := e.Image
		if art == "" {
			art = cover
		}
		it := &Item{ID: id, Lib: lib, Kind: "episode", Title: title, Year: year, Series: name, SID: sidStr, Season: e.Season, Episode: e.Num,
			Path: "cloud:" + e.ID + "." + e.Ext, Mod: 1, Added: now, Dur: e.Dur, VC: e.VC, AC: e.AC, Probed: true, PV: probeVersion, Art: art, SArt: cover}
		if old := a.items[id]; old != nil {
			it.Added = old.Added
		}
		a.items[id] = it
	}
	a.mu.Unlock()
	a.rebuild()
	go a.saveIndex()
	return nil
}

// ---------- reproducción ----------

func isCloud(it *Item) bool { return it != nil && strings.HasPrefix(it.Path, "cloud:") }

func cloudKey(it *Item) int {
	n, _ := strconv.Atoi(strings.TrimPrefix(it.SID, "x"))
	return -n
}

func (a *App) cloudURL(path string) (string, *xc, error) {
	c, err := a.xclient()
	if err != nil {
		return "", nil, err
	}
	f := strings.TrimPrefix(path, "cloud:")
	return fmt.Sprintf("%s/series/%s/%s/%s", c.base, url.PathEscape(c.user), url.PathEscape(c.pass), url.PathEscape(f)), c, nil
}

func busyJSON(w http.ResponseWriter, busy []BusyInfo, max int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(409)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": "busy", "playing": busy, "max": max})
}

// cloudStream reenvía un episodio del proveedor al navegador (con soporte de Range para poder saltar).
func (a *App) cloudStream(w http.ResponseWriter, r *http.Request, it *Item) {
	u, c, err := a.cloudURL(it.Path)
	if err != nil {
		jsonErr(w, 502, err.Error())
		return
	}
	ls, sub, busy := a.hub.vodAcquire(cloudKey(it), "«"+it.Series+"»", false)
	if busy != nil {
		busyJSON(w, busy, a.maxConn())
		return
	}
	defer a.hub.release(ls, sub)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	go func() {
		select {
		case <-ls.ctx.Done():
			cancel()
		case <-ctx.Done():
		}
	}()
	if ls.swapCur(cancel) {
		select { // el proveedor tarda un instante en liberar la conexión anterior
		case <-time.After(350 * time.Millisecond):
		case <-ctx.Done():
			return
		}
	}
	var resp *http.Response
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-time.After(time.Duration(attempt) * 900 * time.Millisecond):
			case <-ctx.Done():
				return
			}
		}
		req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
		req.Header.Set("User-Agent", c.ua)
		if rg := r.Header.Get("Range"); rg != "" {
			req.Header.Set("Range", rg)
		}
		resp, err = vodClient.Do(req)
		if err != nil {
			log.Printf("nube: fallo al conectar (intento %d): %v", attempt+1, cleanErr(err))
			resp = nil
			continue
		}
		if resp.StatusCode == 200 || resp.StatusCode == 206 {
			break
		}
		log.Printf("nube: el proveedor respondió HTTP %d (intento %d)", resp.StatusCode, attempt+1)
		resp.Body.Close()
		resp = nil
	}
	if resp == nil {
		jsonErr(w, 502, "el proveedor no entrega este episodio ahora (¿límite de conexiones?)")
		return
	}
	defer resp.Body.Close()
	for _, h := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "Last-Modified", "ETag"} {
		if v := resp.Header.Get(h); v != "" {
			w.Header().Set(h, v)
		}
	}
	if w.Header().Get("Content-Type") == "" || strings.Contains(strings.ToLower(w.Header().Get("Content-Type")), "octet-stream") {
		switch {
		case strings.HasSuffix(it.Path, ".mkv"):
			w.Header().Set("Content-Type", "video/x-matroska")
		case strings.HasSuffix(it.Path, ".webm"):
			w.Header().Set("Content-Type", "video/webm")
		default:
			w.Header().Set("Content-Type", "video/mp4")
		}
	}
	w.Header().Set("Cache-Control", "private, no-store")
	w.WriteHeader(resp.StatusCode)
	rc := http.NewResponseController(w)
	buf := make([]byte, 128<<10)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			_ = rc.SetWriteDeadline(time.Now().Add(45 * time.Second))
			if _, werr := w.Write(buf[:n]); werr != nil {
				return
			}
		}
		if rerr != nil {
			if rerr != io.EOF {
				log.Printf("nube: lectura interrumpida: %v", cleanErr(rerr))
			}
			return
		}
	}
}

// POST /api/vodtune: comprueba, antes de reproducir, si hay una conexión libre (o hace sitio si force).
func (a *App) hVodTune(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID    string `json:"id"`
		Force bool   `json:"force"`
	}
	if err := readJSON(r, &in); err != nil {
		jsonErr(w, 400, "datos no válidos")
		return
	}
	it := a.item(in.ID)
	if !isCloud(it) {
		writeJSON(w, r, map[string]any{"ok": true})
		return
	}
	if !userFrom(r).libOK(it.Lib) {
		jsonErr(w, 403, "sin acceso")
		return
	}
	if busy := a.hub.tune(cloudKey(it), in.Force); busy != nil {
		busyJSON(w, busy, a.maxConn())
		return
	}
	writeJSON(w, r, map[string]any{"ok": true})
}

// ---------- API de administración y de la biblioteca ----------

func (a *App) hCloudCats(w http.ResponseWriter, r *http.Request) {
	c, err := a.cloudCats(r.URL.Query().Get("refresh") == "1")
	if err != nil {
		jsonErr(w, 502, err.Error())
		return
	}
	writeJSON(w, r, map[string]any{"cats": c})
}

func (a *App) hCloudCat(w http.ResponseWriter, r *http.Request) {
	l, err := a.cloudCat(r.PathValue("id"), false)
	if err != nil {
		jsonErr(w, 502, err.Error())
		return
	}
	writeJSON(w, r, map[string]any{"series": l})
}

// cloudLibCards: series de una biblioteca en la nube (categorías elegidas + series sueltas).
func (a *App) cloudLibCards(l Library) ([]Card, int, error) {
	a.cloudDiskLoad()
	seen := map[int]bool{}
	var list []*cSer
	var firstErr error
	// nombre de cada categoría: así la biblioteca respeta las carpetas del proveedor
	catName := map[string]string{}
	if cs, err := a.cloudCats(false); err == nil {
		for _, c := range cs {
			catName[c.ID] = c.N
		}
	}
	// las categorías ya conocidas salen al instante; las que faltan se piden en paralelo (se espera poco y el resto llega después)
	var wg sync.WaitGroup
	var missing []string
	for _, c := range l.Cats {
		cloud.mu.Lock()
		cloudInit()
		_, ok := cloud.byCat[c]
		cloud.mu.Unlock()
		if !ok {
			missing = append(missing, c)
			cloudFail.Delete(c)
			wg.Add(1)
			go func(c string) {
				defer wg.Done()
				if _, err := a.cloudSingle(c); err != nil {
					cloudFail.Store(c, err.Error())
				}
			}(c)
		}
	}
	if len(missing) > 0 {
		done := make(chan struct{})
		go func() { wg.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(2500 * time.Millisecond):
		}
	}
	pending := 0
	for _, c := range l.Cats {
		cloud.mu.Lock()
		sl, ok := cloud.byCat[c]
		cloud.mu.Unlock()
		if !ok {
			if m, bad := cloudFail.Load(c); bad {
				if firstErr == nil {
					firstErr = errors.New(m.(string))
				}
			} else {
				pending++
			}
			continue
		}
		for _, s := range sl {
			if !seen[s.ID] {
				seen[s.ID] = true
				list = append(list, s)
			}
		}
	}
	for _, id := range l.Sers {
		if seen[id] {
			continue
		}
		cloud.mu.Lock()
		cloudInit()
		s := cloud.sers[id]
		cloud.mu.Unlock()
		if s == nil {
			// tras reiniciar no se conoce la serie suelta: se pide su ficha (el nombre y la carátula salen de ahí)
			if i, err := a.cloudSeriesInfo(id); err == nil && i != nil {
				s = &cSer{ID: id, Name: i.Name, Cover: i.Cover, Year: i.Year}
				cloud.mu.Lock()
				cloud.sers[id] = s
				cloud.mu.Unlock()
			}
		}
		if s != nil {
			seen[id] = true
			list = append(list, s)
		}
	}
	if len(list) == 0 && pending == 0 && firstErr != nil {
		return nil, 0, firstErr
	}
	sort.SliceStable(list, func(i, j int) bool { return strings.ToLower(list[i].Name) < strings.ToLower(list[j].Name) })
	cards := make([]Card, 0, len(list))
	for _, s := range list {
		id := "x" + strconv.Itoa(s.ID)
		co := catName[s.Cat]
		if co == "" {
			co = "Otras series"
		}
		cards = append(cards, Card{T: "s", ID: id, Th: id, SA: true, Ti: s.Name, Y: s.Year, Co: co})
	}
	return cards, pending, nil
}

// cloudCover devuelve la carátula de una serie ("x<id>") si se conoce.
func cloudCover(id string) string {
	n, err := strconv.Atoi(strings.TrimPrefix(id, "x"))
	if err != nil {
		return ""
	}
	cloud.mu.Lock()
	defer cloud.mu.Unlock()
	cloudInit()
	if i := cloud.info[n]; i != nil && i.Cover != "" {
		return i.Cover
	}
	if s := cloud.sers[n]; s != nil {
		return s.Cover
	}
	return ""
}
