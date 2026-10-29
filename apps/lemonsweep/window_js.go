//go:build js

package main

import (
	"errors"
	"syscall/js"
	"time"
)

// The browser locks the pointer itself (input_js.go).

func windowHint() string { return "" }

func openWindow([]string) error { return errors.New("-window is for kitty and Apple's Terminal") }

// screenRate is how many animation frames the page gets a second (60, 120,
// 144), counted over a quarter of a second; 0 where there are none (Node,
// which CI runs the module in, or a page in the background).
func screenRate() int {
	if js.Global().Get("requestAnimationFrame").Type() != js.TypeFunction {
		return 0
	}
	done := make(chan int, 1)
	n, start := 0, 0.0
	var frame js.Func
	frame = js.FuncOf(func(_ js.Value, args []js.Value) any {
		t := args[0].Float()
		if n == 0 {
			start = t
		}
		n++
		if t-start >= 250 {
			done <- int(float64(n-1)*1000/(t-start) + 0.5)
			return nil
		}
		js.Global().Call("requestAnimationFrame", frame)
		return nil
	})
	defer frame.Release()
	js.Global().Call("requestAnimationFrame", frame)
	select {
	case r := <-done:
		return r
	case <-time.After(time.Second):
		return 0
	}
}
