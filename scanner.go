package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type ScanState struct {
	Running bool   `json:"running"`
	Lib     string `json:"lib"`
	Found   int    `json:"found"`
	Pending int    `json:"pending"` // elementos pendientes de analizar (duración/miniatura)
	Last    int64  `json:"last"`
}

var videoExt = map[string]bool{".mp4": true, ".mkv": true, ".avi": true, ".mov": true, ".m4v": true,
	".webm": true, ".ts": true, ".m2ts": true, ".mpg": true, ".mpeg": true, ".wmv": true}
var imgExt = map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".webp": true}

var (
	reSE        = regexp.MustCompile(`(?i)(?:^|[\s._-])s(\d{1,2})[\s._-]*e(\d{1,3})(?:$|[\s._-])`)
	reX         = regexp.MustCompile(`(?i)(?:^|[\s._-])(\d{1,2})x(\d{2,3})(?:$|[\s._-])`)
	reYear      = regexp.MustCompile(`\b((?:19|20)\d{2})\b`)
	reTag       = regexp.MustCompile(`(?i)(?:^|[\s._-])(2160p|1080[pi]|720p|480p|4k|uhd|bluray|blu-ray|brrip|bdrip|bdremux|webrip|web-dl|webdl|hdtv|dvdrip|dvdscr|x ?264|x ?265|h ?26[45]|hevc|xvid|divx|remux|proper|repack|extended|unrated|multi|dual|castellano|latino|vose|hdr|hdr10)(?:$|[\s._-]).*`)
	reSeasonDir = regexp.MustCompile(`(?i)^(season|temporada|serie|s)\s*\d+$`)
	reSpace     = regexp.MustCompile(`\s+`)
	sepRepl     = strings.NewReplacer(".", " ", "_", " ")
)

func cleanTitle(s string) string {
	s = sepRepl.Replace(s)
	if loc := reTag.FindStringIndex(s); loc != nil {
		s = s[:loc[0]]
	}
	s = reSpace.ReplaceAllString(s, " ")
	return strings.Trim(s, " -([{")
}

// titleYear separa "Nombre (1999) 1080p" en ("Nombre", 1999).
func titleYear(base string) (string, int) {
	s := sepRepl.Replace(base)
	year := 0
	ms := reYear.FindAllStringIndex(s, -1)
	for i := len(ms) - 1; i >= 0; i-- {
		m := ms[i]
		before := strings.Trim(s[:m[0]], " -([{")
		if before != "" {
			year, _ = strconv.Atoi(s[m[0]:m[1]])
			s = before
			break
		}
	}
	return cleanTitle(s), year
}

type parsed struct {
	kind, title, series string
	year, season, ep    int
}

func parseName(lib Library, root, p string) parsed {
	base := strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))
	rel, _ := filepath.Rel(root, p)
	parts := strings.Split(filepath.ToSlash(rel), "/")

	if lib.Type != "series" {
		t, y := titleYear(base)
		if t == "" {
			t = base
		}
		return parsed{kind: "movie", title: t, year: y}
	}

	s := sepRepl.Replace(base)
	pr := parsed{kind: "episode"}
	var before, after string
	if m := reSE.FindStringSubmatchIndex(s); m != nil {
		pr.season, _ = strconv.Atoi(s[m[2]:m[3]])
		pr.ep, _ = strconv.Atoi(s[m[4]:m[5]])
		before, after = cleanTitle(s[:m[0]]), cleanTitle(s[m[1]:])
	} else if m := reX.FindStringSubmatchIndex(s); m != nil {
		pr.season, _ = strconv.Atoi(s[m[2]:m[3]])
		pr.ep, _ = strconv.Atoi(s[m[4]:m[5]])
		before, after = cleanTitle(s[:m[0]]), cleanTitle(s[m[1]:])
	} else {
		after = cleanTitle(s)
	}
	// nombre de la serie: carpeta de primer nivel si existe; si no, texto previo al SxxExx
	if len(parts) >= 2 {
		pr.series, pr.year = titleYear(parts[0])
	}
	if pr.series == "" {
		pr.series = before
	}
	if pr.series == "" {
		pr.series = after
	}
	if pr.series == "" {
		pr.series = "Sin título"
	}
	pr.title = after
	if pr.title == "" || strings.EqualFold(pr.title, pr.series) {
		if pr.ep > 0 {
			pr.title = fmt.Sprintf("Episodio %d", pr.ep)
		} else {
			pr.title = base
		}
	}
	return pr
}

func findArt(imgs map[string]string, base string) string {
	for _, suf := range []string{"-fanart", "-backdrop", "-landscape", "-thumb", "-poster", ""} {
		for _, ext := range []string{".jpg", ".jpeg", ".png", ".webp"} {
			if p, ok := imgs[strings.ToLower(base+suf+ext)]; ok {
				return p
			}
		}
	}
	return ""
}

