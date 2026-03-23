//go:build js && wasm

package memcheck

import "runtime/debug"

func init() {
	debug.SetMemoryLimit(900 * 1024 * 1024) // 900 MB
}
