package main

import (
	"embed"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

//go:embed web
var webFS embed.FS

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func main() {
	addr := flag.String("addr", envOr("LUMINA_ADDR", ":8780"), "dirección de escucha")
	data := flag.String("data", envOr("LUMINA_DATA", "data"), "carpeta de datos (config, índice, miniaturas)")
	pass := flag.String("password", os.Getenv("LUMINA_PASSWORD"), "contraseña de acceso (opcional)")
	ver := flag.Bool("version", false, "muestra la versión y sale")
	flag.Parse()
	if *ver {
		fmt.Println("Lumina " + version)
		return
	}

	installLog()
	a, err := NewApp(*data, *pass)
	if err != nil {
		log.Fatal(err)
	}
	_ = os.Remove(startExe + ".old") // copia anterior de una actualización en Windows
	mux := http.NewServeMux()
	a.routes(mux)

	srv := &http.Server{
		Addr:              *addr,
		Handler:           a.auth(mux),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	abs, _ := filepath.Abs(*data)
	log.Printf("Lumina %s escuchando en %s (datos en %s)", version, *addr, abs)
	if *pass == "" && !a.accounts() {
		log.Printf("AVISO: sin contraseña. Cualquiera en tu red puede ver el contenido (crea cuentas en Ajustes → Usuarios, o usa -password / LUMINA_PASSWORD)")
	}
	// tras una actualización el proceso anterior puede tardar un instante en liberar el puerto
	var ln net.Listener
	for i := 0; i < 40; i++ {
		if ln, err = net.Listen("tcp", *addr); err == nil {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	if err != nil {
		log.Fatal(err)
	}
	go a.upd.loop()
	log.Fatal(srv.Serve(ln))
}
