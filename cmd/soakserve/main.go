package main

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
)

func main() {
	wasmDir := filepath.Join("cmd", "soakwasm")

	// Copy wasm_exec.js from GOROOT if missing
	dst := filepath.Join(wasmDir, "wasm_exec.js")
	if _, err := os.Stat(dst); os.IsNotExist(err) {
		goroot, err := goEnvGOROOT()
		if err != nil {
			fmt.Fprintf(os.Stderr, "cannot determine GOROOT: %v\n", err)
			os.Exit(1)
		}
		src := filepath.Join(goroot, "lib", "wasm", "wasm_exec.js")
		data, err := os.ReadFile(src)
		if err != nil {
			fmt.Fprintf(os.Stderr, "cannot read wasm_exec.js: %v\n", err)
			os.Exit(1)
		}
		if err := os.WriteFile(dst, data, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "cannot write wasm_exec.js: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("copied wasm_exec.js from GOROOT")
	}

	addr := ":8080"
	fmt.Printf("serving %s on http://localhost%s\n", wasmDir, addr)
	if err := http.ListenAndServe(addr, http.FileServer(http.Dir(wasmDir))); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}

func goEnvGOROOT() (string, error) {
	out, err := exec.Command("go", "env", "GOROOT").Output()
	if err != nil {
		return "", err
	}
	return string(bytes.TrimSpace(out)), nil
}
