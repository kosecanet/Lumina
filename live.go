package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Hub mantiene como mucho `max_connections` conexiones hacia el proveedor y reparte cada
// canal a todos los clientes que lo estén viendo (una sola conexión por canal).

// Sub es un espectador. La cola se mide en bytes (q) y no en trozos: el proveedor manda al empezar una ráfaga mucho más rápida
// que el vídeo, y eso no es un cliente lento. Solo se corta a quien acumula más de subMaxBytes (~1 min de HD).
type Sub struct {
	ch chan []byte
	q  atomic.Int64
}

const subMaxBytes = 96 << 20

// take se llama por cada trozo leído de la cola.
func (s *Sub) take(b []byte) { s.q.Add(-int64(len(b))) }

// put intenta encolar; false si el espectador no da abasto.
func (s *Sub) put(b []byte) bool {
	if s.q.Load()+int64(len(b)) > subMaxBytes {
		return false
	}
	select {
	case s.ch <- b:
		s.q.Add(int64(len(b)))
		return true
	default:
		return false
	}
}

type LiveStream struct {
	id        int
	hub       *Hub
	subs      map[*Sub]struct{}
	started   time.Time
	cancel    context.CancelFunc
	ready     chan struct{}
	readyOnce sync.Once
	err       error
	bytes     atomic.Int64
	idle      *time.Timer
	dead      bool
	name      string             // nombre para mostrar cuando no es un canal (series de la nube)
	vod       bool               // ocupa una conexión del proveedor pero no tiene bucle de lectura propio
	ctx       context.Context    // solo vod: se cancela al expulsarlo
	cur       context.CancelFunc // solo vod: petición en curso (el navegador abandona las anteriores)
	ring      [][]byte           // últimos segundos recibidos, para dar un arranque inmediato a quien entra (protegido por hub.mu)
	ringBytes int
}

// ringMax: unos 8-10 s de un canal HD. Quien entra recibe esto de golpe y arranca sin esperar a llenar su colchón en tiempo real;
// además queda ese margen de seguridad frente a los tirones de la red o del proveedor.
const ringMax = 10 << 20

type Hub struct {
	a        *App
	mu       sync.Mutex
	streams  map[int]*LiveStream
	lastWarm time.Time
}

func newHub(a *App) *Hub { return &Hub{a: a, streams: map[int]*LiveStream{}} }

type BusyInfo struct {
	ID int    `json:"id"`
	N  string `json:"n"`
	V  int    `json:"v"`
}

var liveClient = &http.Client{Transport: &http.Transport{
	Proxy:                 http.ProxyFromEnvironment,
	ResponseHeaderTimeout: 20 * time.Second,
	TLSHandshakeTimeout:   8 * time.Second,
	DialContext:           (&net.Dialer{Timeout: 8 * time.Second}).DialContext,
	IdleConnTimeout:       5 * time.Second,
	DisableCompression:    true,
}}

func (a *App) chanName(id int) string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if ch := a.chans[id]; ch != nil {
		return ch.Name
	}
	return strconv.Itoa(id)
}

func (a *App) hasChannel(id int) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.chans[id] != nil
}

// makeRoomLocked deja sitio para abrir `id`. Devuelve los canales que lo impiden (si no force).
func (h *Hub) makeRoomLocked(id, max int, force bool) []BusyInfo {
	if ls := h.streams[id]; ls != nil && !ls.dead {
		return nil
	}
	var active []*LiveStream
	for sid, s := range h.streams {
		switch {
		case s.dead:
			delete(h.streams, sid)
		case len(s.subs) == 0: // inactivo: se cierra ya para liberar la conexión del proveedor
			s.stopLocked()
			delete(h.streams, sid)
		default:
			active = append(active, s)
		}
	}
	if len(active) < max {
		return nil
	}
	sort.Slice(active, func(i, j int) bool { return active[i].started.Before(active[j].started) })
	if !force {
		var busy []BusyInfo
		for _, s := range active {
			busy = append(busy, BusyInfo{ID: s.id, V: len(s.subs), N: s.name})
		}
		return busy
	}
	for len(active) >= max {
		s := active[0]
		s.kickLocked()
		delete(h.streams, s.id)
		active = active[1:]
	}
	return nil
}

func (h *Hub) fillNames(b []BusyInfo) []BusyInfo {
	for i := range b {
		if b[i].N == "" {
			b[i].N = h.a.chanName(b[i].ID)
		}
	}
	return b
}

