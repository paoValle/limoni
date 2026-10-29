//go:build js

package main

import (
	"errors"
	"syscall/js"

	"github.com/thebanri/limoni"
)

// The browser locks the pointer itself (input_js.go).

func windowHint() string { return "" }

func openWindow([]string) error { return errors.New("-window is for kitty and Apple's Terminal") }

// behind reports whether xterm.js has yet to show the last frame sent: the
// game and xterm.js share one thread, and a frame drawn before the last is
// shown only takes time from showing it (examples/wasm/index.html).
func behind() bool {
	f := js.Global().Get("__limoni_behind")
	return f.Type() == js.TypeFunction && f.Invoke().Bool()
}

// The next frame is drawn the moment xterm.js has shown the last, not at
// the next tick, which would beat against the screen's own.
func init() {
	js.Global().Set("__limoni_drawn", js.FuncOf(func(js.Value, []js.Value) any {
		limoni.Wakeup()
		return nil
	}))
}
