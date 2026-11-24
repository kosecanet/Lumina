package main

// Actualizaciones desde GitHub Releases.
//
// Flujo: consulta la última release del repositorio configurado → descarga el binario de tu plataforma
// (lumina-<os>-<arch>[.exe]) → verifica su SHA-256 con checksums.txt → comprueba que arranca (-version)
// → sustituye el ejecutable (conserva el anterior como .old) → se reinicia solo.
// Los clientes de TV/Android son un visor de la web del servidor, así que se actualizan con él.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	version     = "dev" // se fija al compilar: -ldflags "-X main.version=v1.2.3"
	defaultRepo = officialRepo
)

type UpdCfg struct {
	Repo string `json:"repo,omitempty"` // "usuario/repositorio"
	Auto bool   `json:"auto,omitempty"` // instalar solo cuando no hay nadie viendo nada
	Pre  bool   `json:"pre,omitempty"`  // incluir pre-releases
}

type ghAsset struct {
	Name    string `json:"name"`
	API     string `json:"url"`
	Browser string `json:"browser_download_url"`
	Size    int64  `json:"size"`
}

type ghRelease struct {
	Tag    string    `json:"tag_name"`
	Name   string    `json:"name"`
	Body   string    `json:"body"`
	HTML   string    `json:"html_url"`
	Draft  bool      `json:"draft"`
	Pre    bool      `json:"prerelease"`
	Assets []ghAsset `json:"assets"`
}

type Updater struct {
	a       *App
	mu      sync.Mutex
	rel     *ghRelease
	checked time.Time
	lastErr string
	busy    bool
	state   string // "", "descargando", "verificando", "instalando", "reiniciando"
}

func newUpdater(a *App) *Updater { return &Updater{a: a} }

func (u *Updater) repo() string {
	if r := strings.TrimSpace(os.Getenv("LUMINA_REPO")); r != "" {
		return r
	}
	u.a.mu.RLock()
	r := u.a.cfg.Update.Repo
	u.a.mu.RUnlock()
	if r == "" {
		r = defaultRepo
	}
	return strings.TrimSpace(r)
}

// ---------- versiones ----------

func verParts(v string) []int {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	var out []int
	for _, p := range strings.Split(v, ".") {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil
		}
		out = append(out, n)
	}
	return out
}

// newerVer: ¿a es posterior a b?
func newerVer(a, b string) bool {
	pa, pb := verParts(a), verParts(b)
	if pa == nil || pb == nil {
		return false
	}
	for i := 0; i < len(pa) || i < len(pb); i++ {
		x, y := 0, 0
		if i < len(pa) {
			x = pa[i]
		}
		if i < len(pb) {
			y = pb[i]
		}
		if x != y {
			return x > y
		}
	}
	return false
}

// ---------- GitHub ----------

var ghHosts = []string{"github.com", "api.github.com", "githubusercontent.com"}

func ghHostOK(h string) bool {
	h = strings.ToLower(h)
	for _, ok := range ghHosts {
		if h == ok || strings.HasSuffix(h, "."+ok) {
			return true
		}
	}
	return false
}

var updClient = &http.Client{
	Timeout: 10 * time.Minute,
	Transport: &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		TLSHandshakeTimeout: 10 * time.Second,
		DialContext:         (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
	},
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 6 || req.URL.Scheme != "https" || !ghHostOK(req.URL.Hostname()) {
			return errors.New("redirección no permitida")
		}
		return nil
	},
}

func (u *Updater) get(ctx context.Context, rawURL, accept string) (*http.Response, error) {
	pu, err := url.Parse(rawURL)
	if err != nil || pu.Scheme != "https" || !ghHostOK(pu.Hostname()) {
		return nil, errors.New("URL de actualización no permitida")
	}
	req, _ := http.NewRequestWithContext(ctx, "GET", rawURL, nil)
	req.Header.Set("User-Agent", "Lumina/"+version)
	req.Header.Set("Accept", accept)
	if t := os.Getenv("LUMINA_GH_TOKEN"); t != "" { // repositorio privado
		req.Header.Set("Authorization", "Bearer "+t)
	}
	r, err := updClient.Do(req)
	if err != nil {
		return nil, cleanErr(err)
	}
	if r.StatusCode != 200 {
		r.Body.Close()
		switch r.StatusCode {
		case 404:
			return nil, errors.New("repositorio o release no encontrados (si es privado, define LUMINA_GH_TOKEN)")
		case 403, 429:
			return nil, errors.New("GitHub ha limitado las consultas; inténtalo más tarde")
		}
		return nil, fmt.Errorf("GitHub respondió HTTP %d", r.StatusCode)
	}
	return r, nil
}

