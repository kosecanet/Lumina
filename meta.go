package main

// Información en castellano de películas y series (sinopsis, géneros, reparto…).
// Fuente principal: TMDB (hace falta una clave gratuita, se pone en Ajustes → Sistema).
// Sin clave se usa la Wikipedia en español, que solo da la sinopsis. Todo se guarda en caché (meta.json).

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Meta struct {
	Title    string   `json:"title,omitempty"`
	Overview string   `json:"overview,omitempty"`
	Tagline  string   `json:"tagline,omitempty"`
	Genres   []string `json:"genres,omitempty"`
	Rating   float64  `json:"rating,omitempty"`
	Runtime  int      `json:"runtime,omitempty"`
	Year     string   `json:"year,omitempty"`
	Cast     []string `json:"cast,omitempty"`
	Director string   `json:"director,omitempty"` // en series: creadores
	Source   string   `json:"source,omitempty"`   // TMDB | Wikipedia
	Backdrop string   `json:"bd,omitempty"`       // imagen de fondo grande (TMDB, 1280 px)
	Img      int      `json:"img,omitempty"`      // 1: ya se buscó la imagen de fondo
	At       int64    `json:"at"`
}

var metaState struct {
	mu     sync.Mutex // serializa las consultas y protege el mapa
	loaded bool
	found  map[string]*Meta
	miss   map[string]int64
}

var metaClient = &http.Client{Timeout: 9 * time.Second, Transport: &http.Transport{
	Proxy: http.ProxyFromEnvironment, TLSHandshakeTimeout: 6 * time.Second, ResponseHeaderTimeout: 8 * time.Second,
}}

func metaKey(kind, title string, year int) string {
	return kind + "|" + strings.ToLower(strings.TrimSpace(title)) + "|" + fmt.Sprint(year)
}

func (a *App) metaLoadLocked() {
	if metaState.loaded {
		return
	}
	metaState.loaded = true
	metaState.found = map[string]*Meta{}
	metaState.miss = map[string]int64{}
	if b, err := os.ReadFile(filepath.Join(a.dir, "meta.json")); err == nil {
		_ = json.Unmarshal(b, &metaState.found)
	}
}

func (a *App) metaSaveLocked() {
	b, _ := json.Marshal(metaState.found)
	a.writeFile("meta.json", b)
}

func (a *App) tmdbKey() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return strings.TrimSpace(a.cfg.TMDBKey)
}

// metaFor busca (o recupera de la caché) la ficha de una película (kind "movie") o serie ("tv").
func (a *App) metaFor(kind, title string, year int) *Meta {
	key := metaKey(kind, title, year)
	metaState.mu.Lock()
	defer metaState.mu.Unlock()
	a.metaLoadLocked()
	if m := metaState.found[key]; m != nil && (m.Source != "TMDB" || m.Img == 1) {
		return m
	}
	old := metaState.found[key] // ficha de TMDB guardada antes de existir las imágenes de fondo: se vuelve a pedir una vez
	if t := metaState.miss[key]; t > 0 && time.Now().Unix()-t < 6*3600 {
		return nil
	}
	var m *Meta
	if k := a.tmdbKey(); k != "" {
		m, _ = tmdbFetch(k, kind, title, year)
	}
	if m == nil && old != nil {
		old.Img = 1
		a.metaSaveLocked()
		return old
	}
	if m == nil { // sin clave, o TMDB sin resultado: Wikipedia en español
		m, _ = wikiFetch(kind, title, year)
	}
	if m == nil {
		metaState.miss[key] = time.Now().Unix()
		return nil
	}
	m.At = time.Now().Unix()
	metaState.found[key] = m
	a.metaSaveLocked()
	return m
}

func getJSON(u string, hdr map[string]string, out any) error {
	req, _ := http.NewRequest("GET", u, nil)
	req.Header.Set("User-Agent", "Lumina/1.0 (+https://github.com/kosecanet/Lumina)")
	req.Header.Set("Accept", "application/json")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	r, err := metaClient.Do(req)
	if err != nil {
		return err
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", r.StatusCode)
	}
	return json.NewDecoder(r.Body).Decode(out)
}

