package terminal

// Benchmarks for screens of many small pictures, the way a tile map draws
// them: tiles that stay put, one tile that changes, and every tile moving.

import (
	"image"
	"testing"

	"github.com/thebanri/limoni/core/cell"
	"github.com/thebanri/limoni/core/driver"
	"github.com/thebanri/limoni/graphics"
)

// discardIO is a terminal that throws its output away.
type discardIO struct{ w, h uint16 }

func (d discardIO) Read(p []byte) (int, error)    { select {} }
func (d discardIO) Write(p []byte) (int, error)   { return len(p), nil }
func (d discardIO) Size() (uint16, uint16, error) { return d.w, d.h, nil }

// tileMove is how many cells the tiles move through before they come back
// to where they started.
const tileMove = 20

// tileMap draws 390 two-cell tiles on a 100×10 screen. With change set one
// tile shows another picture each frame; with move set every tile moves one
// cell right each frame.
type tileMap struct {
	term   *Terminal
	pics   [2]image.Image
	change bool
	move   bool
}

func newTileMap(tb testing.TB, proto graphics.Protocol, change, move bool) *tileMap {
	tb.Setenv("LIMONI_PROBE", "0")
	term, err := New(driver.NewPortableBackend(discardIO{100, 10}))
	if err != nil {
		tb.Fatal(err)
	}
	caps := term.Capabilities()
	caps.GraphicsProto = proto
	term.SetCapabilities(caps)
	return &tileMap{term: term, pics: [2]image.Image{picture(16, 16), picture(16, 16)}, change: change, move: move}
}

func (m *tileMap) frame(tb testing.TB, n int) {
	off := uint16(0)
	if m.move {
		off = uint16(n % tileMove)
	}
	err := m.term.Draw(func(f *Frame) {
		for i := 0; i < 390; i++ {
			area := cell.NewRect(off+uint16(2*(i%39)), uint16(i/39), 2, 1)
			for x := area.X; x < area.X+area.Width; x++ {
				f.Buffer.SetCell(x, area.Y, cell.Cell{Content: cell.RuneImage})
			}
			img := m.pics[0]
			if m.change && i == n%390 {
				img = m.pics[1]
			}
			f.RegisterImageRegion(ImageRegion{Area: area, Img: img})
		}
	})
	if err != nil {
		tb.Fatal(err)
	}
}

// warm draws the frames that send every picture and place every tile at
// each of its positions once, so later frames only reuse them.
func (m *tileMap) warm(tb testing.TB) {
	for n := -1; n < 2*tileMove; n++ {
		m.frame(tb, n)
	}
}

func benchTiles(b *testing.B, proto graphics.Protocol, change, move bool) {
	m := newTileMap(b, proto, change, move)
	m.warm(b)
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		m.frame(b, n)
	}
}

func BenchmarkTilesKittyStill(b *testing.B)  { benchTiles(b, graphics.ProtocolKitty, false, false) }
func BenchmarkTilesKittyChange(b *testing.B) { benchTiles(b, graphics.ProtocolKitty, true, false) }
func BenchmarkTilesKittyMove(b *testing.B)   { benchTiles(b, graphics.ProtocolKitty, false, true) }

func TestKittyTilesDoNotAllocate(t *testing.T) {
	for _, tc := range []struct {
		name         string
		change, move bool
	}{
		{"still", false, false},
		{"change", true, false},
		{"move", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newTileMap(t, graphics.ProtocolKitty, tc.change, tc.move)
			m.warm(t)
			n := 0
			allocs := testing.AllocsPerRun(100, func() {
				m.frame(t, n)
				n++
			})
			if allocs != 0 {
				t.Fatalf("a frame of Kitty tiles allocated %.0f times", allocs)
			}
		})
	}
}
