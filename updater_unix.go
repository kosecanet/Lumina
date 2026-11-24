//go:build !windows

package main

import (
	"log"
	"os"
	"syscall"
)

// restartSelf sustituye el proceso por el ejecutable nuevo (mismo PID: systemd/Docker no notan nada).
func restartSelf() {
	exe, err := os.Executable()
	if err != nil {
		log.Printf("reinicio: %v", err)
		return
	}
	log.Printf("reiniciando…")
	if err := syscall.Exec(exe, os.Args, os.Environ()); err != nil {
		log.Printf("reinicio: %v (sal y vuelve a arrancar Lumina a mano)", err)
		os.Exit(0) // un gestor de servicios con Restart=always lo levantará
	}
}