// ---------- TMDB ----------

func tmdbURL(key, path string, q url.Values) (string, map[string]string) {
	if q == nil {
		q = url.Values{}
	}
	q.Set("language", "es-ES")
	hdr := map[string]string{}
	if len(key) > 40 { // token de lectura (v4)
		hdr["Authorization"] = "Bearer " + key
	} else {
		q.Set("api_key", key)
	}
	return "https://api.themoviedb.org/3" + path + "?" + q.Encode(), hdr
}

func tmdbCheck(key string) error {
	u, h := tmdbURL(key, "/configuration", nil)
	var x map[string]any
	if err := getJSON(u, h, &x); err != nil {
		if strings.Contains(err.Error(), "401") {
			return errors.New("TMDB no acepta esa clave")
		}
		return fmt.Errorf("no se pudo consultar TMDB: %v", err)
	}
	return nil
}

func tmdbFetch(key, kind, title string, year int) (*Meta, error) {
	type res struct {
		Results []struct {
			ID int `json:"id"`
		} `json:"results"`
	}
	search := func(y int) (int, error) {
		q := url.Values{"query": {title}, "include_adult": {"false"}}
		if y > 0 {
			if kind == "tv" {
				q.Set("first_air_date_year", fmt.Sprint(y))
			} else {
				q.Set("year", fmt.Sprint(y))
			}
		}
		u, h := tmdbURL(key, "/search/"+kind, q)
		var r res
		if err := getJSON(u, h, &r); err != nil {
			return 0, err
		}
		if len(r.Results) == 0 {
			return 0, nil
		}
		return r.Results[0].ID, nil
	}
	id, err := search(year)
	if err != nil {
		return nil, err
	}
	if id == 0 && year > 0 {
		if id, err = search(0); err != nil {
			return nil, err
		}
	}
	if id == 0 {
		return nil, nil
	}
	u, h := tmdbURL(key, fmt.Sprintf("/%s/%d", kind, id), url.Values{"append_to_response": {"credits"}})
	var d struct {
		Title        string  `json:"title"`
		Name         string  `json:"name"`
		Overview     string  `json:"overview"`
		Tagline      string  `json:"tagline"`
		Vote         float64 `json:"vote_average"`
		Runtime      int     `json:"runtime"`
		EpRuntime    []int   `json:"episode_run_time"`
		ReleaseDate  string  `json:"release_date"`
		FirstAirDate string  `json:"first_air_date"`
		Genres       []struct {
			Name string `json:"name"`
		} `json:"genres"`
		Backdrop  string `json:"backdrop_path"`
		CreatedBy []struct {
			Name string `json:"name"`
		} `json:"created_by"`
		Credits struct {
			Cast []struct {
				Name string `json:"name"`
			} `json:"cast"`
			Crew []struct {
				Name string `json:"name"`
				Job  string `json:"job"`
			} `json:"crew"`
		} `json:"credits"`
	}
	if err := getJSON(u, h, &d); err != nil {
		return nil, err
	}
	m := &Meta{Title: d.Title, Overview: strings.TrimSpace(d.Overview), Tagline: strings.TrimSpace(d.Tagline), Rating: float64(int(d.Vote*10+0.5)) / 10, Runtime: d.Runtime, Source: "TMDB", Img: 1}
	if d.Backdrop != "" {
		m.Backdrop = "https://image.tmdb.org/t/p/w1280" + d.Backdrop
	}
	if kind == "tv" {
		m.Title = d.Name
		if len(d.EpRuntime) > 0 {
			m.Runtime = d.EpRuntime[0]
		}
		var cr []string
		for _, c := range d.CreatedBy {
			cr = append(cr, c.Name)
		}
		m.Director = strings.Join(cr, ", ")
	} else {
		var dr []string
		for _, c := range d.Credits.Crew {
			if c.Job == "Director" && len(dr) < 3 {
				dr = append(dr, c.Name)
			}
		}
		m.Director = strings.Join(dr, ", ")
	}
	date := d.ReleaseDate
	if kind == "tv" {
		date = d.FirstAirDate
	}
	if len(date) >= 4 {
		m.Year = date[:4]
	}
	for _, g := range d.Genres {
		m.Genres = append(m.Genres, g.Name)
	}
	for i, c := range d.Credits.Cast {
		if i >= 8 {
			break
		}
		m.Cast = append(m.Cast, c.Name)
	}
	if m.Overview == "" && len(m.Genres) == 0 {
		return nil, nil
	}
	return m, nil
}

