package main

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
)

// ---------- tipos flexibles (los paneles Xtream mezclan números y cadenas) ----------

type flexInt int

func (f *flexInt) UnmarshalJSON(b []byte) error {
	s := strings.Trim(strings.TrimSpace(string(b)), `"`)
	if s == "" || s == "null" {
		*f = 0
		return nil
	}
	if n, err := strconv.Atoi(s); err == nil {
		*f = flexInt(n)
		return nil
	}
	if fl, err := strconv.ParseFloat(s, 64); err == nil {
		*f = flexInt(fl)
		return nil
	}
	*f = 0
	return nil
}

type flexStr string

func (f *flexStr) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "null" {
		*f = ""
		return nil
	}
	if strings.HasPrefix(s, `"`) {
		var v string
		if err := json.Unmarshal(b, &v); err != nil {
			return err
		}
		*f = flexStr(v)
		return nil
	}
	*f = flexStr(s)
	return nil
}

// ---------- datos ----------

type Category struct {
	ID   string `json:"id"`
	Name string `json:"n"`
}

type Channel struct {
	ID   int    `json:"id"`
	Name string `json:"n"`
	Cat  string `json:"c"`
	Icon string `json:"i,omitempty"`
	EPG  string `json:"e,omitempty"`
}

type Catalog struct {
	Updated  int64      `json:"updated"`
	Cats     []Category `json:"cats"`
	Channels []Channel  `json:"channels"`
}

type Prog struct {
	S int64  `json:"s"`
	E int64  `json:"e"`
	T string `json:"t"`
	D string `json:"d,omitempty"`
}

type epgFile struct {
	At    int64             `json:"at"`
	Progs map[string][]Prog `json:"progs"`
}

func (a *App) reindexChannels() {
	m := make(map[int]*Channel, len(a.cat.Channels))
	for i := range a.cat.Channels {
		m[a.cat.Channels[i].ID] = &a.cat.Channels[i]
	}
	a.chans = m
}

// ---------- cliente ----------

type xc struct{ base, user, pass, ua string }

const defaultUA = "VLC/3.0.20 LibVLC/3.0.20"

func newXC(c XCfg) *xc {
	base := strings.TrimRight(strings.TrimSpace(c.URL), "/")
	if base != "" && !strings.Contains(base, "://") {
		base = "http://" + base
	}
	for _, suf := range []string{"/player_api.php", "/get.php", "/xmltv.php", "/panel_api.php"} {
		base = strings.TrimSuffix(base, suf)
	}
	ua := c.UA
	if ua == "" {
		ua = defaultUA
	}
	return &xc{base, c.User, c.Pass, ua}
}

func (a *App) xclient() (*xc, error) {
	a.mu.RLock()
	c := a.cfg.Xtream
	a.mu.RUnlock()
	if c.URL == "" || c.User == "" {
		return nil, errors.New("Xtream no está configurado")
	}
	return newXC(c), nil
}

func (c *xc) apiURL(action string) string {
	q := url.Values{"username": {c.user}, "password": {c.pass}}
	if action != "" {
		q.Set("action", action)
	}
	return c.base + "/player_api.php?" + q.Encode()
}

func (c *xc) liveURL(id int) string {
	return fmt.Sprintf("%s/live/%s/%s/%d.ts", c.base, url.PathEscape(c.user), url.PathEscape(c.pass), id)
}

// cleanErr evita que las credenciales (que van en la URL) acaben en mensajes de error.
func cleanErr(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		err = ue.Err
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return errors.New("tiempo de espera agotado")
	}
	return errors.New(err.Error())
}

var (
	apiClient = &http.Client{Timeout: 90 * time.Second}
	epgClient = &http.Client{Timeout: 8 * time.Minute}
)

func (c *xc) get(ctx context.Context, u string, cl *http.Client) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, errors.New("URL no válida")
	}
	req.Header.Set("User-Agent", c.ua)
	resp, err := cl.Do(req)
	if err != nil {
		return nil, cleanErr(err)
	}
	if resp.StatusCode != 200 {
		resp.Body.Close()
		return nil, fmt.Errorf("el servidor respondió HTTP %d", resp.StatusCode)
	}
	return resp, nil
}

