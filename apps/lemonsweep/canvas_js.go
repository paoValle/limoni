//go:build js

package main

import (
	"encoding/binary"
	"image/color"
	"math"
	"syscall/js"

	"github.com/thebanri/limoni"
	"github.com/thebanri/limoni/core/buffer"
	"github.com/thebanri/limoni/core/cell"
)

// In the browser the game paints its cells on two canvases of its own, laid
// over xterm.js, instead of sending them to it as text: the half blocks one
// pixel each in a small image the browser scales up, the rest drawn as -gui
// draws them. xterm.js still takes the keys and the mouse, but has nothing
// to draw. Parsing some 70 KB of escapes a frame and making a glyph for
// every new pair of colours was most of a frame, and in Firefox the most.

type webCanvas struct {
	buf          *buffer.Buffer
	pix, ops     []byte
	jsPix, jsOps js.Value
	paint, clear js.Value
}

// newWebCanvas is nil where there is no page to draw on (Node, which CI
// runs the module in); the game then draws to the terminal.
func newWebCanvas() *webCanvas {
	doc := js.Global().Get("document")
	if doc.Type() != js.TypeObject || doc.Get("querySelector").Type() != js.TypeFunction ||
		js.Global().Get("requestAnimationFrame").Type() != js.TypeFunction {
		return nil
	}
	fn := js.Global().Get("Function")
	return &webCanvas{
		paint: fn.New("pix", "cols", "rows", "ops", "n", paintJS),
		clear: fn.New(clearJS),
	}
}

// frame draws the game into the canvases' own buffer and paints it; the
// terminal's buffer is left blank, so nothing is sent to xterm.js.
func (w *webCanvas) frame(g *game, area cell.Rect) {
	if w.buf == nil || w.buf.Area != area {
		w.buf = buffer.NewBuffer(area)
	}
	w.buf.Clear()
	g.render(w.buf)

	cols, rows := int(area.Width), int(area.Height)
	if n := cols * rows * 2 * 4; len(w.pix) != n {
		w.pix = make([]byte, n)
		w.jsPix = js.Global().Get("Uint8Array").New(n)
	}
	w.ops = w.ops[:0]
	for i := range w.buf.Content {
		c := &w.buf.Content[i]
		top, bot, glyph := cellHalves(c)
		x, y := i%cols, i/cols
		setPixel(w.pix[4*(2*y*cols+x):], top)
		setPixel(w.pix[4*((2*y+1)*cols+x):], bot)
		if glyph {
			fg, _ := cellColors(c)
			w.ops = appendGlyph(w.ops, x, y, c.Content, fg)
		}
	}
	js.CopyBytesToJS(w.jsPix, w.pix)
	if w.jsOps.IsUndefined() || w.jsOps.Get("length").Int() < len(w.ops) {
		w.jsOps = js.Global().Get("Uint8Array").New(max(4096, 2*len(w.ops)))
	}
	js.CopyBytesToJS(w.jsOps, w.ops)
	w.paint.Invoke(w.jsPix, cols, rows, w.jsOps, len(w.ops)/opSize)
}

// wakeOnFrames draws a frame on each of the page's animation frames: as
// many as the screen shows (60, 144), and fewer, never piling up, on a
// machine that cannot keep up.
func (w *webCanvas) wakeOnFrames() {
	var frame js.Func
	frame = js.FuncOf(func(js.Value, []js.Value) any {
		limoni.Wakeup()
		js.Global().Call("requestAnimationFrame", frame)
		return nil
	})
	js.Global().Call("requestAnimationFrame", frame)
}

// close takes the canvases off the page when the game ends.
func (w *webCanvas) close() { w.clear.Invoke() }

func setPixel(p []byte, c color.RGBA) { p[0], p[1], p[2], p[3] = c.R, c.G, c.B, 255 }

// An op is six little-endian int32s: kind, column, row, two arguments and
// the colour as 0xRRGGBBAA.
const opSize = 6 * 4

const (
	opRect     = 1 // a, b: x<<8|y and w<<8|h in eighths of the cell
	opQuadrant = 2 // a: the quarters, 1 upper left, 2 upper right, 4 lower left, 8 lower right
	opLines    = 3 // a: 1 left, 2 right, 4 up, 8 down; b: 1 for heavy
	opText     = 4 // a: the character
)

