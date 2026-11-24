//go:build windows

package main

import (
	"log"
	"os"
	"os/exec"
)

func restartSelf() {
	exe, err := os.Executable()
	if err != nil {
		log.Printf("reinicio: %v", err)
		return
	}
	log.Printf("reiniciando…")
	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		log.Printf("reinicio: %v", err)
		return
	}
	os.Exit(0)
}