func (c *xc) getJSON(ctx context.Context, u string, out any) error {
	resp, err := c.get(ctx, u, apiClient)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return errors.New("respuesta no válida del servidor (¿URL, usuario o contraseña incorrectos?)")
	}
	return nil
}

func (c *xc) userInfo(ctx context.Context) (XInfo, error) {
	var r struct {
		UI struct {
			Auth   flexInt `json:"auth"`
			Status flexStr `json:"status"`
			Exp    flexStr `json:"exp_date"`
			Active flexInt `json:"active_cons"`
			Max    flexInt `json:"max_connections"`
		} `json:"user_info"`
	}
	if err := c.getJSON(ctx, c.apiURL(""), &r); err != nil {
		return XInfo{}, err
	}
	if r.UI.Auth == 0 && !strings.EqualFold(string(r.UI.Status), "active") {
		return XInfo{}, errors.New("usuario o contraseña incorrectos")
	}
	exp, _ := strconv.ParseInt(string(r.UI.Exp), 10, 64)
	return XInfo{Status: string(r.UI.Status), Exp: exp, Active: int(r.UI.Active), Max: int(r.UI.Max), Checked: time.Now().Unix()}, nil
}

// ---------- catálogo ----------

var xBusy atomic.Bool
var xErr atomic.Value // string

func (a *App) refreshCatalog() error {
	c, err := a.xclient()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	var cats []struct {
		ID   flexStr `json:"category_id"`
		Name flexStr `json:"category_name"`
	}
	if err := c.getJSON(ctx, c.apiURL("get_live_categories"), &cats); err != nil {
		return err
	}
	var ss []struct {
		ID   flexInt `json:"stream_id"`
		Name flexStr `json:"name"`
		Cat  flexStr `json:"category_id"`
		Icon flexStr `json:"stream_icon"`
		EPG  flexStr `json:"epg_channel_id"`
	}
	if err := c.getJSON(ctx, c.apiURL("get_live_streams"), &ss); err != nil {
		return err
	}
	cat := Catalog{Updated: time.Now().Unix()}
	for _, x := range cats {
		cat.Cats = append(cat.Cats, Category{string(x.ID), strings.TrimSpace(string(x.Name))})
	}
	for _, s := range ss {
		if s.ID == 0 {
			continue
		}
		cat.Channels = append(cat.Channels, Channel{ID: int(s.ID), Name: strings.TrimSpace(string(s.Name)),
			Cat: string(s.Cat), Icon: string(s.Icon), EPG: string(s.EPG)})
	}
	info, ierr := c.userInfo(ctx)
	a.mu.Lock()
	a.cat = cat
	a.reindexChannels()
	if ierr == nil {
		a.cfg.Xtream.Info = info
	}
	a.mu.Unlock()
	b, _ := json.Marshal(cat)
	a.writeFile("xlive.json", b)
	a.saveConfig()
	return nil
}

// ---------- EPG (XMLTV en streaming: memoria constante) ----------

type latin1 struct {
	r   io.Reader
	buf []byte
}

func (l *latin1) Read(p []byte) (int, error) {
	if len(p) < 2 {
		return 0, io.ErrShortBuffer
	}
	if cap(l.buf) < len(p)/2 {
		l.buf = make([]byte, len(p)/2)
	}
	tmp := l.buf[:len(p)/2]
	n, err := l.r.Read(tmp)
	j := 0
	for _, b := range tmp[:n] {
		if b < 0x80 {
			p[j] = b
			j++
		} else {
			p[j] = 0xC0 | b>>6
			p[j+1] = 0x80 | b&0x3F
			j += 2
		}
	}
	return j, err
}

func charsetReader(label string, in io.Reader) (io.Reader, error) {
	switch strings.ToLower(label) {
	case "iso-8859-1", "latin1", "windows-1252", "iso-8859-15":
		return &latin1{r: in}, nil
	}
	return in, nil
}

func parseXT(s string) int64 {
	s = strings.TrimSpace(s)
	for _, l := range []string{"20060102150405 -0700", "20060102150405"} {
		if t, err := time.Parse(l, s); err == nil {
			return t.Unix()
		}
	}
	return 0
}

