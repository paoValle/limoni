//go:build js

package main

import "errors"

// The browser locks the pointer itself (input_js.go).

func windowHint() string { return "" }

func openWindow([]string) error { return errors.New("-window is for kitty and Apple's Terminal") }