// appendGlyph is the op that draws r: block, quadrant and box characters by
// hand, as terminals do, so they meet their neighbours exactly; the rest as
// text.
func appendGlyph(ops []byte, x, y int, r rune, fg color.RGBA) []byte {
	op := func(kind, a, b int, alpha float32) []byte {
		for _, v := range [...]uint32{uint32(kind), uint32(x), uint32(y), uint32(a), uint32(b),
			uint32(fg.R)<<24 | uint32(fg.G)<<16 | uint32(fg.B)<<8 | uint32(math.Round(float64(255*alpha)))} {
			ops = binary.LittleEndian.AppendUint32(ops, v)
		}
		return ops
	}
	if bx, by, bw, bh, a, ok := blockRect(r); ok {
		return op(opRect, bx<<8|by, bw<<8|bh, a)
	}
	if q, ok := quadrants[r]; ok {
		return op(opQuadrant, int(q), 0, 1)
	}
	switch r {
	case '━':
		return op(opLines, 1|2, 1, 1)
	case '┃':
		return op(opLines, 4|8, 1, 1)
	}
	if seg, ok := cell.BoxDrawingSegments(r); ok {
		lines := 0
		for bit, s := range [...]uint8{cell.LineLeft, cell.LineRight, cell.LineUp, cell.LineDown} {
			if seg&s != 0 {
				lines |= 1 << bit
			}
		}
		return op(opLines, lines, 0, 1)
	}
	return op(opText, int(r), 0, 1)
}

const paintJS = `
const scr = document.querySelector('#terminal .xterm-screen');
if (!scr) { return; }
let s = scr.lemonsweep;
if (!s) {
  const make = () => {
    const c = document.createElement('canvas');
    c.style.cssText = 'position:absolute;left:0;top:0;width:100%;height:100%;z-index:5;pointer-events:none';
    scr.appendChild(c);
    return c;
  };
  const p = make();
  p.style.imageRendering = 'pixelated';
  const g = make();
  s = scr.lemonsweep = { p, g, pc: p.getContext('2d'), gc: g.getContext('2d'), img: null };
}
const h2 = rows * 2;
if (s.p.width !== cols || s.p.height !== h2) {
  s.p.width = cols;
  s.p.height = h2;
  s.img = new ImageData(cols, h2);
}
s.img.data.set(pix.subarray(0, cols * h2 * 4));
s.pc.putImageData(s.img, 0, 0);

const dpr = window.devicePixelRatio || 1;
const gw = Math.round(scr.clientWidth * dpr), gh = Math.round(scr.clientHeight * dpr);
if (s.g.width !== gw || s.g.height !== gh) {
  s.g.width = gw;
  s.g.height = gh;
}
const c = s.gc;
c.clearRect(0, 0, gw, gh);
const cw = gw / cols, ch = gh / rows, line = Math.max(1, Math.round(dpr));
c.font = Math.round(cw / 0.6) + 'px ui-monospace, Menlo, Monaco, "Courier New", monospace';
c.textAlign = 'center';
c.textBaseline = 'middle';
const o = new Int32Array(ops.buffer, 0, n * 6);
for (let i = 0; i < n * 6; i += 6) {
  const kind = o[i], x = o[i + 1] * cw, y = o[i + 2] * ch, a = o[i + 3], b = o[i + 4], rgba = o[i + 5] >>> 0;
  c.fillStyle = 'rgba(' + (rgba >>> 24) + ',' + (rgba >>> 16 & 255) + ',' + (rgba >>> 8 & 255) + ',' + (rgba & 255) / 255 + ')';
  if (kind === 1) {
    c.fillRect(x + (a >> 8) * cw / 8, y + (a & 255) * ch / 8, (b >> 8) * cw / 8, (b & 255) * ch / 8);
  } else if (kind === 2) {
    for (let q = 0; q < 4; q++) {
      if (a & (1 << q)) { c.fillRect(x + (q & 1) * cw / 2, y + (q >> 1) * ch / 2, cw / 2, ch / 2); }
    }
  } else if (kind === 3) {
    const w = b ? 2 * line : line, mx = Math.floor(cw / 2 - w / 2), my = Math.floor(ch / 2 - w / 2);
    if (a & 1) { c.fillRect(x, y + my, mx + w, w); }
    if (a & 2) { c.fillRect(x + mx, y + my, cw - mx, w); }
    if (a & 4) { c.fillRect(x + mx, y, w, my + w); }
    if (a & 8) { c.fillRect(x + mx, y + my, w, ch - my); }
  } else {
    c.fillText(String.fromCodePoint(a), x + cw / 2, y + ch / 2);
  }
}
`

const clearJS = `
const scr = document.querySelector('#terminal .xterm-screen');
if (scr && scr.lemonsweep) {
  scr.lemonsweep.p.remove();
  scr.lemonsweep.g.remove();
  delete scr.lemonsweep;
}
`