func (a *App) refreshEPG() error {
	if !a.epgRun.TryLock() {
		return nil
	}
	defer a.epgRun.Unlock()
	c, err := a.xclient()
	if err != nil {
		return err
	}
	a.mu.RLock()
	want := map[string]bool{}
	for _, id := range a.cfg.Selected {
		if ch := a.chans[id]; ch != nil && ch.EPG != "" {
			want[ch.EPG] = true
		}
	}
	a.mu.RUnlock()
	if len(want) == 0 {
		return nil
	}
	q := url.Values{"username": {c.user}, "password": {c.pass}}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	resp, err := c.get(ctx, c.base+"/xmltv.php?"+q.Encode(), epgClient)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	dec := xml.NewDecoder(resp.Body)
	dec.Strict = false
	dec.CharsetReader = charsetReader
	now := time.Now()
	lo, hi := now.Add(-2*time.Hour).Unix(), now.Add(48*time.Hour).Unix()
	progs := map[string][]Prog{}
	for {
		tok, err := dec.Token()
		if err != nil {
			if err != io.EOF && len(progs) == 0 {
				return errors.New("XMLTV no válido: " + cleanErr(err).Error())
			}
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "programme" {
			continue
		}
		var ch, start, stop string
		for _, at := range se.Attr {
			switch at.Name.Local {
			case "channel":
				ch = at.Value
			case "start":
				start = at.Value
			case "stop":
				stop = at.Value
			}
		}
		if !want[ch] {
			_ = dec.Skip()
			continue
		}
		var p struct {
			Title []struct {
				V string `xml:",chardata"`
			} `xml:"title"`
			Desc []struct {
				V string `xml:",chardata"`
			} `xml:"desc"`
		}
		if err := dec.DecodeElement(&p, &se); err != nil {
			continue
		}
		s, e := parseXT(start), parseXT(stop)
		if s == 0 {
			continue
		}
		if e <= s {
			e = s + 1800
		}
		if e < lo || s > hi {
			continue
		}
		pr := Prog{S: s, E: e}
		if len(p.Title) > 0 {
			pr.T = strings.TrimSpace(p.Title[0].V)
		}
		if len(p.Desc) > 0 {
			d := strings.TrimSpace(p.Desc[0].V)
			if r := []rune(d); len(r) > 220 {
				d = string(r[:220]) + "…"
			}
			pr.D = d
		}
		progs[ch] = append(progs[ch], pr)
	}
	for k := range progs {
		l := progs[k]
		sort.Slice(l, func(i, j int) bool { return l[i].S < l[j].S })
	}
	a.mu.Lock()
	a.epg = progs
	a.epgAt = time.Now().Unix()
	at := a.epgAt
	a.mu.Unlock()
	b, _ := json.Marshal(epgFile{At: at, Progs: progs})
	a.writeFile("epg.json", b)
	return nil
}

// nowNext devuelve el programa actual y el siguiente (a.mu en lectura).
func (a *App) nowNext(epgID string) (*Prog, *Prog) {
	l := a.epg[epgID]
	if len(l) == 0 {
		return nil, nil
	}
	now := time.Now().Unix()
	for i := range l {
		if l[i].E > now {
			if l[i].S <= now {
				var nx *Prog
				if i+1 < len(l) {
					nx = &l[i+1]
				}
				return &l[i], nx
			}
			return nil, &l[i]
		}
	}
	return nil, nil
}

// ---------- vistas ----------

type NowNext struct {
	T string `json:"t"`
	S int64  `json:"s"`
	E int64  `json:"e"`
}

type ChanView struct {
	ID   int      `json:"id"`
	N    string   `json:"n"`
	C    string   `json:"c"`
	Cn   string   `json:"cn,omitempty"`
	F    bool     `json:"f,omitempty"`
	Ic   bool     `json:"ic,omitempty"`
	V    int      `json:"v,omitempty"`
	Now  *NowNext `json:"now,omitempty"`
	Next *NowNext `json:"next,omitempty"`
}

func toNN(p *Prog) *NowNext {
	if p == nil {
		return nil
	}
	return &NowNext{p.T, p.S, p.E}
}

// view construye la vista de un canal (a.mu en lectura).
func (a *App) view(ch *Channel, favs map[int]bool, viewers map[int]int, catNames map[string]string) ChanView {
	now, next := a.nowNext(ch.EPG)
	return ChanView{ID: ch.ID, N: ch.Name, C: ch.Cat, Cn: catNames[ch.Cat], F: favs[ch.ID], Ic: ch.Icon != "",
		V: viewers[ch.ID], Now: toNN(now), Next: toNN(next)}
}

func (a *App) catNames() map[string]string {
	m := map[string]string{}
	for _, c := range a.cat.Cats {
		m[c.ID] = c.Name
	}
	return m
}

func (a V) chanViews(favFirst bool, limit int) []ChanView {
	viewers := a.hub.viewerMap() // antes de tomar a.mu (orden de bloqueos)
	a.mu.RLock()
	defer a.mu.RUnlock()
	favs := map[int]bool{}
	for _, id := range a.favList() {
		favs[id] = true
	}
	names := a.catNames()
	out := []ChanView{}
	for pass := 0; pass < 2; pass++ {
		for _, id := range a.cfg.Selected {
			ch := a.chans[id]
			if ch == nil {
				continue
			}
			if favFirst && (pass == 0) != favs[id] {
				continue
			}
			if !favFirst && pass == 1 {
				continue
			}
			out = append(out, a.view(ch, favs, viewers, names))
			if limit > 0 && len(out) >= limit {
				return out
			}
		}
		if !favFirst {
			break
		}
	}
	return out
}

func (a V) searchChannels(q string, limit int) []ChanView {
	out := []ChanView{}
	if len(q) < 2 {
		return out
	}
	viewers := a.hub.viewerMap()
	a.mu.RLock()
	defer a.mu.RUnlock()
	favs := map[int]bool{}
	for _, id := range a.favList() {
		favs[id] = true
	}
	names := a.catNames()
	for _, id := range a.cfg.Selected {
		if ch := a.chans[id]; ch != nil && strings.Contains(strings.ToLower(ch.Name), q) {
			out = append(out, a.view(ch, favs, viewers, names))
			if len(out) >= limit {
				break
			}
		}
	}
	return out
}

// ---------- manejadores ----------

func (a *App) xtreamStatus() map[string]any {
	a.mu.RLock()
	defer a.mu.RUnlock()
	x := a.cfg.Xtream
	errS, _ := xErr.Load().(string)
	return map[string]any{
		"url": x.URL, "user": x.User, "hasPass": x.Pass != "", "maxConn": x.MaxConn, "ua": x.UA, "info": x.Info,
		"channels": len(a.cat.Channels), "cats": len(a.cat.Cats), "catAt": a.cat.Updated,
		"selected": len(a.cfg.Selected), "epgAt": a.epgAt, "epgChannels": len(a.epg),
		"busy": xBusy.Load(), "error": errS, "effectiveMax": func() int {
			if x.MaxConn > 0 {
				return x.MaxConn
			}
			if x.Info.Max > 0 {
				return x.Info.Max
			}
			return 1
		}(),
	}
}

func (a *App) hXtreamGet(w http.ResponseWriter, r *http.Request) { writeJSON(w, r, a.xtreamStatus()) }

func (a *App) kickRefresh() {
	if !xBusy.CompareAndSwap(false, true) {
		return
	}
	xErr.Store("")
	go func() {
		defer xBusy.Store(false)
		if err := a.refreshCatalog(); err != nil {
			xErr.Store(err.Error())
			return
		}
		if err := a.refreshEPG(); err != nil {
			xErr.Store("Lista de canales OK, pero la guía (EPG) falló: " + err.Error())
		}
	}()
}

func (a *App) hXtreamSave(w http.ResponseWriter, r *http.Request) {
	var in struct {
		URL     string `json:"url"`
		User    string `json:"user"`
		Pass    string `json:"pass"`
		MaxConn int    `json:"maxConn"`
		UA      string `json:"ua"`
	}
	if err := readJSON(r, &in); err != nil {
		jsonErr(w, 400, "datos no válidos")
		return
	}
	in.URL = strings.TrimSpace(in.URL)
	// permite pegar la URL M3U completa: http://host:puerto/get.php?username=..&password=..
	if u, err := url.Parse(in.URL); err == nil && u.Host != "" && u.RawQuery != "" {
		if in.User == "" {
			in.User = u.Query().Get("username")
		}
		if in.Pass == "" {
			in.Pass = u.Query().Get("password")
		}
		in.URL = u.Scheme + "://" + u.Host
	}
	a.mu.RLock()
	old := a.cfg.Xtream
	a.mu.RUnlock()
	if in.Pass == "" && in.User == old.User {
		in.Pass = old.Pass
	}
	nc := XCfg{URL: in.URL, User: strings.TrimSpace(in.User), Pass: in.Pass, MaxConn: in.MaxConn, UA: strings.TrimSpace(in.UA)}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	info, err := newXC(nc).userInfo(ctx)
	if err != nil {
		jsonErr(w, 400, "No se pudo conectar: "+err.Error())
		return
	}
	nc.Info = info
	a.mu.Lock()
	changed := old.URL != nc.URL || old.User != nc.User
	a.cfg.Xtream = nc
	a.mu.Unlock()
	a.saveConfig()
	a.mu.RLock()
	noCat := len(a.cat.Channels) == 0
	a.mu.RUnlock()
	if changed || noCat {
		a.kickRefresh()
	}
	writeJSON(w, r, a.xtreamStatus())
}

func (a *App) hXtreamRefresh(w http.ResponseWriter, r *http.Request) {
	a.kickRefresh()
	writeJSON(w, r, a.xtreamStatus())
}

func (a *App) hXtreamCats(w http.ResponseWriter, r *http.Request) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	sel := map[int]bool{}
	for _, id := range a.cfg.Selected {
		sel[id] = true
	}
	type cv struct {
		ID    string `json:"id"`
		N     string `json:"n"`
		Total int    `json:"total"`
		Sel   int    `json:"sel"`
	}
	idx := map[string]*cv{}
	var out []*cv
	for _, c := range a.cat.Cats {
		v := &cv{ID: c.ID, N: c.Name}
		idx[c.ID] = v
		out = append(out, v)
	}
	for _, ch := range a.cat.Channels {
		v := idx[ch.Cat]
		if v == nil {
			v = &cv{ID: ch.Cat, N: "Sin categoría"}
			idx[ch.Cat] = v
			out = append(out, v)
		}
		v.Total++
		if sel[ch.ID] {
			v.Sel++
		}
	}
	res := []*cv{}
	for _, v := range out {
		if v.Total > 0 {
			res = append(res, v)
		}
	}
	writeJSON(w, r, map[string]any{"cats": res, "total": len(a.cat.Channels), "selected": len(a.cfg.Selected)})
}