// Check consulta la última release publicada.
func (u *Updater) Check(ctx context.Context) error {
	repo := u.repo()
	u.mu.Lock()
	defer u.mu.Unlock()
	u.checked = time.Now()
	if repo == "" {
		u.lastErr = "Falta configurar el repositorio de GitHub (usuario/nombre)"
		return errors.New(u.lastErr)
	}
	if strings.Count(repo, "/") != 1 || strings.ContainsAny(repo, " ?#\\") {
		u.lastErr = "Repositorio no válido: usa el formato usuario/nombre"
		return errors.New(u.lastErr)
	}
	u.a.mu.RLock()
	pre := u.a.cfg.Update.Pre
	u.a.mu.RUnlock()
	api := "https://api.github.com/repos/" + repo + "/releases/latest"
	if pre {
		api = "https://api.github.com/repos/" + repo + "/releases?per_page=10"
	}
	resp, err := u.get(ctx, api, "application/vnd.github+json")
	if err != nil {
		u.lastErr = err.Error()
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	var rel *ghRelease
	if pre {
		var list []ghRelease
		if json.Unmarshal(b, &list) == nil {
			for i := range list {
				if !list[i].Draft && (rel == nil || newerVer(list[i].Tag, rel.Tag)) {
					rel = &list[i]
				}
			}
		}
	} else {
		rel = new(ghRelease)
		if json.Unmarshal(b, rel) != nil {
			rel = nil
		}
	}
	if rel == nil || rel.Tag == "" {
		u.lastErr = "no se encontró ninguna release"
		return errors.New(u.lastErr)
	}
	u.rel, u.lastErr = rel, ""
	return nil
}

func assetName() string {
	n := "lumina-" + runtime.GOOS + "-" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		n += ".exe"
	}
	return n
}

func (u *Updater) available() bool {
	return u.rel != nil && newerVer(u.rel.Tag, version)
}

// Status es lo que ve la web.
func (u *Updater) Status() map[string]any {
	u.mu.Lock()
	defer u.mu.Unlock()
	m := map[string]any{
		"version": version, "repo": u.repo(), "busy": u.busy, "state": u.state, "error": u.lastErr,
		"checked": u.checked.Unix(), "dev": version == "dev", "platform": assetName(),
	}
	u.a.mu.RLock()
	m["auto"], m["pre"] = u.a.cfg.Update.Auto, u.a.cfg.Update.Pre
	u.a.mu.RUnlock()
	if u.rel != nil {
		m["latest"] = u.rel.Tag
		m["available"] = u.available()
		m["notes"] = truncRunes(u.rel.Body, 1500)
		m["url"] = u.rel.HTML
		has := false
		for _, as := range u.rel.Assets {
			if as.Name == assetName() {
				has = true
			}
		}
		m["hasAsset"] = has
	}
	return m
}

func truncRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// ---------- instalación ----------

func (u *Updater) setState(s string) { u.mu.Lock(); u.state = s; u.mu.Unlock() }

// Apply descarga, verifica e instala la release encontrada por Check. No reinicia: devuelve nil y el llamador llama a restartSelf.
func (u *Updater) Apply(ctx context.Context) (string, error) {
	u.mu.Lock()
	if u.busy {
		u.mu.Unlock()
		return "", errors.New("ya hay una actualización en curso")
	}
	if version == "dev" {
		u.mu.Unlock()
		return "", errors.New("esta es una compilación de desarrollo; las actualizaciones solo funcionan con binarios de una release")
	}
	if !u.available() {
		u.mu.Unlock()
		return "", errors.New("no hay ninguna versión nueva")
	}
	rel := u.rel
	u.busy = true
	u.mu.Unlock()
	defer func() { u.mu.Lock(); u.busy, u.state = false, ""; u.mu.Unlock() }()

	var bin, sums *ghAsset
	for i := range rel.Assets {
		switch rel.Assets[i].Name {
		case assetName():
			bin = &rel.Assets[i]
		case "checksums.txt":
			sums = &rel.Assets[i]
		}
	}
	if bin == nil {
		return "", fmt.Errorf("la release %s no incluye %s", rel.Tag, assetName())
	}
	if sums == nil {
		return "", errors.New("la release no incluye checksums.txt; no se instala sin verificar")
	}
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if p, err := filepath.EvalSymlinks(exe); err == nil {
		exe = p
	}
	dir := filepath.Dir(exe)
	tmp := filepath.Join(dir, ".lumina-update.tmp")
	if f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755); err != nil {
		return "", fmt.Errorf("no se puede escribir en %s (%v); instala la actualización a mano", dir, err)
	} else {
		f.Close()
	}
	defer os.Remove(tmp)

	dl := func(as *ghAsset, w io.Writer, max int64) error {
		src, accept := as.Browser, "application/octet-stream"
		if os.Getenv("LUMINA_GH_TOKEN") != "" && as.API != "" {
			src = as.API
		}
		r, err := u.get(ctx, src, accept)
		if err != nil {
			return err
		}
		defer r.Body.Close()
		n, err := io.Copy(w, io.LimitReader(r.Body, max+1))
		if err == nil && n > max {
			err = errors.New("archivo demasiado grande")
		}
		return err
	}

	u.setState("descargando")
	var sb strings.Builder
	if err := dl(sums, &sb, 1<<20); err != nil {
		return "", fmt.Errorf("checksums.txt: %w", err)
	}
	want := ""
	for _, ln := range strings.Split(sb.String(), "\n") {
		f := strings.Fields(ln)
		if len(f) >= 2 && strings.TrimPrefix(f[len(f)-1], "*") == bin.Name {
			want = strings.ToLower(f[0])
		}
	}
	if len(want) != 64 {
		return "", fmt.Errorf("checksums.txt no contiene %s", bin.Name)
	}
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	if err := dl(bin, io.MultiWriter(f, h), 300<<20); err != nil {
		f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	u.setState("verificando")
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		return "", errors.New("la firma SHA-256 no coincide: descarga corrupta o manipulada; no se instala")
	}
	_ = os.Chmod(tmp, 0o755)
	// el binario nuevo debe arrancar y declarar la versión esperada
	cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	out, err := exec.CommandContext(cctx, tmp, "-version").Output()
	cancel()
	if err != nil || !strings.Contains(string(out), strings.TrimPrefix(rel.Tag, "v")) {
		return "", errors.New("el binario descargado no arranca en este sistema; se mantiene la versión actual")
	}

	u.setState("instalando")
	old := exe + ".old"
	_ = os.Remove(old)
	if err := os.Rename(exe, old); err != nil {
		return "", fmt.Errorf("no se pudo reemplazar el ejecutable: %w", err)
	}
	if err := os.Rename(tmp, exe); err != nil {
		_ = os.Rename(old, exe) // deshacer
		return "", fmt.Errorf("no se pudo instalar: %w", err)
	}
	log.Printf("actualizado %s → %s (copia anterior: %s)", version, rel.Tag, old)
	u.setState("reiniciando")
	return rel.Tag, nil
}