func findDirArt(imgs map[string]string) string {
	for _, n := range []string{"fanart", "backdrop", "landscape", "thumb", "folder", "cover", "poster"} {
		for _, ext := range []string{".jpg", ".jpeg", ".png", ".webp"} {
			if p, ok := imgs[n+ext]; ok {
				return p
			}
		}
	}
	return ""
}

// ---------- escaneo ----------

func (a *App) startScan(libID string) bool {
	a.mu.Lock()
	if a.scan.Running {
		a.mu.Unlock()
		return false
	}
	a.scan = ScanState{Running: true, Lib: libID}
	libs := append([]Library(nil), a.cfg.Libraries...)
	a.mu.Unlock()
	go func() {
		for _, l := range libs {
			if l.Type == "cloud" {
				continue
			}
			if libID == "" || l.ID == libID {
				if err := a.scanLib(l); err != nil {
					log.Printf("escaneo de %q: %v", l.Name, err)
				}
			}
		}
		a.rebuild()
		a.saveIndex()
		a.mu.Lock()
		a.scan.Running = false
		a.scan.Last = time.Now().Unix()
		a.mu.Unlock()
	}()
	return true
}

func (a *App) scanLib(lib Library) error {
	root := lib.Path
	if st, err := os.Stat(root); err != nil || !st.IsDir() {
		return fmt.Errorf("la carpeta %q no existe o no es accesible", root)
	}
	type vid struct {
		path      string
		size, mod int64
	}
	var vids []vid
	imgs := map[string]map[string]string{}
	dirVids := map[string]int{}

	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if p != root && (strings.HasPrefix(name, ".") || name == "@eaDir" || name == "$RECYCLE.BIN" || strings.EqualFold(name, "System Volume Information")) {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(name, ".") {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(name))
		dir := filepath.Dir(p)
		if imgExt[ext] {
			m := imgs[dir]
			if m == nil {
				m = map[string]string{}
				imgs[dir] = m
			}
			m[strings.ToLower(name)] = p
			return nil
		}
		if !videoExt[ext] {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		low := strings.ToLower(name)
		if (strings.Contains(low, "sample") || strings.Contains(low, "trailer")) && info.Size() < 100<<20 {
			return nil
		}
		vids = append(vids, vid{p, info.Size(), info.ModTime().Unix()})
		dirVids[dir]++
		a.mu.Lock()
		a.scan.Found = len(vids)
		a.mu.Unlock()
		return nil
	})

	// colecciones de películas: subcarpetas de primer nivel con >= 4 títulos
	colCount := map[string]int{}
	firstDir := func(p string) string {
		rel, _ := filepath.Rel(root, p)
		parts := strings.Split(filepath.ToSlash(rel), "/")
		if len(parts) >= 2 {
			return parts[0]
		}
		return ""
	}
	if lib.Type != "series" {
		for _, v := range vids {
			if d := firstDir(v.path); d != "" {
				colCount[d]++
			}
		}
	}

	now := time.Now().Unix()
	seen := map[string]bool{}
	var fresh []string
	for _, v := range vids {
		id := shortHash(lib.ID + "|" + filepath.ToSlash(v.path))
		seen[id] = true
		pr := parseName(lib, root, v.path)
		base := strings.TrimSuffix(filepath.Base(v.path), filepath.Ext(v.path))
		dir := filepath.Dir(v.path)

		it := &Item{ID: id, Lib: lib.ID, Kind: pr.kind, Title: pr.title, Year: pr.year, Series: pr.series,
			Season: pr.season, Episode: pr.ep, Path: v.path, Size: v.size, Mod: v.mod, Added: v.mod}
		if pr.kind == "episode" {
			it.SID = shortHash(lib.ID + "|" + strings.ToLower(pr.series))
			it.Art = findArt(imgs[dir], base)
			if fd := firstDir(v.path); fd != "" {
				it.SArt = findDirArt(imgs[filepath.Join(root, fd)])
			}
		} else {
			if fd := firstDir(v.path); fd != "" && colCount[fd] >= 4 {
				it.Collection = cleanTitle(fd)
			}
			it.Art = findArt(imgs[dir], base)
			if it.Art == "" && dir != root && dirVids[dir] == 1 {
				it.Art = findDirArt(imgs[dir])
			}
		}

		a.mu.Lock()
		if old := a.items[id]; old != nil && old.Size == v.size && old.Mod == v.mod {
			it.Dur, it.VC, it.AC, it.Probed, it.Added = old.Dur, old.VC, old.AC, old.Probed, old.Added
			it.PV, it.Tracks = old.PV, old.Tracks
		} else {
			it.Added = now
			if old != nil {
				it.Added = old.Added
			}
		}
		if old := a.items[id]; old == nil || !it.Probed || it.PV < probeVersion {
			fresh = append(fresh, id)
		}
		a.items[id] = it
		a.mu.Unlock()
	}

	// eliminar lo que ya no existe en esta biblioteca
	a.mu.Lock()
	for id, it := range a.items {
		if it.Lib == lib.ID && !seen[id] {
			delete(a.items, id)
		}
	}
	a.mu.Unlock()
	for _, id := range fresh {
		a.enqueue(id)
	}
	return nil
}

