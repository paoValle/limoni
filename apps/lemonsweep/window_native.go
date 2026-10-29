//go:build !js

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/thebanri/limoni/core/cell"
)

// No program running in a terminal can hide the mouse pointer: kitty has no
// escape for it, and on Wayland nothing inside the terminal may lock it.
// kitty's own mouse_hide_wait can, though: hidden once the mouse rests for
// 50 ms, and shown again only after a billion mouse events. -window opens
// the game in a kitty window of its own, full screen, with that set.
const hiddenPointer = "mouse_hide_wait=0.05 1000 1000000 no"

// fastRepaint lets kitty draw as often as a fast screen refreshes: its
// default waits 10 ms between frames, about 100 a second at most.
const fastRepaint = "repaint_delay=4"

// inOwnWindow reports whether this is the game -window opened.
func inOwnWindow() bool { return os.Getenv("LEMONSWEEP_WINDOW") == "1" }

// windowHint is the line the title screen shows in kitty, where -window
// would hide the pointer, and in Apple's Terminal, which -gui leaves behind.
func windowHint() string {
	if inOwnWindow() {
		return ""
	}
	if appleTerminal() {
		return "lemonsweep -gui: a window of its own, keys that never stick, the mouse locked"
	}
	if !strings.Contains(os.Getenv("TERM"), "kitty") {
		return ""
	}
	if _, err := exec.LookPath("kitty"); err != nil {
		return ""
	}
	return "lemonsweep -window: its own kitty window, full screen, no mouse pointer"
}

// openWindow starts the game again in a kitty window of its own, with the
// rest of its flags, and returns once that window is open.
func openWindow(args []string) error {
	if appleTerminal() {
		return openAppleWindow(args)
	}
	kitty, err := exec.LookPath("kitty")
	if err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return windowCmd(kitty, exe, args).Start()
}

func windowCmd(kitty, exe string, args []string) *exec.Cmd {
	cmd := exec.Command(kitty, append([]string{"-o", hiddenPointer, "-o", fastRepaint, "-o", "input_delay=1", "--start-as=fullscreen", "--title", "Lemon Sweep", exe}, args...)...)
	cmd.Env = append(os.Environ(), "LEMONSWEEP_WINDOW=1")
	return cmd
}

// webCanvas is the browser's (canvas_js.go); a terminal is drawn to.
type webCanvas struct{}

func newWebCanvas() *webCanvas            { return nil }
func (*webCanvas) frame(*game, cell.Rect) {}
func (*webCanvas) close()                 {}
func (*webCanvas) wakeOnFrames()          {}

func appleTerminal() bool { return os.Getenv("TERM_PROGRAM") == "Apple_Terminal" }

// Apple's Terminal draws block glyphs from the font into a row taller than
// the font, so a half block covers about a third of the cell and the
// picture stripes. At a line spacing of 0.75 SF Mono's rows are as tall as
// its glyphs: ▀ is half of a 10-pixel row, █ all of it (measured on macOS
// 26, Terminal 470). -window opens a window with that profile, running the
// game in half blocks.
const appleLineSpacing = "0.75"

// openAppleWindow writes the profile as a .terminal file and opens it,
// which opens a Terminal window with those settings running the game.
func openAppleWindow(args []string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	dir = filepath.Join(dir, "lemonsweep")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, "Lemon Sweep.terminal")
	if err := os.WriteFile(path, appleProfile(exe, args, systemMonoFont()), 0o644); err != nil {
		return err
	}
	return exec.Command("open", path).Run()
}

// systemMonoFont is SF Mono 11, Terminal's default, archived the way a
// profile stores its font; empty if osascript cannot make it, and the
// window then has Terminal's fallback font.
func systemMonoFont() string {
	out, err := exec.Command("osascript", "-l", "JavaScript", "-e", `ObjC.import("AppKit");
$.NSKeyedArchiver.archivedDataWithRootObjectRequiringSecureCodingError($.NSFont.monospacedSystemFontOfSizeWeight(11, 0), false, null).base64EncodedStringWithOptions(0).js`).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// appleProfile is the .terminal file: the line spacing, a large window,
// and the game with the user's own flags, closed when the game quits.
func appleProfile(exe string, args []string, font string) []byte {
	cmd := "LEMONSWEEP_WINDOW=1 " + shellQuote(exe)
	for _, a := range args {
		cmd += " " + shellQuote(a)
	}
	esc := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
	<key>name</key><string>Lemon Sweep</string>
	<key>type</key><string>Window Settings</string>
	<key>FontHeightSpacing</key><real>` + appleLineSpacing + `</real>
	<key>FontWidthSpacing</key><real>1</real>
	<key>columnCount</key><integer>160</integer>
	<key>rowCount</key><integer>50</integer>
	<key>shellExitAction</key><integer>1</integer>
	<key>CommandString</key><string>` + esc.Replace(cmd) + `</string>
	<key>RunCommandAsShell</key><false/>
`)
	if font != "" {
		b.WriteString("\t<key>Font</key><data>" + font + "</data>\n")
	}
	b.WriteString("</dict></plist>\n")
	return []byte(b.String())
}

// shellQuote quotes s for sh: Terminal runs the profile's command through one.
func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }
