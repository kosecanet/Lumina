package main

// Cuentas de usuario con contraseña y permisos. Mientras no haya ninguna cuenta, Lumina funciona como antes
// (sin contraseña o con la contraseña global). Al crear la primera cuenta (administrador) se activa el modo cuentas:
// cada cuenta tiene su propio perfil (historial y favoritos) y puede limitarse a ciertas bibliotecas y a la TV en directo.

import (
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type User struct {
	ID      string   `json:"id"` // coincide con el ID de su perfil
	Name    string   `json:"name"`
	Salt    string   `json:"salt"`
	Hash    string   `json:"hash"`
	Admin   bool     `json:"admin,omitempty"`
	AllLibs bool     `json:"allLibs,omitempty"`
	Libs    []string `json:"libs,omitempty"`
	Live    bool     `json:"live,omitempty"`
}

const (
	pbkdfIter  = 120000
	sessionTTL = 90 * 24 * time.Hour
)

type ctxKey struct{}

func hashPass(pass, salt string) string {
	k, err := pbkdf2.Key(sha256.New, pass, []byte(salt), pbkdfIter, 32)
	if err != nil {
		return ""
	}
	return hex.EncodeToString(k)
}

func (u *User) checkPass(pass string) bool {
	h := hashPass(pass, u.Salt)
	return h != "" && subtle.ConstantTimeCompare([]byte(h), []byte(u.Hash)) == 1
}

// libOK indica si el usuario puede ver la biblioteca. Un usuario nil es el modo sin cuentas (todo permitido).
func (u *User) libOK(id string) bool {
	if u == nil || u.Admin || u.AllLibs {
		return true
	}
	for _, l := range u.Libs {
		if l == id {
			return true
		}
	}
	return false
}

func (u *User) liveOK() bool  { return u == nil || u.Admin || u.Live }
func (u *User) isAdmin() bool { return u == nil || u.Admin }

func (a *App) accounts() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return len(a.cfg.Users) > 0
}

func userFrom(r *http.Request) *User {
	u, _ := r.Context().Value(ctxKey{}).(*User)
	return u
}

// ---------- sesiones (cookie firmada) ----------

func (a *App) secret() []byte {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cfg.Secret == "" {
		a.cfg.Secret = randHex(32)
		a.saveConfigLocked()
	}
	return []byte(a.cfg.Secret)
}

func (a *App) sign(uid string, exp int64) string {
	m := hmac.New(sha256.New, a.secret())
	m.Write([]byte(uid + "." + strconv.FormatInt(exp, 10)))
	return hex.EncodeToString(m.Sum(nil))
}

func (a *App) newSession(uid string) string {
	exp := time.Now().Add(sessionTTL).Unix()
	return "s1." + uid + "." + strconv.FormatInt(exp, 10) + "." + a.sign(uid, exp)
}

// sessionUser devuelve el usuario de la sesión (cookie "lum" o cabecera Bearer) o nil.
func (a *App) sessionUser(r *http.Request) *User {
	t := ""
	if c, err := r.Cookie("lum"); err == nil {
		t = c.Value
	} else if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		t = strings.TrimPrefix(h, "Bearer ")
	}
	p := strings.Split(t, ".")
	if len(p) != 4 || p[0] != "s1" {
		return nil
	}
	exp, err := strconv.ParseInt(p[2], 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return nil
	}
	if subtle.ConstantTimeCompare([]byte(a.sign(p[1], exp)), []byte(p[3])) != 1 {
		return nil
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	for i := range a.cfg.Users {
		if a.cfg.Users[i].ID == p[1] {
			u := a.cfg.Users[i]
			return &u
		}
	}
	return nil
}

func setSessionCookie(w http.ResponseWriter, val string) {
	http.SetCookie(w, &http.Cookie{Name: "lum", Value: val, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: int(sessionTTL.Seconds())})
}

// ---------- permisos por ruta ----------

// allowed aplica las reglas de acceso de una cuenta a una petición (administración, TV en directo y bibliotecas).
func (a *App) allowed(u *User, r *http.Request) bool {
	p := r.URL.Path
	has := func(pre string) bool { return strings.HasPrefix(p, pre) }
	switch {
	case has("/api/users"), has("/api/fs"), has("/api/system"), has("/api/update"), has("/api/libraries"), has("/api/cloud/"):
		return u.isAdmin()
	case p == "/api/xtream/fav":
		return u.liveOK()
	case has("/api/xtream/"):
		return u.isAdmin()
	case has("/api/live/"), has("/live/"), has("/api/logo/"):
		return u.liveOK()
	case has("/api/profiles"):
		return r.Method == "GET" // en modo cuentas los perfiles los gestiona el administrador desde Usuarios
	}
	first := func(pre string) string {
		s := strings.TrimPrefix(p, pre)
		if i := strings.IndexByte(s, '/'); i >= 0 {
			s = s[:i]
		}
		return s
	}
	switch {
	case has("/api/library/"):
		return u.libOK(first("/api/library/"))
	case has("/api/series/"):
		if s := first("/api/series/"); strings.HasPrefix(s, "x") { // serie de la nube: depende de su biblioteca
			n, _ := strconv.Atoi(s[1:])
			if lib := a.cloudLibFor(n); lib != "" {
				return u.libOK(lib)
			}
			return false
		}
		a.mu.RLock()
		defer a.mu.RUnlock()
		if eps := a.bySeries[first("/api/series/")]; len(eps) > 0 {
			return u.libOK(eps[0].Lib)
		}
	case has("/api/item/"), has("/api/meta/"), has("/api/thumb/"), has("/stream/"), has("/remux/"), has("/sub/"):
		id := first("/api/item/")
		for _, pre := range []string{"/api/meta/", "/api/thumb/", "/stream/", "/remux/", "/sub/"} {
			if has(pre) {
				id = first(pre)
			}
		}
		a.mu.RLock()
		defer a.mu.RUnlock()
		if it := a.items[id]; it != nil {
			return u.libOK(it.Lib)
		}
	}
	return true
}