// matchChannels filtra el catálogo (a.mu en lectura).
func (a *App) matchChannels(cat, q string, onlySel bool) []*Channel {
	toks := strings.Fields(strings.ToLower(q))
	sel := map[int]bool{}
	for _, id := range a.cfg.Selected {
		sel[id] = true
	}
	ok := func(ch *Channel) bool {
		if cat != "" && ch.Cat != cat {
			return false
		}
		if onlySel && !sel[ch.ID] {
			return false
		}
		n := strings.ToLower(ch.Name)
		for _, t := range toks {
			if !strings.Contains(n, t) {
				return false
			}
		}
		return true
	}
	var out []*Channel
	if onlySel {
		for _, id := range a.cfg.Selected {
			if ch := a.chans[id]; ch != nil && ok(ch) {
				out = append(out, ch)
			}
		}
		return out
	}
	for i := range a.cat.Channels {
		if ch := &a.cat.Channels[i]; ok(ch) {
			out = append(out, ch)
		}
	}
	return out
}

func (app *App) hXtreamChannels(w http.ResponseWriter, r *http.Request) {
	a := app.V(r)
	qv := r.URL.Query()
	off, _ := strconv.Atoi(qv.Get("off"))
	lim, _ := strconv.Atoi(qv.Get("lim"))
	if lim <= 0 || lim > 300 {
		lim = 100
	}
	a.mu.RLock()
	list := a.matchChannels(qv.Get("cat"), qv.Get("q"), qv.Get("sel") == "1")
	sel := map[int]bool{}
	for _, id := range a.cfg.Selected {
		sel[id] = true
	}
	favs := map[int]bool{}
	for _, id := range a.favList() {
		favs[id] = true
	}
	type cv struct {
		ID int    `json:"id"`
		N  string `json:"n"`
		C  string `json:"c"`
		S  bool   `json:"s"`
		F  bool   `json:"f,omitempty"`
		Ic bool   `json:"ic,omitempty"`
		E  bool   `json:"e,omitempty"`
	}
	total := len(list)
	items := []cv{}
	for i := off; i < total && i < off+lim; i++ {
		ch := list[i]
		items = append(items, cv{ch.ID, ch.Name, ch.Cat, sel[ch.ID], favs[ch.ID], ch.Icon != "", ch.EPG != ""})
	}
	a.mu.RUnlock()
	writeJSON(w, r, map[string]any{"total": total, "items": items})
}