// tune se usa antes de conectar el reproductor: comprueba/hace sitio sin abrir el stream.
func (h *Hub) tune(id int, force bool) []BusyInfo {
	max := h.a.maxConn()
	var busy []BusyInfo
	// Al hacer zapping, el navegador tarda unas décimas en cerrar la emisión anterior: antes de dar el canal por ocupado
	// se espera un momento a que ese espectador desaparezca (con «forzar» no hace falta).
	for i := 0; i < 15; i++ {
		h.mu.Lock()
		busy = h.makeRoomLocked(id, max, force)
		h.mu.Unlock()
		if busy == nil || force {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	return h.fillNames(busy)
}

func (h *Hub) acquire(id int) (*LiveStream, *Sub, []BusyInfo, error) {
	if !h.a.hasChannel(id) {
		return nil, nil, nil, errors.New("canal desconocido")
	}
	c, err := h.a.xclient()
	if err != nil {
		return nil, nil, nil, err
	}
	max := h.a.maxConn()
	h.mu.Lock()
	if busy := h.makeRoomLocked(id, max, false); busy != nil {
		h.mu.Unlock()
		return nil, nil, h.fillNames(busy), nil
	}
	ls := h.streams[id]
	if ls == nil {
		ctx, cancel := context.WithCancel(context.Background())
		ls = &LiveStream{id: id, hub: h, subs: map[*Sub]struct{}{}, started: time.Now(), cancel: cancel, ready: make(chan struct{})}
		h.streams[id] = ls
		go ls.run(ctx, c.liveURL(id), c.ua)
	}
	if ls.idle != nil {
		ls.idle.Stop()
		ls.idle = nil
	}
	sub := &Sub{ch: make(chan []byte, 16384)}
	ls.replayLocked(sub)
	ls.subs[sub] = struct{}{}
	h.mu.Unlock()
	return ls, sub, nil, nil
}

func (h *Hub) release(ls *LiveStream, sub *Sub) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(ls.subs, sub)
	if len(ls.subs) == 0 && !ls.dead && ls.idle == nil {
		ls.armIdleLocked(10 * time.Second) // margen por si el usuario vuelve a este canal enseguida (zapping)
	}
}

// armIdleLocked cierra la emisión si pasa `d` sin espectadores.
func (ls *LiveStream) armIdleLocked(d time.Duration) {
	h := ls.hub
	if ls.idle != nil {
		ls.idle.Stop()
	}
	ls.idle = time.AfterFunc(d, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if len(ls.subs) == 0 && !ls.dead {
			ls.stopLocked()
			if h.streams[ls.id] == ls {
				delete(h.streams, ls.id)
			}
		}
	})
}

// warm abre el canal por adelantado (sin espectador) para que, al pulsarlo, ya haya vídeo recibido y arranque al instante.
// Solo si no molesta: si ya se está viendo otro canal (o no queda conexión libre) no hace nada. Se limita el ritmo para no
// golpear al proveedor al pasar el ratón por la lista.
func (h *Hub) warm(id int) bool {
	if !h.a.hasChannel(id) {
		return false
	}
	c, err := h.a.xclient()
	if err != nil {
		return false
	}
	max := h.a.maxConn()
	h.mu.Lock()
	defer h.mu.Unlock()
	ls := h.streams[id]
	if ls == nil || ls.dead {
		if time.Since(h.lastWarm) < 8*time.Second {
			return false
		}
		if h.makeRoomLocked(id, max, false) != nil {
			return false
		}
		h.lastWarm = time.Now()
		ctx, cancel := context.WithCancel(context.Background())
		ls = &LiveStream{id: id, hub: h, subs: map[*Sub]struct{}{}, started: time.Now(), cancel: cancel, ready: make(chan struct{})}
		h.streams[id] = ls
		go ls.run(ctx, c.liveURL(id), c.ua)
	}
	if len(ls.subs) == 0 {
		ls.armIdleLocked(30 * time.Second)
	}
	return true
}

// vodAcquire ocupa (o reutiliza) la conexión del proveedor para reproducir una serie de la nube. `key` es negativo
// (-id de la serie) para no chocar con los canales. Devuelve quién lo impide si no hay sitio y no se fuerza.
func (h *Hub) vodAcquire(key int, name string, force bool) (*LiveStream, *Sub, []BusyInfo) {
	max := h.a.maxConn()
	h.mu.Lock()
	defer h.mu.Unlock()
	if busy := h.makeRoomLocked(key, max, force); busy != nil {
		return nil, nil, h.fillNames(busy)
	}
	ls := h.streams[key]
	if ls == nil || ls.dead {
		ctx, cancel := context.WithCancel(context.Background())
		ls = &LiveStream{id: key, hub: h, subs: map[*Sub]struct{}{}, started: time.Now(), cancel: cancel, ctx: ctx, vod: true, name: name,
			ready: make(chan struct{})}
		h.streams[key] = ls
	}
	if ls.idle != nil {
		ls.idle.Stop()
		ls.idle = nil
	}
	sub := &Sub{ch: make(chan []byte, 1)}
	ls.subs[sub] = struct{}{}
	return ls, sub, nil
}