// ---------- API pública de sesión ----------

type meView struct {
	ID    string    `json:"id"`
	Name  string    `json:"name"`
	Admin bool      `json:"admin"`
	Live  bool      `json:"live"`
	Libs  []Library `json:"libs"`
}

func (a *App) hMe(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	a.mu.RLock()
	libs := []Library{}
	for _, l := range a.cfg.Libraries {
		if u.libOK(l.ID) {
			if !u.isAdmin() {
				l.Path = "" // las rutas del servidor son solo para administradores
			}
			libs = append(libs, l)
		}
	}
	live := a.cfg.Xtream.URL != "" && len(a.cfg.Selected) > 0 && u.liveOK()
	a.mu.RUnlock()
	me := meView{Admin: u.isAdmin(), Live: live, Libs: libs}
	if u != nil {
		me.ID, me.Name = u.ID, u.Name
	}
	writeJSON(w, r, map[string]any{"accounts": u != nil, "me": me})
}

func (a *App) hLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: "lum", Value: "", Path: "/", HttpOnly: true, MaxAge: -1})
	http.SetCookie(w, &http.Cookie{Name: "lp", Value: "", Path: "/", MaxAge: -1})
	w.WriteHeader(204)
}

// loginAccount valida usuario y contraseña. La contraseña global (-password) sigue sirviendo como acceso de emergencia
// del primer administrador, por si se olvida la contraseña de la cuenta.
func (a *App) loginAccount(name, pass string) *User {
	name = strings.ToLower(strings.TrimSpace(name))
	a.mu.RLock()
	users := append([]User(nil), a.cfg.Users...)
	a.mu.RUnlock()
	for i := range users {
		if strings.ToLower(users[i].Name) == name && users[i].checkPass(pass) {
			return &users[i]
		}
	}
	if a.pass != "" && (name == "admin" || name == "") && subtle.ConstantTimeCompare([]byte(pass), []byte(a.pass)) == 1 {
		for i := range users {
			if users[i].Admin {
				return &users[i]
			}
		}
	}
	return nil
}

func clientIP(r *http.Request) string {
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	return ip
}

// ---------- gestión de usuarios (solo administradores) ----------

type userIn struct {
	Name     string   `json:"name"`
	Password string   `json:"password"`
	Admin    bool     `json:"admin"`
	AllLibs  bool     `json:"allLibs"`
	Libs     []string `json:"libs"`
	Live     bool     `json:"live"`
	Color    int      `json:"color"`
	Kind     string   `json:"kind"`
}

type userView struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Admin   bool     `json:"admin"`
	AllLibs bool     `json:"allLibs"`
	Libs    []string `json:"libs"`
	Live    bool     `json:"live"`
	Color   int      `json:"color"`
	Kind    string   `json:"kind"`
}

func (a *App) userViews() []userView {
	out := []userView{}
	for _, u := range a.cfg.Users {
		v := userView{ID: u.ID, Name: u.Name, Admin: u.Admin, AllLibs: u.AllLibs, Libs: u.Libs, Live: u.Live, Kind: "u"}
		if v.Libs == nil {
			v.Libs = []string{}
		}
		for _, p := range a.cfg.Profiles {
			if p.ID == u.ID {
				v.Color, v.Kind = p.Color, p.Kind
			}
		}
		out = append(out, v)
	}
	return out
}

func (a *App) hUsers(w http.ResponseWriter, r *http.Request) {
	a.mu.RLock()
	v := a.userViews()
	libs := append([]Library(nil), a.cfg.Libraries...)
	a.mu.RUnlock()
	writeJSON(w, r, map[string]any{"users": v, "accounts": len(v) > 0, "libs": libs})
}

func (in *userIn) clean() error {
	in.Name = strings.TrimSpace(in.Name)
	if n := utf8.RuneCountInString(in.Name); n < 2 || n > 24 {
		return errors.New("el nombre debe tener entre 2 y 24 caracteres")
	}
	if in.Password != "" && utf8.RuneCountInString(in.Password) < 6 {
		return errors.New("la contraseña debe tener al menos 6 caracteres")
	}
	if in.Color < 0 || in.Color > 5 {
		in.Color = 0
	}
	if in.Kind != "k" {
		in.Kind = "u"
	}
	return nil
}