func (a *App) epgSoon() { a.later("epg", 20*time.Second, func() { _ = a.refreshEPG() }) }

func (a *App) hXtreamSelect(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs []int `json:"ids"`
		On  bool  `json:"on"`
	}
	if err := readJSON(r, &in); err != nil {
		jsonErr(w, 400, "datos no válidos")
		return
	}
	a.applySelect(in.IDs, in.On)
	writeJSON(w, r, map[string]any{"selected": len(a.cfg.Selected)})
}

func (a *App) applySelect(ids []int, on bool) {
	a.mu.Lock()
	cur := map[int]bool{}
	for _, id := range a.cfg.Selected {
		cur[id] = true
	}
	if on {
		for _, id := range ids {
			if !cur[id] && a.chans[id] != nil {
				cur[id] = true
				a.cfg.Selected = append(a.cfg.Selected, id)
			}
		}
	} else {
		rm := map[int]bool{}
		for _, id := range ids {
			rm[id] = true
		}
		kept := a.cfg.Selected[:0:0]
		for _, id := range a.cfg.Selected {
			if !rm[id] {
				kept = append(kept, id)
			}
		}
		a.cfg.Selected = kept
	}
	a.mu.Unlock()
	a.saveConfig()
	a.epgSoon()
}

func (a *App) hXtreamSelectQuery(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Cat string `json:"cat"`
		Q   string `json:"q"`
		On  bool   `json:"on"`
	}
	if err := readJSON(r, &in); err != nil {
		jsonErr(w, 400, "datos no válidos")
		return
	}
	a.mu.RLock()
	list := a.matchChannels(in.Cat, in.Q, false)
	ids := make([]int, 0, len(list))
	for _, ch := range list {
		if len(ids) >= 5000 {
			break
		}
		ids = append(ids, ch.ID)
	}
	a.mu.RUnlock()
	a.applySelect(ids, in.On)
	writeJSON(w, r, map[string]any{"changed": len(ids)})
}