// swapCur anota la petición en curso de un vod y cancela la anterior (el navegador abandona sus peticiones Range al saltar,
// pero el proveedor solo admite una conexión y tarda en liberarla). Devuelve si había una anterior.
func (ls *LiveStream) swapCur(c context.CancelFunc) bool {
	ls.hub.mu.Lock()
	old := ls.cur
	ls.cur = c
	ls.hub.mu.Unlock()
	if old != nil {
		old()
	}
	return old != nil
}

func (ls *LiveStream) stopLocked() {
	ls.dead = true
	ls.cancel()
}

func (ls *LiveStream) kickLocked() {
	ls.dead = true
	ls.cancel()
	for s := range ls.subs {
		close(s.ch)
	}
	ls.subs = map[*Sub]struct{}{}
}

func (ls *LiveStream) finish(err error) {
	h := ls.hub
	h.mu.Lock()
	if h.streams[ls.id] == ls {
		delete(h.streams, ls.id)
	}
	ls.dead = true
	for s := range ls.subs {
		close(s.ch)
	}
	ls.subs = map[*Sub]struct{}{}
	h.mu.Unlock()
	ls.readyOnce.Do(func() { ls.err = err; close(ls.ready) })
}

// replayLocked entrega al nuevo espectador lo ya recibido (agrupado en trozos de 64 KB, siempre en múltiplos de 188 bytes).
func (ls *LiveStream) replayLocked(sub *Sub) {
	var slab []byte
	flush := func() {
		if len(slab) > 0 {
			sub.put(slab)
			slab = nil
		}
	}
	for _, c := range ls.ring {
		if len(slab) > 0 && len(slab)+len(c) > 64<<10 {
			flush()
		}
		slab = append(slab, c...)
	}
	flush()
}

func (ls *LiveStream) resetRing() {
	ls.hub.mu.Lock()
	ls.ring, ls.ringBytes = nil, 0
	ls.hub.mu.Unlock()
}

func (ls *LiveStream) broadcast(b []byte) {
	ls.hub.mu.Lock()
	ls.ring = append(ls.ring, append([]byte(nil), b...))
	ls.ringBytes += len(b)
	for ls.ringBytes > ringMax && len(ls.ring) > 1 {
		ls.ringBytes -= len(ls.ring[0])
		ls.ring[0] = nil
		ls.ring = ls.ring[1:]
	}
	for s := range ls.subs {
		if !s.put(b) {
			// Cliente que no da abasto (red lenta): tirar trozos sueltos estropea el vídeo (saltos, imagen rota). Mejor cortarlo
			// limpiamente; el reproductor reconecta y vuelve a entrar en el directo desde un punto limpio.
			delete(ls.subs, s)
			close(s.ch)
			log.Printf("directo: canal %d: un cliente no recibía a tiempo y se ha reconectado (%d MB en cola)", ls.id, s.q.Load()>>20)
		}
	}
	ls.hub.mu.Unlock()
}

// connect abre la emisión del proveedor con reintentos (tras un corte el proveedor tarda un instante en liberar la conexión).
func (ls *LiveStream) connect(ctx context.Context, url, ua string) (*http.Response, error) {
	var lastErr error
	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 {
			select {
			case <-time.After(time.Duration(attempt) * 900 * time.Millisecond):
			case <-ctx.Done():
				return nil, errors.New("cancelado")
			}
		}
		req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
		req.Header.Set("User-Agent", ua)
		r, err := liveClient.Do(req)
		if err != nil {
			lastErr = cleanErr(err)
			log.Printf("directo: fallo al conectar (intento %d): %v", attempt+1, lastErr)
			continue
		}
		if r.StatusCode != 200 {
			lastErr = fmt.Errorf("el proveedor respondió HTTP %d (¿límite de conexiones alcanzado?)", r.StatusCode)
			log.Printf("directo: el proveedor (%s) respondió HTTP %d (intento %d)", r.Request.URL.Host, r.StatusCode, attempt+1)
			r.Body.Close()
			if r.StatusCode == 404 {
				break
			}
			if r.StatusCode == 511 && attempt >= 1 {
				break // 511 del proveedor: insistir más no sirve y puede agravarlo (401/403 sí se reintentan: a veces aún libera la conexión anterior)
			}
			continue
		}
		ct := strings.ToLower(r.Header.Get("Content-Type"))
		if strings.Contains(ct, "text/") || strings.Contains(ct, "json") {
			// algunos proveedores sirven el TS con un Content-Type erróneo: se acepta si empieza por el byte de sincronía 0x47
			br := bufio.NewReaderSize(r.Body, 64<<10)
			b, _ := br.Peek(1)
			if len(b) == 1 && b[0] == 0x47 {
				r.Body = struct {
					io.Reader
					io.Closer
				}{br, r.Body}
				return r, nil
			}
			lastErr = errors.New("el proveedor no devolvió vídeo (¿canal caído o límite de conexiones?)")
			log.Printf("directo: respuesta sin vídeo de %s (Content-Type %q, intento %d)", r.Request.URL.Host, ct, attempt+1)
			r.Body.Close()
			continue
		}
		return r, nil
	}
	return nil, lastErr
}