func (a *App) nameTaken(name, except string) bool {
	for _, u := range a.cfg.Users {
		if strings.EqualFold(u.Name, name) && u.ID != except {
			return true
		}
	}
	return false
}

func (a *App) hUserCreate(w http.ResponseWriter, r *http.Request) {
	var in userIn
	if err := readJSON(r, &in); err != nil {
		jsonErr(w, 400, "datos no válidos")
		return
	}
	if err := in.clean(); err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	if in.Password == "" {
		jsonErr(w, 400, "escribe una contraseña (mínimo 6 caracteres)")
		return
	}
	a.mu.Lock()
	first := len(a.cfg.Users) == 0
	if first {
		in.Admin = true // la primera cuenta siempre es administradora
	}
	if a.nameTaken(in.Name, "") {
		a.mu.Unlock()
		jsonErr(w, 409, "ya existe un usuario con ese nombre")
		return
	}
	u := User{ID: randHex(5), Name: in.Name, Salt: randHex(12), Admin: in.Admin, AllLibs: in.AllLibs, Libs: in.Libs, Live: in.Live}
	u.Hash = hashPass(in.Password, u.Salt)
	if first && len(a.cfg.Profiles) > 0 {
		// la primera cuenta hereda el perfil existente (con su historial y favoritos)
		u.ID = a.cfg.Profiles[0].ID
		a.cfg.Profiles[0].Name, a.cfg.Profiles[0].Color, a.cfg.Profiles[0].Kind = in.Name, in.Color, in.Kind
	} else {
		a.cfg.Profiles = append(a.cfg.Profiles, Profile{ID: u.ID, Name: in.Name, Color: in.Color, Kind: in.Kind})
	}
	a.cfg.Users = append(a.cfg.Users, u)
	a.saveConfigLocked()
	a.mu.Unlock()
	if first { // quien activa las cuentas no debe quedarse fuera
		setSessionCookie(w, a.newSession(u.ID))
	}
	log.Printf("cuenta creada: %s (admin=%v)", u.Name, u.Admin)
	writeJSON(w, r, map[string]any{"ok": true, "id": u.ID})
}

func (a *App) hUserUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var in userIn
	if err := readJSON(r, &in); err != nil {
		jsonErr(w, 400, "datos no válidos")
		return
	}
	if err := in.clean(); err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	idx := -1
	for i := range a.cfg.Users {
		if a.cfg.Users[i].ID == id {
			idx = i
		}
	}
	if idx < 0 {
		jsonErr(w, 404, "usuario no encontrado")
		return
	}
	if a.nameTaken(in.Name, id) {
		jsonErr(w, 409, "ya existe un usuario con ese nombre")
		return
	}
	if !in.Admin && a.cfg.Users[idx].Admin && a.adminCount() <= 1 {
		jsonErr(w, 400, "debe quedar al menos un administrador")
		return
	}
	u := &a.cfg.Users[idx]
	u.Name, u.Admin, u.AllLibs, u.Libs, u.Live = in.Name, in.Admin, in.AllLibs, in.Libs, in.Live
	if in.Password != "" {
		u.Salt = randHex(12)
		u.Hash = hashPass(in.Password, u.Salt)
	}
	for i := range a.cfg.Profiles {
		if a.cfg.Profiles[i].ID == id {
			a.cfg.Profiles[i].Name, a.cfg.Profiles[i].Color, a.cfg.Profiles[i].Kind = in.Name, in.Color, in.Kind
		}
	}
	a.saveConfigLocked()
	writeJSON(w, r, map[string]any{"ok": true})
}

func (a *App) adminCount() int {
	n := 0
	for _, u := range a.cfg.Users {
		if u.Admin {
			n++
		}
	}
	return n
}

func (a *App) hUserDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	a.mu.Lock()
	idx := -1
	for i := range a.cfg.Users {
		if a.cfg.Users[i].ID == id {
			idx = i
		}
	}
	if idx < 0 {
		a.mu.Unlock()
		jsonErr(w, 404, "usuario no encontrado")
		return
	}
	if a.cfg.Users[idx].Admin && a.adminCount() <= 1 {
		a.mu.Unlock()
		jsonErr(w, 400, "no se puede eliminar al único administrador")
		return
	}
	if me := userFrom(r); me != nil && me.ID == id {
		a.mu.Unlock()
		jsonErr(w, 400, "no puedes eliminar tu propia cuenta")
		return
	}
	a.cfg.Users = append(a.cfg.Users[:idx:idx], a.cfg.Users[idx+1:]...)
	profs := a.cfg.Profiles[:0:0]
	for _, p := range a.cfg.Profiles {
		if p.ID != id {
			profs = append(profs, p)
		}
	}
	a.cfg.Profiles = profs
	delete(a.progs, id)
	a.saveConfigLocked()
	a.mu.Unlock()
	a.saveProgress()
	writeJSON(w, r, map[string]any{"ok": true})
}