// ---------- Wikipedia (sin clave) ----------

func wikiFetch(kind, title string, year int) (*Meta, error) {
	what := " película"
	if kind == "tv" {
		what = " serie de televisión"
	}
	q := url.Values{"action": {"query"}, "list": {"search"}, "srsearch": {title + what + yearStr(year)}, "srlimit": {"3"}, "format": {"json"}, "utf8": {"1"}}
	var s struct {
		Query struct {
			Search []struct {
				Title string `json:"title"`
			} `json:"search"`
		} `json:"query"`
	}
	if err := getJSON("https://es.wikipedia.org/w/api.php?"+q.Encode(), nil, &s); err != nil {
		return nil, err
	}
	for _, hit := range s.Query.Search {
		var p struct {
			Extract string `json:"extract"`
			Titles  struct {
				Display string `json:"display"`
			} `json:"titles"`
		}
		if err := getJSON("https://es.wikipedia.org/api/rest_v1/page/summary/"+url.PathEscape(strings.ReplaceAll(hit.Title, " ", "_")), nil, &p); err != nil {
			continue
		}
		ex := strings.TrimSpace(p.Extract)
		low := strings.ToLower(ex)
		ok := strings.Contains(low, "película") || strings.Contains(low, "largometraje") || strings.Contains(low, "film")
		if kind == "tv" {
			ok = strings.Contains(low, "serie")
		}
		if !ok || ex == "" {
			continue
		}
		return &Meta{Title: hit.Title, Overview: cutSentence(ex, 900), Source: "Wikipedia"}, nil
	}
	return nil, nil
}

func yearStr(y int) string {
	if y > 0 {
		return " " + fmt.Sprint(y)
	}
	return ""
}

func cutSentence(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	cut := string(r[:n])
	if i := strings.LastIndex(cut, ". "); i > n/2 {
		return cut[:i+1]
	}
	return cut + "…"
}

// ---------- API ----------

// GET /api/meta/{id}: ficha del título (o de su serie si es un episodio).
func (a *App) hMeta(w http.ResponseWriter, r *http.Request) {
	it := a.item(r.PathValue("id"))
	if it == nil {
		jsonErr(w, 404, "no encontrado")
		return
	}
	kind, title, year := "movie", it.Title, it.Year
	if it.Kind == "episode" {
		kind, title, year = "tv", it.Series, 0
	}
	if strings.TrimSpace(title) == "" {
		writeJSON(w, r, map[string]any{"meta": nil})
		return
	}
	writeJSON(w, r, map[string]any{"meta": a.metaFor(kind, title, year), "kind": kind})
}

// GET/POST /api/system/meta (solo administradores): clave de TMDB.
func (a *App) hMetaCfg(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var in struct {
			Key string `json:"key"`
		}
		if err := readJSON(r, &in); err != nil {
			jsonErr(w, 400, "datos no válidos")
			return
		}
		k := strings.TrimSpace(in.Key)
		if k != "" {
			if err := tmdbCheck(k); err != nil {
				jsonErr(w, 400, err.Error())
				return
			}
		}
		a.mu.Lock()
		a.cfg.TMDBKey = k
		a.mu.Unlock()
		a.saveConfig()
		metaState.mu.Lock()
		a.metaLoadLocked()
		metaState.miss = map[string]int64{}
		metaState.mu.Unlock()
	}
	a.mu.RLock()
	has := strings.TrimSpace(a.cfg.TMDBKey) != ""
	a.mu.RUnlock()
	writeJSON(w, r, map[string]any{"hasKey": has})
}