func (ls *LiveStream) subCount() int {
	ls.hub.mu.Lock()
	defer ls.hub.mu.Unlock()
	return len(ls.subs)
}

func (ls *LiveStream) run(ctx context.Context, url, ua string) {
	t0 := time.Now()
	log.Printf("directo: abriendo canal %d", ls.id)
	resp, err := ls.connect(ctx, url, ua)
	if err != nil {
		log.Printf("directo: canal %d no se pudo abrir tras %.1fs: %v", ls.id, time.Since(t0).Seconds(), err)
		ls.finish(err)
		return
	}
	log.Printf("directo: canal %d conectado en %.1fs (host %s, %s)", ls.id, time.Since(t0).Seconds(), resp.Request.URL.Host, resp.Header.Get("Content-Type"))
	defer func() {
		log.Printf("directo: canal %d cerrado tras %.1fs, %d KB recibidos", ls.id, time.Since(t0).Seconds(), ls.bytes.Load()/1024)
	}()
	ls.readyOnce.Do(func() { close(ls.ready) })
	wd := time.AfterFunc(25*time.Second, ls.cancel) // si el proveedor se queda mudo, se corta
	defer wd.Stop()
	// Si el proveedor corta la emisión, se reconecta sin que los espectadores pierdan la sesión (máx. 3 cortes por minuto).
	var drops []time.Time
	for {
		// Se reenvía en cuanto llegan datos (menor latencia de arranque), siempre en múltiplos de 188 bytes (paquetes TS)
		var carry []byte
		for {
			b := make([]byte, 64<<10)
			k := copy(b, carry)
			n, rerr := resp.Body.Read(b[k:])
			total := k + n
			if n > 0 {
				wd.Reset(25 * time.Second)
				ls.bytes.Add(int64(n))
				if emit := total - total%188; emit > 0 {
					ls.broadcast(b[:emit])
					carry = append(carry[:0], b[emit:total]...)
				} else {
					carry = append(carry[:0], b[:total]...)
				}
			}
			if rerr != nil {
				break
			}
		}
		resp.Body.Close()
		if ctx.Err() != nil || ls.subCount() == 0 {
			break
		}
		now := time.Now()
		keep := drops[:0]
		for _, t := range drops {
			if now.Sub(t) < time.Minute {
				keep = append(keep, t)
			}
		}
		drops = append(keep, now)
		if len(drops) > 3 {
			break
		}
		wd.Reset(25 * time.Second)
		ls.resetRing() // tras un corte hay salto de tiempos: lo anterior no sirve para quien entre ahora
		if resp, err = ls.connect(ctx, url, ua); err != nil {
			break
		}
	}
	ls.finish(nil)
}

func (h *Hub) viewerMap() map[int]int {
	h.mu.Lock()
	defer h.mu.Unlock()
	m := map[int]int{}
	for id, s := range h.streams {
		if !s.dead {
			m[id] = len(s.subs)
		}
	}
	return m
}

func (h *Hub) status() []map[string]any {
	type row struct {
		id, v int
		since time.Time
		bytes int64
		name  string
	}
	var rows []row
	h.mu.Lock()
	for id, s := range h.streams {
		if !s.dead {
			rows = append(rows, row{id, len(s.subs), s.started, s.bytes.Load(), s.name})
		}
	}
	h.mu.Unlock()
	out := []map[string]any{}
	for _, r := range rows {
		secs := time.Since(r.since).Seconds()
		kbps := 0.0
		if secs > 1 {
			kbps = float64(r.bytes) * 8 / 1000 / secs
		}
		n := r.name
		if n == "" {
			n = h.a.chanName(r.id)
		}
		out = append(out, map[string]any{"id": r.id, "n": n, "v": r.v,
			"secs": int(secs), "kbps": int(kbps)})
	}
	return out
}