func (a *App) hXtreamMove(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID int    `json:"id"`
		To string `json:"to"`
	}
	if err := readJSON(r, &in); err != nil {
		jsonErr(w, 400, "datos no válidos")
		return
	}
	a.mu.Lock()
	s := a.cfg.Selected
	i := -1
	for k, id := range s {
		if id == in.ID {
			i = k
		}
	}
	if i >= 0 {
		j := i
		switch in.To {
		case "up":
			j = i - 1
		case "down":
			j = i + 1
		case "top":
			j = 0
		case "bottom":
			j = len(s) - 1
		}
		if j >= 0 && j < len(s) && j != i {
			v := s[i]
			s = append(s[:i], s[i+1:]...)
			s = append(s[:j], append([]int{v}, s[j:]...)...)
			a.cfg.Selected = s
		}
	}
	a.mu.Unlock()
	a.saveConfig()
	w.WriteHeader(204)
}

func (app *App) hXtreamFav(w http.ResponseWriter, r *http.Request) {
	a := app.V(r)
	var in struct {
		ID int  `json:"id"`
		On bool `json:"on"`
	}
	if err := readJSON(r, &in); err != nil {
		jsonErr(w, 400, "datos no válidos")
		return
	}
	a.mu.Lock()
	kept := a.cfg.Favs[:0:0]
	for _, id := range a.favList() {
		if id != in.ID {
			kept = append(kept, id)
		}
	}
	if in.On {
		kept = append(kept, in.ID)
	}
	a.setFavs(kept)
	a.mu.Unlock()
	a.saveConfig()
	w.WriteHeader(204)
}

func (app *App) hLiveChannels(w http.ResponseWriter, r *http.Request) {
	a := app.V(r)
	chans := a.chanViews(false, 0)
	a.mu.RLock()
	used := map[string]bool{}
	for _, c := range chans {
		used[c.C] = true
	}
	cats := []Category{}
	for _, c := range a.cat.Cats {
		if used[c.ID] {
			cats = append(cats, c)
		}
	}
	a.mu.RUnlock()
	writeJSON(w, r, map[string]any{"chans": chans, "cats": cats, "max": a.maxConn()})
}

