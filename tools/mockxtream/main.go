// mockxtream: servidor Xtream Codes simulado para probar Lumina sin proveedor real.
// Uso: go run ./tools/mockxtream -addr :9080   (usuario "demo", contraseña "demo", 1 conexión)
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

var cats = []string{"España", "Deportes", "Películas", "Noticias"}
var names = []string{"Canal Uno", "Dos TV", "Tres Plus", "Cuatro Max", "Sol TV", "Luna HD", "Mar Directo", "Cielo", "Monte TV", "Valle"}

type ch struct {
	ID   int
	Name string
	Cat  int
}

var chans []ch

var active atomic.Int32

const maxConn = 1

func main() {
	addr := flag.String("addr", ":9080", "dirección")
	flag.DurationVar(&slow, "slow", 0, "retardo artificial al listar series de una categoría")
	flag.Parse()
	for i := 1; i <= 40; i++ {
		c := (i - 1) % len(cats)
		chans = append(chans, ch{100 + i, fmt.Sprintf("%s %s %d", names[(i-1)%len(names)], []string{"HD", "FHD", "4K"}[i%3], i), c})
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/player_api.php", api)
	mux.HandleFunc("/xmltv.php", xmltv)
	mux.HandleFunc("/icon/", icon)
	mux.HandleFunc("/live/", live)
	mux.HandleFunc("/series/", seriesFile)
	log.Printf("mockxtream en %s (demo/demo, %d canales, max %d conexión)", *addr, len(chans), maxConn)
	log.Fatal(http.ListenAndServe(*addr, mux))
}

func auth(r *http.Request) bool {
	q := r.URL.Query()
	return q.Get("username") == "demo" && q.Get("password") == "demo"
}

func api(w http.ResponseWriter, r *http.Request) {
	if !auth(r) {
		w.Write([]byte(`{"user_info":{"auth":0}}`))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	switch r.URL.Query().Get("action") {
	case "get_live_categories":
		var out []map[string]any
		for i, c := range cats {
			out = append(out, map[string]any{"category_id": strconv.Itoa(i + 1), "category_name": c, "parent_id": 0})
		}
		json.NewEncoder(w).Encode(out)
	case "get_live_streams":
		var out []map[string]any
		for _, c := range chans {
			out = append(out, map[string]any{"num": c.ID, "name": c.Name, "stream_type": "live", "stream_id": c.ID,
				"stream_icon": fmt.Sprintf("http://%s/icon/%d.png", r.Host, c.ID), "epg_channel_id": fmt.Sprintf("ch%d", c.ID),
				"category_id": strconv.Itoa(c.Cat + 1), "tv_archive": 0})
		}
		json.NewEncoder(w).Encode(out)
	case "get_series_categories":
		json.NewEncoder(w).Encode([]map[string]any{{"category_id": "11", "category_name": "Series | Drama", "parent_id": 0}, {"category_id": "12", "category_name": "Series | Comedia", "parent_id": 0}})
	case "get_series":
		time.Sleep(slow)
		cid := r.URL.Query().Get("category_id")
		var out []map[string]any
		for id := 1; id <= 6; id++ {
			c := "11"
			if id%2 == 0 {
				c = "12"
			}
			if cid != "" && cid != c {
				continue
			}
			out = append(out, map[string]any{"series_id": id, "name": fmt.Sprintf("Serie de prueba %d", id), "cover": fmt.Sprintf("http://%s/icon/%d.png", r.Host, id),
				"plot": "Una serie de ejemplo.", "releaseDate": "2020-01-01", "rating": "7.5", "category_id": c})
		}
		json.NewEncoder(w).Encode(out)
	case "get_series_info":
		sid, _ := strconv.Atoi(r.URL.Query().Get("series_id"))
		eps := map[string][]map[string]any{}
		for sn := 1; sn <= 2; sn++ {
			for e := 1; e <= 3; e++ {
				eps[strconv.Itoa(sn)] = append(eps[strconv.Itoa(sn)], map[string]any{"id": strconv.Itoa(sid*100 + sn*10 + e), "episode_num": e, "title": fmt.Sprintf("Episodio %d", e),
					"container_extension": "mp4", "season": sn, "info": map[string]any{"duration_secs": 20, "movie_image": fmt.Sprintf("http://%s/icon/%d.png", r.Host, sid)}})
			}
		}
		json.NewEncoder(w).Encode(map[string]any{"info": map[string]any{"name": fmt.Sprintf("Serie de prueba %d", sid), "cover": fmt.Sprintf("http://%s/icon/%d.png", r.Host, sid)}, "episodes": eps})
	default:
		json.NewEncoder(w).Encode(map[string]any{
			"user_info": map[string]any{"username": "demo", "auth": 1, "status": "Active", "exp_date": strconv.FormatInt(time.Now().Add(90*24*time.Hour).Unix(), 10),
				"active_cons": strconv.Itoa(int(active.Load())), "max_connections": strconv.Itoa(maxConn)},
			"server_info": map[string]any{"url": r.Host},
		})
	}
}

func xmltv(w http.ResponseWriter, r *http.Request) {
	if !auth(r) {
		http.Error(w, "forbidden", 403)
		return
	}
	w.Header().Set("Content-Type", "application/xml")
	fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?><tv generator-info-name="mock">`)
	for _, c := range chans {
		fmt.Fprintf(w, `<channel id="ch%d"><display-name>%s</display-name></channel>`, c.ID, c.Name)
	}
	base := time.Now().UTC().Truncate(time.Hour).Add(-24 * time.Hour)
	titles := []string{"Informativo", "Cine de la tarde", "El gran debate", "Documental: Océanos", "Liga: resumen", "Magacín & más", "Serie: La casa", "Noche de estreno"}
	for _, c := range chans {
		for k := 0; k < 30; k++ {
			s := base.Add(time.Duration(k) * 90 * time.Minute)
			e := s.Add(90 * time.Minute)
			t := titles[(k+c.ID)%len(titles)]
			fmt.Fprintf(w, `<programme start="%s +0000" stop="%s +0000" channel="ch%d"><title lang="es">%s</title><desc lang="es">Emisión número %d de %s con descripción de ejemplo.</desc></programme>`,
				s.Format("20060102150405"), e.Format("20060102150405"), c.ID, t, k, c.Name)
		}
	}
	fmt.Fprint(w, `</tv>`)
}

func icon(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/icon/"), ".png"))
	img := image.NewRGBA(image.Rect(0, 0, 96, 96))
	c := color.RGBA{uint8(40 + id*37%180), uint8(60 + id*53%160), uint8(90 + id*29%150), 255}
	for y := 0; y < 96; y++ {
		for x := 0; x < 96; x++ {
			if (x-48)*(x-48)+(y-48)*(y-48) < 44*44 {
				img.Set(x, y, c)
			}
		}
	}
	w.Header().Set("Content-Type", "image/png")
	png.Encode(w, img)
}

func live(w http.ResponseWriter, r *http.Request) {
	p := strings.Split(strings.TrimPrefix(r.URL.Path, "/live/"), "/")
	if len(p) != 3 || p[0] != "demo" || p[1] != "demo" {
		http.Error(w, "forbidden", 403)
		return
	}
	id, _ := strconv.Atoi(strings.TrimSuffix(p[2], ".ts"))
	if active.Add(1) > maxConn {
		active.Add(-1)
		log.Printf("RECHAZADO canal %d: límite de conexiones", id)
		http.Error(w, "max connections", 403)
		return
	}
	log.Printf("ABIERTO canal %d (activas=%d)", id, active.Load())
	defer func() {
		time.Sleep(1200 * time.Millisecond) // el proveedor tarda en liberar la conexión
		active.Add(-1)
		log.Printf("CERRADO canal %d (activas=%d)", id, active.Load())
	}()
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	cmd := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-loglevel", "error", "-re",
		"-f", "lavfi", "-i", "testsrc2=size=640x360:rate=25",
		"-f", "lavfi", "-i", fmt.Sprintf("sine=frequency=%d", 300+id*7),
		"-vf", fmt.Sprintf("hue=h=%d", id*9),
		"-c:v", "libx264", "-preset", "ultrafast", "-tune", "zerolatency", "-g", "25", "-pix_fmt", "yuv420p", "-b:v", "700k",
		"-c:a", "aac", "-b:a", "64k", "-f", "mpegts", "-muxdelay", "0", "pipe:1")
	out, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "video/mp2t")
	buf := make([]byte, 32<<10)
	fl, _ := w.(http.Flusher)
	for {
		n, err := out.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				break
			}
			if fl != nil {
				fl.Flush()
			}
		}
		if err != nil {
			break
		}
	}
	cancel()
	cmd.Wait()
}

var sample string
var slow time.Duration

// seriesFile sirve un MP4 de prueba (con Range) para cualquier episodio.
func seriesFile(w http.ResponseWriter, r *http.Request) {
	p := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(p) < 4 || p[1] != "demo" || p[2] != "demo" {
		http.Error(w, "forbidden", 403)
		return
	}
	if sample == "" {
		f := os.TempDir() + "/mockxtream-sample.mp4"
		if _, err := os.Stat(f); err != nil {
			if out, err := exec.Command("ffmpeg", "-y", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc=size=640x360:rate=25:duration=20", "-f", "lavfi", "-i", "sine=frequency=440:duration=20",
				"-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", "-movflags", "+faststart", "-shortest", f).CombinedOutput(); err != nil {
				http.Error(w, string(out), 500)
				return
			}
		}
		sample = f
	}
	http.ServeFile(w, r, sample)
}