// ---------- fondo: comprobación periódica y auto-instalación ----------

func (u *Updater) loop() {
	time.Sleep(time.Minute)
	for {
		if u.repo() != "" {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			err := u.Check(ctx)
			cancel()
			u.a.mu.RLock()
			auto := u.a.cfg.Update.Auto
			u.a.mu.RUnlock()
			if err == nil && auto && u.available() && version != "dev" {
				u.autoApply()
			}
		}
		time.Sleep(6 * time.Hour)
	}
}

// autoApply instala cuando nadie está viendo nada (espera hasta 2 h a que haya un momento tranquilo).
func (u *Updater) autoApply() {
	for i := 0; i < 24; i++ {
		if len(u.a.hub.status()) == 0 && !u.a.scan.Running {
			if _, err := u.Apply(context.Background()); err != nil {
				log.Printf("actualización automática: %v", err)
				return
			}
			time.Sleep(300 * time.Millisecond)
			restartSelf()
			return
		}
		time.Sleep(5 * time.Minute)
	}
}

// ---------- API ----------

func (a *App) updateAllowed(r *http.Request) bool {
	if a.pass != "" || a.accounts() {
		return true // ya pasó por la autenticación (y las cuentas limitan esto a administradores)
	}
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	p := net.ParseIP(ip)
	return p != nil && p.IsLoopback()
}

func (a *App) hUpdate(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("check") == "1" {
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		_ = a.upd.Check(ctx)
		cancel()
	}
	st := a.upd.Status()
	st["canApply"] = a.updateAllowed(r)
	writeJSON(w, r, st)
}

func (a *App) hUpdateCfg(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Repo string `json:"repo"`
		Auto bool   `json:"auto"`
		Pre  bool   `json:"pre"`
	}
	if err := readJSON(r, &in); err != nil {
		jsonErr(w, 400, "datos no válidos")
		return
	}
	repo := strings.TrimSpace(in.Repo)
	repo = strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(repo, "https://github.com/"), "http://github.com/"), ".git")
	if repo != "" && (strings.Count(repo, "/") != 1 || strings.ContainsAny(repo, " ?#\\")) {
		jsonErr(w, 400, "usa el formato usuario/repositorio")
		return
	}
	a.mu.Lock()
	a.cfg.Update = UpdCfg{Repo: repo, Auto: in.Auto && a.updateAllowed(r), Pre: in.Pre}
	a.mu.Unlock()
	a.saveConfig()
	a.upd.mu.Lock()
	a.upd.rel = nil
	a.upd.mu.Unlock()
	a.hUpdate(w, r)
}

func (a *App) hUpdateApply(w http.ResponseWriter, r *http.Request) {
	if !a.updateAllowed(r) {
		jsonErr(w, 403, "por seguridad, actualizar desde la web requiere haber definido una contraseña (-password) o hacerlo desde el propio servidor")
		return
	}
	tag, err := a.upd.Apply(context.Background())
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	writeJSON(w, r, map[string]any{"ok": true, "version": tag, "restarting": true})
	go func() { time.Sleep(600 * time.Millisecond); restartSelf() }()
}