func (a *App) hLiveEPG(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.PathValue("id"))
	out := []Prog{}
	a.mu.RLock()
	if ch := a.chans[id]; ch != nil {
		now := time.Now().Unix()
		for _, p := range a.epg[ch.EPG] {
			if p.E > now && len(out) < 14 {
				out = append(out, p)
			}
		}
	}
	a.mu.RUnlock()
	writeJSON(w, r, map[string]any{"progs": out})
}

func (a *App) hLiveTune(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID    int  `json:"id"`
		Force bool `json:"force"`
	}
	if err := readJSON(r, &in); err != nil {
		jsonErr(w, 400, "datos no válidos")
		return
	}
	if busy := a.hub.tune(in.ID, in.Force); busy != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(409)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "busy", "playing": busy, "max": a.maxConn()})
		return
	}
	writeJSON(w, r, map[string]any{"ok": true})
}

func (a *App) hLiveWarm(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID int `json:"id"`
	}
	if err := readJSON(r, &in); err != nil {
		jsonErr(w, 400, "datos no válidos")
		return
	}
	writeJSON(w, r, map[string]any{"ok": a.hub.warm(in.ID)})
}

func (a *App) hLiveStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, r, map[string]any{"streams": a.hub.status(), "max": a.maxConn()})
}

// ---------- logos en caché ----------

var logoClient = &http.Client{
	Timeout: 10 * time.Second,
	Transport: &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		TLSHandshakeTimeout: 6 * time.Second,
		DialContext: (&net.Dialer{
			Timeout: 6 * time.Second,
			Control: func(network, address string, _ syscall.RawConn) error {
				host, _, _ := net.SplitHostPort(address)
				if ip := net.ParseIP(host); ip != nil && (ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified()) {
					return errors.New("destino no permitido")
				}
				return nil
			},
		}).DialContext,
	},
}

func (a *App) hLogo(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.PathValue("id"))
	a.mu.RLock()
	ch := a.chans[id]
	var icon string
	if ch != nil {
		icon = ch.Icon
	}
	a.mu.RUnlock()
	if icon == "" {
		http.NotFound(w, r)
		return
	}
	p := filepath.Join(a.dir, "logos", strconv.Itoa(id))
	st, err := os.Stat(p)
	if err != nil || time.Since(st.ModTime()) > 14*24*time.Hour {
		if t, ok := a.logoNeg.Load(id); ok && time.Since(t.(time.Time)) < time.Hour {
			http.NotFound(w, r)
			return
		}
		if err := a.fetchLogo(icon, p); err != nil {
			a.logoNeg.Store(id, time.Now())
			if _, e2 := os.Stat(p); e2 != nil {
				http.NotFound(w, r)
				return
			}
		}
	}
	f, err := os.Open(p)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	head := make([]byte, 512)
	n, _ := f.Read(head)
	ct := http.DetectContentType(head[:n])
	if strings.Contains(string(head[:n]), "<svg") {
		ct = "image/svg+xml"
	}
	_, _ = f.Seek(0, 0)
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "public, max-age=604800")
	// los logos vienen del proveedor: un SVG con scripts no debe poder ejecutarse en el origen de Lumina
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
	st2, _ := f.Stat()
	http.ServeContent(w, r, "", st2.ModTime(), f)
}

func (a *App) fetchLogo(icon, dst string) error {
	u, err := url.Parse(icon)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errors.New("URL de logo no válida")
	}
	a.logoSem <- struct{}{}
	defer func() { <-a.logoSem }()
	req, _ := http.NewRequest("GET", icon, nil)
	req.Header.Set("User-Agent", defaultUA)
	resp, err := logoClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil || len(b) == 0 {
		return errors.New("logo vacío")
	}
	head := b
	if len(head) > 512 {
		head = head[:512]
	}
	if ct := http.DetectContentType(head); !strings.HasPrefix(ct, "image/") && !strings.Contains(string(head), "<svg") {
		return errors.New("no es una imagen")
	}
	tmp := dst + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}
