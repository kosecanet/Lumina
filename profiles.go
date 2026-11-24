package main

// Perfiles de usuario (como "¿Quién está viendo?"): cada perfil tiene su propio "seguir viendo",
// visto/no visto y canales favoritos. La biblioteca y los ajustes son comunes.

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

type Profile struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color int    `json:"color"`          // 0..5: color del avatar
	Kind  string `json:"kind,omitempty"` // "u" silueta, "k" estrella (niños)
	Favs  []int  `json:"favs,omitempty"`
}

const maxProfiles = 6

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// initProfiles crea el perfil inicial y migra el progreso y los favoritos de versiones anteriores.
func (a *App) initProfiles() {
	a.progs = map[string]map[string]Progress{}
	a.load("progress2.json", &a.progs)
	if a.progs == nil {
		a.progs = map[string]map[string]Progress{}
	}
	if len(a.cfg.Profiles) == 0 {
		p := Profile{ID: randHex(5), Name: "Perfil", Kind: "u", Favs: a.cfg.Favs}
		a.cfg.Profiles = []Profile{p}
		a.cfg.Favs = nil
		var old map[string]Progress
		a.load("progress.json", &old)
		if len(old) > 0 {
			a.progs[p.ID] = old
		}
		a.saveConfig()
		a.saveProgress()
	}
}

// pidOf devuelve el perfil activo de la petición (cookie "lp"); si no es válido, el primero.
func (a *App) pidOf(r *http.Request) string {
	if u := userFrom(r); u != nil { // con cuentas, el perfil es el de la cuenta
		return u.ID
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	if c, err := r.Cookie("lp"); err == nil {
		for _, p := range a.cfg.Profiles {
			if p.ID == c.Value {
				return p.ID
			}
		}
	}
	return a.cfg.Profiles[0].ID
}

// V es la vista de la aplicación desde un perfil: las funciones que construyen tarjetas y canales
// usan el progreso y los favoritos de ese perfil.
type V struct {
	*App
	pid string
	u   *User // nil = sin cuentas (todo permitido)
}

func (a *App) V(r *http.Request) V { return V{a, a.pidOf(r), userFrom(r)} }

func (v V) libOK(id string) bool { return v.u.libOK(id) }

// pr devuelve el progreso del perfil (el llamador debe tener a.mu).
func (v V) pr() map[string]Progress { return v.progs[v.pid] }

// favList devuelve los favoritos del perfil (el llamador debe tener a.mu).
func (v V) favList() []int {
	for _, p := range v.cfg.Profiles {
		if p.ID == v.pid {
			return p.Favs
		}
	}
	return nil
}

func (v V) setFavs(f []int) {
	for i := range v.cfg.Profiles {
		if v.cfg.Profiles[i].ID == v.pid {
			v.cfg.Profiles[i].Favs = f
		}
	}
}

// ---------- API ----------

type profView struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color int    `json:"color"`
	Kind  string `json:"kind"`
}

func (a *App) hProfiles(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r)
	a.mu.RLock()
	out := make([]profView, 0, len(a.cfg.Profiles))
	for _, p := range a.cfg.Profiles {
		if me != nil && p.ID != me.ID { // con cuentas solo se ve el perfil propio
			continue
		}
		out = append(out, profView{p.ID, p.Name, p.Color, p.Kind})
	}
	a.mu.RUnlock()
	cur := ""
	if me != nil {
		writeJSON(w, r, map[string]any{"profiles": out, "current": me.ID, "max": 1, "accounts": true})
		return
	}
	if c, err := r.Cookie("lp"); err == nil {
		for _, p := range out {
			if p.ID == c.Value {
				cur = c.Value
			}
		}
	}
	writeJSON(w, r, map[string]any{"profiles": out, "current": cur, "max": maxProfiles})
}

func cleanProfile(in *profView) bool {
	in.Name = strings.TrimSpace(in.Name)
	if n := utf8.RuneCountInString(in.Name); n < 1 || n > 20 {
		return false
	}
	if in.Color < 0 || in.Color > 5 {
		in.Color = 0
	}
	if in.Kind != "k" {
		in.Kind = "u"
	}
	return true
}

func (a *App) hProfileCreate(w http.ResponseWriter, r *http.Request) {
	var in profView
	if readJSON(r, &in) != nil || !cleanProfile(&in) {
		jsonErr(w, 400, "escribe un nombre (1-20 caracteres)")
		return
	}
	a.mu.Lock()
	if len(a.cfg.Profiles) >= maxProfiles {
		a.mu.Unlock()
		jsonErr(w, 400, "máximo 6 perfiles")
		return
	}
	p := Profile{ID: randHex(5), Name: in.Name, Color: in.Color, Kind: in.Kind}
	a.cfg.Profiles = append(a.cfg.Profiles, p)
	a.mu.Unlock()
	a.saveConfig()
	writeJSON(w, r, profView{p.ID, p.Name, p.Color, p.Kind})
}

func (a *App) hProfileUpdate(w http.ResponseWriter, r *http.Request) {
	var in profView
	if readJSON(r, &in) != nil || !cleanProfile(&in) {
		jsonErr(w, 400, "escribe un nombre (1-20 caracteres)")
		return
	}
	id := r.PathValue("id")
	ok := false
	a.mu.Lock()
	for i := range a.cfg.Profiles {
		if a.cfg.Profiles[i].ID == id {
			a.cfg.Profiles[i].Name, a.cfg.Profiles[i].Color, a.cfg.Profiles[i].Kind = in.Name, in.Color, in.Kind
			ok = true
		}
	}
	a.mu.Unlock()
	if !ok {
		http.NotFound(w, r)
		return
	}
	a.saveConfig()
	w.WriteHeader(204)
}

func (a *App) hProfileDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	a.mu.Lock()
	if len(a.cfg.Profiles) <= 1 {
		a.mu.Unlock()
		jsonErr(w, 400, "debe quedar al menos un perfil")
		return
	}
	keep := a.cfg.Profiles[:0:0]
	for _, p := range a.cfg.Profiles {
		if p.ID != id {
			keep = append(keep, p)
		}
	}
	found := len(keep) != len(a.cfg.Profiles)
	a.cfg.Profiles = keep
	delete(a.progs, id)
	a.mu.Unlock()
	if !found {
		http.NotFound(w, r)
		return
	}
	a.saveConfig()
	a.saveProgress()
	w.WriteHeader(204)
}

// hProfileSelect fija el perfil activo en una cookie (válida para todos los dispositivos que la guarden).
func (a *App) hProfileSelect(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	a.mu.RLock()
	ok := false
	for _, p := range a.cfg.Profiles {
		ok = ok || p.ID == id
	}
	a.mu.RUnlock()
	if !ok {
		http.NotFound(w, r)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "lp", Value: id, Path: "/", MaxAge: 365 * 24 * 3600, SameSite: http.SameSiteLaxMode, Expires: time.Now().AddDate(1, 0, 0)})
	w.WriteHeader(204)
}
