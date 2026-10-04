//go:build windows

package screenshot

import (
	"testing"

	"github.com/tailscale/win"
)

func TestCountupMonitorCallback(t *testing.T) {
	count := 0
	for range 3 {
		if ret := countupMonitorCallback(0, 0, nil, &count); ret != 1 {
			t.Fatalf("ret = %d, want 1", ret)
		}
	}
	if count != 3 {
		t.Errorf("count = %d, want 3", count)
	}
}

func TestGetMonitorBoundsCallback(t *testing.T) {
	ctx := &getMonitorBoundsContext{Index: 1}
	rect := &win.RECT{Left: 10, Top: 20, Right: 110, Bottom: 220}

	if ret := getMonitorBoundsCallback(0, 0, rect, ctx); ret != 1 {
		t.Fatalf("skip monitor: ret = %d, want 1", ret)
	}
	if ctx.Count != 1 {
		t.Fatalf("Count = %d, want 1", ctx.Count)
	}

	// hMonitor 0 makes GetMonitorInfo fail, so lprcMonitor is used.
	if ret := getMonitorBoundsCallback(0, 0, rect, ctx); ret != 0 {
		t.Fatalf("match monitor: ret = %d, want 0", ret)
	}
	if ctx.Rect != *rect {
		t.Errorf("Rect = %+v, want %+v", ctx.Rect, *rect)
	}
}

// Regression: under -race (checkptr) these used to abort with
// "checkptr: pointer arithmetic result points to invalid allocation".
func TestDisplayEnumeration(t *testing.T) {
	n := NumActiveDisplays()
	if n < 1 {
		t.Skip("no active displays")
	}
	for i := range n {
		if b := GetDisplayBounds(i); b.Empty() {
			t.Errorf("GetDisplayBounds(%d) is empty", i)
		}
	}
}