// ---------- análisis en segundo plano ----------

func (a *App) enqueue(id string) {
	select {
	case a.aq <- id:
	default:
	}
}

func (a *App) analysisLoop() {
	for id := range a.aq {
		a.mu.RLock()
		it := a.items[id]
		a.mu.RUnlock()
		if it == nil {
			continue
		}
		if !it.Probed || it.PV < probeVersion {
			a.probe(it)
		}
		a.ensureThumb(it, 640, false)
		if len(a.aq) == 0 {
			a.later("index", 4*time.Second, a.saveIndex)
		}
	}
}

func (a *App) probe(it *Item) {
	var dur float64
	var vc, ac string
	var tracks []Track
	if a.ffprobe != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
		out, err := exec.CommandContext(ctx, a.ffprobe, "-v", "error", "-print_format", "json",
			"-show_entries", "format=duration:stream=codec_type,codec_name,channels:stream_tags=language,title:stream_disposition=default", it.Path).Output()
		cancel()
		if err == nil {
			var r struct {
				Format  struct{ Duration string } `json:"format"`
				Streams []struct {
					T    string `json:"codec_type"`
					N    string `json:"codec_name"`
					Ch   int    `json:"channels"`
					Tags struct {
						Lang  string `json:"language"`
						Title string `json:"title"`
					} `json:"tags"`
					Disp struct {
						Default int `json:"default"`
					} `json:"disposition"`
				} `json:"streams"`
			}
			if json.Unmarshal(out, &r) == nil {
				dur, _ = strconv.ParseFloat(r.Format.Duration, 64)
				na, ns := 0, 0
				for _, s := range r.Streams {
					switch s.T {
					case "video":
						if vc == "" {
							vc = s.N
						}
					case "audio":
						if ac == "" {
							ac = s.N
						}
						tracks = append(tracks, Track{"a", na, s.Tags.Lang, s.Tags.Title, s.N, s.Ch, s.Disp.Default == 1})
						na++
					case "subtitle":
						tracks = append(tracks, Track{"s", ns, s.Tags.Lang, s.Tags.Title, s.N, 0, s.Disp.Default == 1})
						ns++
					}
				}
			}
		}
	}
	a.mu.Lock()
	it.Dur, it.VC, it.AC, it.Probed, it.PV, it.Tracks = dur, vc, ac, true, probeVersion, tracks
	a.mu.Unlock()
}

// thumbPath devuelve la ruta de la miniatura en caché.
func (a *App) thumbPath(id string, w int, series bool) string {
	s := ""
	if series {
		s = "s"
	}
	return filepath.Join(a.dir, "thumbs", fmt.Sprintf("%s%s_%d.jpg", id, s, w))
}

// ensureThumb genera (si falta) la miniatura y devuelve su ruta; "" si no se pudo.
func (a *App) ensureThumb(it *Item, w int, series bool) string {
	out := a.thumbPath(it.ID, w, series)
	if _, err := os.Stat(out); err == nil {
		return out
	}
	if a.ffmpeg == "" {
		return ""
	}
	mi, _ := a.thumbMu.LoadOrStore(out, new(sync.Mutex))
	l := mi.(*sync.Mutex)
	l.Lock()
	defer l.Unlock()
	if _, err := os.Stat(out); err == nil {
		return out
	}
	a.thumbSem <- struct{}{}
	defer func() { <-a.thumbSem }()

	a.mu.RLock()
	art, sart, dur, path := it.Art, it.SArt, it.Dur, it.Path
	a.mu.RUnlock()
	if series && sart != "" {
		art = sart
	}
	tmp := out + ".tmp.jpg"
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	vf := fmt.Sprintf("scale=%d:-2:flags=fast_bilinear", w)
	var args []string
	if art != "" {
		args = []string{"-hide_banner", "-loglevel", "error", "-y", "-i", art, "-frames:v", "1", "-vf", vf, "-q:v", "4", tmp}
	} else {
		t := 30.0
		if dur > 0 {
			t = dur * 0.12
			if t > 900 {
				t = 900
			}
			if t < 2 {
				t = 0
			}
		}
		args = []string{"-hide_banner", "-loglevel", "error", "-y", "-ss", fmt.Sprintf("%.1f", t), "-i", path,
			"-frames:v", "1", "-vf", vf, "-q:v", "5", tmp}
	}
	if err := exec.CommandContext(ctx, a.ffmpeg, args...).Run(); err != nil {
		_ = os.Remove(tmp)
		return ""
	}
	if st, err := os.Stat(tmp); err != nil || st.Size() == 0 {
		_ = os.Remove(tmp)
		return ""
	}
	_ = os.Rename(tmp, out)
	return out
}
