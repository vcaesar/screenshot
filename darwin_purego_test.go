//go:build darwin && purego

package screenshot

import (
	"image"
	"testing"
)

func TestIntersectRect(t *testing.T) {
	tests := []struct {
		name string
		a, b cgRect
		want cgRect
		ok   bool
	}{
		{"overlap", cgRect{0, 0, 100, 100}, cgRect{50, 25, 100, 100}, cgRect{50, 25, 50, 75}, true},
		{"contained", cgRect{0, 0, 100, 100}, cgRect{10, 10, 20, 20}, cgRect{10, 10, 20, 20}, true},
		{"negative origin", cgRect{-1920, 0, 1920, 1080}, cgRect{-100, 0, 200, 50}, cgRect{-100, 0, 100, 50}, true},
		{"touching edge", cgRect{0, 0, 100, 100}, cgRect{100, 0, 50, 50}, cgRect{}, false},
		{"disjoint", cgRect{0, 0, 10, 10}, cgRect{20, 20, 10, 10}, cgRect{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := intersectRect(tt.a, tt.b)
			if got != tt.want || ok != tt.ok {
				t.Errorf("intersectRect(%v, %v) = %v, %v; want %v, %v", tt.a, tt.b, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestCGCoordinateOfDisplay(t *testing.T) {
	main := cgRect{0, 0, 1440, 900}
	if got, want := cgCoordinateOfDisplay(main, main), main; got != want {
		t.Errorf("main display: got %v, want %v", got, want)
	}
	// Secondary display above the main one: y = -1080 in top-left coordinates.
	above := cgRect{0, -1080, 1920, 1080}
	if got, want := cgCoordinateOfDisplay(above, main), (cgRect{0, 900, 1920, 1080}); got != want {
		t.Errorf("display above: got %v, want %v", got, want)
	}
}

func TestRoundUpEven(t *testing.T) {
	for in, want := range map[int]int{0: 0, 1: 2, 2: 2, 1079: 1080} {
		if got := roundUpEven(in); got != want {
			t.Errorf("roundUpEven(%d) = %d, want %d", in, got, want)
		}
	}
}

func TestArgbToRGBA(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 1))
	copy(img.Pix, []byte{0, 1, 2, 3, 9, 4, 5, 6})
	argbToRGBA(img)
	want := []byte{1, 2, 3, 255, 4, 5, 6, 255}
	if string(img.Pix) != string(want) {
		t.Errorf("got %v, want %v", img.Pix, want)
	}
}

func TestDisplayFuncs(t *testing.T) {
	n := NumActiveDisplays()
	if n == 0 {
		t.Skip("no active display")
	}
	b := GetDisplayBounds(0)
	if b.Min != (image.Point{}) || b.Dx() <= 0 || b.Dy() <= 0 {
		t.Errorf("unexpected main display bounds %v", b)
	}
}