// hLiveStream sirve el canal como MPEG-TS continuo (reproducible con mpegts.js, ExoPlayer, VLC…).
func (a *App) hLiveStream(w http.ResponseWriter, r *http.Request) {
	t0 := time.Now()
	id, err := strconv.Atoi(strings.TrimSuffix(r.PathValue("id"), ".ts"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	ls, sub, busy, err := a.hub.acquire(id)
	if err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	if busy != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(409)
		fmt.Fprintf(w, `{"error":"busy"}`)
		return
	}
	defer a.hub.release(ls, sub)
	select {
	case <-ls.ready:
	case <-r.Context().Done():
		return
	}
	if ls.err != nil {
		jsonErr(w, 502, ls.err.Error())
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	rc := http.NewResponseController(w)
	tReady := time.Since(t0)
	if mode := r.URL.Query().Get("clean"); (mode == "1" || mode == "2") && a.ffmpeg != "" {
		a.cleanLive(w, r, rc, sub, mode == "2", id, tReady)
		return
	}
	w.Header().Set("Content-Type", "video/mp2t")
	for {
		select {
		case b, ok := <-sub.ch:
			if !ok {
				return
			}
			sub.take(b)
			// un cliente que no lee (TV apagada, wifi caído) no debe retener la conexión del proveedor
			_ = rc.SetWriteDeadline(time.Now().Add(20 * time.Second))
			if _, err := w.Write(b); err != nil {
				return
			}
			_ = rc.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

// cleanLive re-empaqueta el directo sin recodificar el vídeo: solo el primer vídeo y el primer audio (pasado a AAC), sin
// subtítulos DVB ni tablas extrañas. Algunos proveedores emiten TS que el navegador no digiere tal cual; limpio sí. No toca la conexión al proveedor.
func (a *App) cleanLive(w http.ResponseWriter, r *http.Request, rc *http.ResponseController, sub *Sub, fmp4 bool, id int, tReady time.Duration) {
	t1 := time.Now()
	first := true
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin",
		"-fflags", "+genpts+discardcorrupt+nobuffer", "-analyzeduration", "700000", "-probesize", "1200000",
		"-f", "mpegts", "-i", "pipe:0", "-map", "0:v:0", "-map", "0:a:0?", "-c:v", "copy",
		// el audio se pasa siempre a AAC estéreo: muchos canales emiten AC3/E-AC3 (Dolby) o MP2, que los navegadores de
		// escritorio no decodifican (vídeo negro o «could not open»); convertir solo el audio apenas gasta CPU
		"-c:a", "aac", "-b:a", "160k", "-ac", "2", "-af", "aresample=async=1", "-sn", "-dn"}
	if fmp4 {
		// MP4 fragmentado (como hace Jellyfin): el <video> lo reproduce sin mpegts.js
		w.Header().Set("Content-Type", "video/mp4")
		args = append(args, "-movflags", "frag_keyframe+empty_moov+default_base_moof", "-frag_duration", "1000000", "-flush_packets", "1", "-f", "mp4", "pipe:1")
	} else {
		w.Header().Set("Content-Type", "video/mp2t")
		args = append(args, "-muxdelay", "0", "-flush_packets", "1", "-f", "mpegts", "pipe:1")
	}
	cmd := exec.CommandContext(ctx, a.ffmpeg, args...)
	in, err := cmd.StdinPipe()
	if err != nil {
		return
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return
	}
	if err := cmd.Start(); err != nil {
		log.Printf("directo: no se pudo lanzar ffmpeg: %v", err)
		return
	}
	defer func() { cancel(); _ = cmd.Wait() }()
	go func() {
		defer in.Close()
		for {
			select {
			case b, ok := <-sub.ch:
				if !ok {
					return
				}
				sub.take(b)
				if _, err := in.Write(b); err != nil {
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	buf := make([]byte, 64<<10)
	for {
		n, err := out.Read(buf)
		if n > 0 {
			if first {
				first = false
				log.Printf("directo: canal %d: proveedor listo en %.1fs, primer dato de vídeo %.1fs después", id, tReady.Seconds(), time.Since(t1).Seconds())
			}
			_ = rc.SetWriteDeadline(time.Now().Add(20 * time.Second))
			if _, werr := w.Write(buf[:n]); werr != nil {
				return
			}
			_ = rc.Flush()
		}
		if err != nil {
			return
		}
	}
}
