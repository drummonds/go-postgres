//go:build !wasip1

package pglike

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// goroot returns GOROOT via "go env GOROOT".
func goroot(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("go", "env", "GOROOT").Output()
	if err != nil {
		t.Fatalf("go env GOROOT: %v", err)
	}
	return strings.TrimSpace(string(out))
}

// runWASMTest cross-compiles and runs a specific test under wasip1/wasm.
// Returns the combined output and any error.
func runWASMTest(t *testing.T, testName string) (string, error) {
	t.Helper()

	if _, err := exec.LookPath("wazero"); err != nil {
		t.Skip("wazero not in PATH")
	}

	wasmExec := filepath.Join(goroot(t), "lib", "wasm", "go_wasip1_wasm_exec")
	if _, err := os.Stat(wasmExec); err != nil {
		t.Skipf("go_wasip1_wasm_exec not found: %v", err)
	}

	dir := t.TempDir()
	wasmBin := filepath.Join(dir, "pglike.test.wasm")

	// Cross-compile the test binary for wasip1/wasm.
	build := exec.Command("go", "test", "-c", "-o", wasmBin, ".")
	build.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm")
	build.Dir = "."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("cross-compile failed: %v\n%s", err, out)
	}

	run := exec.Command(wasmExec, wasmBin,
		"-test.run", "^"+testName+"$",
		"-test.v",
		"-test.count=1",
	)
	run.Env = append(os.Environ(), "GOWASIRUNTIME=wazero")
	out, err := run.CombinedOutput()
	return string(out), err
}

// TestWASMBasicQuery verifies that basic :memory: usage works under WASM
// (single connection, no pooling).
func TestWASMBasicQuery(t *testing.T) {
	output, err := runWASMTest(t, "TestDriverCreateTableAndInsert")
	if err != nil {
		t.Fatalf("basic query should work under WASM:\n%s", output)
	}
}

// TestWASMMemoryPoolSharing verifies that :memory: connection pool sharing
// works under WASM via the shared single-connection fallback.
func TestWASMMemoryPoolSharing(t *testing.T) {
	output, err := runWASMTest(t, "TestDriverMemoryPoolSharing")
	if err != nil {
		t.Fatalf("pool sharing should work under WASM:\n%s", output)
	}
}
