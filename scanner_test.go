package main

import (
	"strings"
	"testing"
)

func TestNewerVer(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"v0.3.0", "0.2.9", true}, {"v1.0.0", "v1.0.0", false}, {"v1.10.0", "v1.9.9", true},
		{"v1.0", "v1.0.1", false}, {"v2.0.0-rc1", "v1.9.0", true}, {"dev", "v1.0.0", false}, {"v1.0.0", "dev", false},
	}
	for _, c := range cases {
		if got := newerVer(c.a, c.b); got != c.want {
			t.Errorf("newerVer(%q,%q)=%v, esperado %v", c.a, c.b, got, c.want)
		}
	}
}

func TestTitleYear(t *testing.T) {
	cases := []struct {
		in    string
		title string
		year  int
	}{
		{"Blade Runner 2049 2017", "Blade Runner 2049", 2017},
		{"La Gran Evasion (1963)", "La Gran Evasion", 1963},
		{"Pelicula.Rara.1999.1080p.BluRay.x264", "Pelicula Rara", 1999},
	}
	for _, c := range cases {
		ti, y := titleYear(c.in)
		if ti != c.title || y != c.year {
			t.Errorf("titleYear(%q)=(%q,%d), esperado (%q,%d)", c.in, ti, y, c.title, c.year)
		}
	}
}

func TestShiftVTT(t *testing.T) {
	in := "WEBVTT\n\n00:00:01.000 --> 00:00:03.000\nUno\n\n00:00:05.000 --> 00:00:08.000 align:start\nDos\n"
	out := shiftVTT(in, 2)
	if strings.Contains(out, "Uno") == false || !strings.Contains(out, "00:00:00.000 --> 00:00:01.000") {
		t.Errorf("el primer cue debería recortarse a 0-1 s:\n%s", out)
	}
	if !strings.Contains(out, "00:00:03.000 --> 00:00:06.000 align:start") {
		t.Errorf("el segundo cue debería desplazarse 2 s conservando ajustes:\n%s", out)
	}
	if strings.Contains(shiftVTT(in, 4), "Uno") {
		t.Error("un cue anterior al punto de inicio debe descartarse")
	}
}

func TestAssetNames(t *testing.T) {
	if !strings.HasPrefix(assetName(), "lumina-") {
		t.Error("nombre de binario inesperado: " + assetName())
	}
}

func TestUserPerms(t *testing.T) {
	var none *User
	if !none.libOK("x") || !none.liveOK() || !none.isAdmin() {
		t.Fatal("sin cuentas todo debe estar permitido")
	}
	u := &User{Libs: []string{"a"}}
	if !u.libOK("a") || u.libOK("b") || u.liveOK() || u.isAdmin() {
		t.Fatal("permisos de usuario limitado incorrectos")
	}
	if !(&User{AllLibs: true}).libOK("z") || !(&User{Admin: true}).libOK("z") {
		t.Fatal("todas las bibliotecas / admin")
	}
	s := randHex(8)
	h := hashPass("clave123", s)
	us := &User{Salt: s, Hash: h}
	if h == "" || !us.checkPass("clave123") || us.checkPass("otra") {
		t.Fatal("hash de contraseña")
	}
}

func TestBroadcastKicksSlowClient(t *testing.T) {
	h := &Hub{streams: map[int]*LiveStream{}}
	ls := &LiveStream{id: 1, hub: h, subs: map[*Sub]struct{}{}}
	slow := &Sub{ch: make(chan []byte, 2)}
	fast := &Sub{ch: make(chan []byte, 10)}
	ls.subs[slow] = struct{}{}
	ls.subs[fast] = struct{}{}
	for i := 0; i < 3; i++ {
		ls.broadcast([]byte{byte(i)})
	}
	if _, ok := ls.subs[slow]; ok {
		t.Fatal("el cliente lento debía cortarse")
	}
	n := 0
	for range slow.ch { // vacía lo pendiente y comprueba que está cerrado
		n++
	}
	if n != 2 || len(fast.ch) != 3 {
		t.Fatalf("slow=%d fast=%d", n, len(fast.ch))
	}
}

func TestRingReplay(t *testing.T) {
	h := &Hub{streams: map[int]*LiveStream{}}
	ls := &LiveStream{id: 1, hub: h, subs: map[*Sub]struct{}{}}
	chunk := make([]byte, 188*100) // 18,8 KB
	n := (ringMax/len(chunk))*2 + 5
	for i := 0; i < n; i++ {
		ls.broadcast(chunk)
	}
	if ls.ringBytes > ringMax || ls.ringBytes < ringMax-len(chunk) {
		t.Fatalf("ringBytes=%d", ls.ringBytes)
	}
	sub := &Sub{ch: make(chan []byte, 640)}
	ls.replayLocked(sub)
	tot := 0
	close(sub.ch)
	for b := range sub.ch {
		if len(b)%188 != 0 || len(b) > 64<<10 {
			t.Fatalf("trozo no válido: %d", len(b))
		}
		tot += len(b)
	}
	if tot != ls.ringBytes {
		t.Fatalf("repetido %d de %d", tot, ls.ringBytes)
	}
	ls.resetRing()
	if ls.ringBytes != 0 || len(ls.ring) != 0 {
		t.Fatal("el colchón debía vaciarse")
	}
}

func TestBurstNotKicked(t *testing.T) {
	h := &Hub{streams: map[int]*LiveStream{}}
	ls := &LiveStream{id: 1, hub: h, subs: map[*Sub]struct{}{}}
	s := &Sub{ch: make(chan []byte, 16384)}
	ls.subs[s] = struct{}{}
	chunk := make([]byte, 188*90) // ~16 KB: ráfaga inicial de 40 MB sin que el cliente lea nada
	for i := 0; i < 2500; i++ {
		ls.broadcast(chunk)
	}
	if _, ok := ls.subs[s]; !ok {
		t.Fatal("una ráfaga inicial no debe cortar al espectador")
	}
	n := 0
	for len(s.ch) > 0 {
		s.take(<-s.ch)
		n++
	}
	if n != 2500 || s.q.Load() != 0 {
		t.Fatalf("n=%d q=%d", n, s.q.Load())
	}
	big := make([]byte, 188*1000)
	for i := 0; i < 600; i++ { // 112 MB sin leer: ahora sí se corta
		ls.broadcast(big)
	}
	if _, ok := ls.subs[s]; ok {
		t.Fatal("un cliente que no lee nada en un minuto debe cortarse")
	}
}

func TestVodOccupiesSlot(t *testing.T) {
	a := &App{}
	a.cfg.Xtream.MaxConn = 1
	a.hub = newHub(a)
	ls, sub, busy := a.hub.vodAcquire(-5, "«Serie»", false)
	if busy != nil || ls == nil || sub == nil {
		t.Fatal("el primer vod debería entrar")
	}
	b := a.hub.tune(101, false)
	if len(b) != 1 || b[0].N != "«Serie»" {
		t.Fatalf("el directo debería chocar con el vod: %+v", b)
	}
	if b := a.hub.tune(101, true); b != nil {
		t.Fatal("forzar debe liberar el hueco")
	}
}

func TestStripLang(t *testing.T) {
	for in, want := range map[string]string{
		"ES - La casa de papel": "La casa de papel", "EN – Friends": "Friends", "LAT | Narcos": "Narcos",
		"A - Team": "A - Team", "Breaking Bad": "Breaking Bad", "ES - ": "ES -", "24 - Redención": "24 - Redención",
	} {
		if got := stripLang(in); got != want {
			t.Errorf("%q -> %q (quería %q)", in, got, want)
		}
	}
}
