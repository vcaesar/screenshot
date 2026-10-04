//go:build darwin && purego

package screenshot

import (
	"errors"
	"fmt"
	"image"
	"runtime"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

// cgRect matches the memory layout of CGRect on 64-bit macOS (CGFloat is float64).
type cgRect struct {
	X, Y, W, H float64
}

const kCGImageAlphaNoneSkipFirst = 6

var (
	cgMainDisplayID                 func() uint32
	cgGetActiveDisplayList          func(maxDisplays uint32, ids *uint32, count *uint32) int32
	cgDisplayBounds                 func(id uint32) cgRect
	cgDisplayCreateImageForRect     func(id uint32, r cgRect) uintptr
	cgImageCreateCopyWithColorSpace func(img, colorSpace uintptr) uintptr
	cgImageRelease                  func(img uintptr)
	cgColorSpaceCreateWithName      func(name uintptr) uintptr
	cgColorSpaceRelease             func(colorSpace uintptr)
	cgBitmapContextCreate           func(data unsafe.Pointer, width, height, bitsPerComponent, bytesPerRow uintptr, colorSpace uintptr, bitmapInfo uint32) uintptr
	cgContextDrawImage              func(ctx uintptr, r cgRect, img uintptr)
	cgContextRelease                func(ctx uintptr)

	kCGColorSpaceSRGB uintptr

	// msgSendRect sends a message whose only argument is a CGRect.
	msgSendRect func(id objc.ID, sel objc.SEL, r cgRect)

	// useScreenCaptureKit is true when SCScreenshotManager (macOS 14+) is available.
	useScreenCaptureKit bool
)

var loadFrameworks = sync.OnceValue(func() error {
	cg, err := purego.Dlopen("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return err
	}
	syms := []struct {
		name string
		fn   any
	}{
		{"CGMainDisplayID", &cgMainDisplayID},
		{"CGGetActiveDisplayList", &cgGetActiveDisplayList},
		{"CGDisplayBounds", &cgDisplayBounds},
		{"CGImageCreateCopyWithColorSpace", &cgImageCreateCopyWithColorSpace},
		{"CGImageRelease", &cgImageRelease},
		{"CGColorSpaceCreateWithName", &cgColorSpaceCreateWithName},
		{"CGColorSpaceRelease", &cgColorSpaceRelease},
		{"CGBitmapContextCreate", &cgBitmapContextCreate},
		{"CGContextDrawImage", &cgContextDrawImage},
		{"CGContextRelease", &cgContextRelease},
	}
	for _, s := range syms {
		addr, err := purego.Dlsym(cg, s.name)
		if err != nil {
			return err
		}
		purego.RegisterFunc(s.fn, addr)
	}
	srgb, err := purego.Dlsym(cg, "kCGColorSpaceSRGB")
	if err != nil {
		return err
	}
	kCGColorSpaceSRGB = **(**uintptr)(unsafe.Pointer(&srgb))

	if _, err := purego.Dlopen("/System/Library/Frameworks/Foundation.framework/Foundation", purego.RTLD_NOW|purego.RTLD_GLOBAL); err != nil {
		return err
	}
	libobjc, err := purego.Dlopen("/usr/lib/libobjc.A.dylib", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return err
	}
	msgSend, err := purego.Dlsym(libobjc, "objc_msgSend")
	if err != nil {
		return err
	}
	purego.RegisterFunc(&msgSendRect, msgSend)

	// ScreenCaptureKit is preferred; CGDisplayCreateImageForRect is the fallback for older macOS.
	_, sckErr := purego.Dlopen("/System/Library/Frameworks/ScreenCaptureKit.framework/ScreenCaptureKit", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	useScreenCaptureKit = sckErr == nil && objc.GetClass("SCScreenshotManager") != 0
	if !useScreenCaptureKit {
		addr, err := purego.Dlsym(cg, "CGDisplayCreateImageForRect")
		if err != nil {
			return fmt.Errorf("no screen capture API available: %w", err)
		}
		purego.RegisterFunc(&cgDisplayCreateImageForRect, addr)
	}
	return nil
})

var (
	selAlloc             = objc.RegisterName("alloc")
	selInit              = objc.RegisterName("init")
	selRelease           = objc.RegisterName("release")
	selRetain            = objc.RegisterName("retain")
	selDrain             = objc.RegisterName("drain")
	selArray             = objc.RegisterName("array")
	selCount             = objc.RegisterName("count")
	selObjectAtIndex     = objc.RegisterName("objectAtIndex:")
	selDisplays          = objc.RegisterName("displays")
	selDisplayID         = objc.RegisterName("displayID")
	selGetContent        = objc.RegisterName("getShareableContentWithCompletionHandler:")
	selInitWithDisplay   = objc.RegisterName("initWithDisplay:excludingWindows:")
	selSetSourceRect     = objc.RegisterName("setSourceRect:")
	selSetWidth          = objc.RegisterName("setWidth:")
	selSetHeight         = objc.RegisterName("setHeight:")
	selSetShowsCursor    = objc.RegisterName("setShowsCursor:")
	selCaptureWithFilter = objc.RegisterName("captureImageWithFilter:configuration:completionHandler:")
)

func Capture(x, y, width, height int) (*image.RGBA, error) {
	if width <= 0 || height <= 0 {
		return nil, errors.New("width or height should be > 0")
	}
	if err := loadFrameworks(); err != nil {
		return nil, err
	}

	img, err := createImage(image.Rect(0, 0, width, height))
	if err != nil {
		return nil, err
	}

	// cg: CoreGraphics coordinate (origin: lower-left corner of primary display, x-axis: rightward, y-axis: upward)
	// win: Windows coordinate (origin: upper-left corner of primary display, x-axis: rightward, y-axis: downward)
	// di: Display local coordinate (origin: upper-left corner of the display, x-axis: rightward, y-axis: downward)
	mainBounds := cgDisplayBounds(cgMainDisplayID())
	cgCaptureBounds := cgRect{float64(x), mainBounds.H - float64(y+height), float64(width), float64(height)}

	colorSpace := cgColorSpaceCreateWithName(kCGColorSpaceSRGB)
	if colorSpace == 0 {
		return nil, errors.New("cannot create colorspace")
	}
	defer cgColorSpaceRelease(colorSpace)

	ctx := cgBitmapContextCreate(unsafe.Pointer(&img.Pix[0]), uintptr(width), uintptr(height), 8,
		uintptr(img.Stride), colorSpace, kCGImageAlphaNoneSkipFirst)
	if ctx == 0 {
		return nil, errors.New("cannot create bitmap context")
	}
	defer cgContextRelease(ctx)

	for _, id := range activeDisplayList() {
		cgBounds := cgCoordinateOfDisplay(cgDisplayBounds(id), mainBounds)
		cgIntersect, ok := intersectRect(cgBounds, cgCaptureBounds)
		if !ok {
			continue
		}

		// CGDisplayCreateImageForRect potentially fail in case width/height is odd number.
		cgIntersect.W = float64(roundUpEven(int(cgIntersect.W)))
		cgIntersect.H = float64(roundUpEven(int(cgIntersect.H)))

		diIntersectDisplayLocal := cgRect{
			cgIntersect.X - cgBounds.X,
			cgBounds.Y + cgBounds.H - (cgIntersect.Y + cgIntersect.H),
			cgIntersect.W, cgIntersect.H,
		}

		captured := captureDisplay(id, diIntersectDisplayLocal, colorSpace)
		if captured == 0 {
			return nil, errors.New("cannot capture display")
		}
		cgContextDrawImage(ctx, cgRect{
			cgIntersect.X - cgCaptureBounds.X, cgIntersect.Y - cgCaptureBounds.Y,
			cgIntersect.W, cgIntersect.H,
		}, captured)
		cgImageRelease(captured)
	}

	argbToRGBA(img)
	return img, nil
}

func NumActiveDisplays() int {
	if loadFrameworks() != nil {
		return 0
	}
	var count uint32
	if cgGetActiveDisplayList(0, nil, &count) != 0 {
		return 0
	}
	return int(count)
}

func GetDisplayBounds(displayIndex int) image.Rectangle {
	if loadFrameworks() != nil {
		return image.Rectangle{}
	}
	id := getDisplayId(displayIndex)
	main := cgMainDisplayID()
	mainBounds := cgDisplayBounds(main)
	bounds := cgCoordinateOfDisplay(cgDisplayBounds(id), mainBounds)

	var rect image.Rectangle
	rect.Min.X = int(bounds.X)
	if main != id {
		rect.Min.Y = int(mainBounds.H - (bounds.Y + bounds.H))
	}
	rect.Max.X = rect.Min.X + int(bounds.W)
	rect.Max.Y = rect.Min.Y + int(bounds.H)
	return rect
}

func getDisplayId(displayIndex int) uint32 {
	main := cgMainDisplayID()
	if displayIndex == 0 {
		return main
	}
	index := 0
	for _, id := range activeDisplayList() {
		if id == main {
			continue
		}
		index++
		if index == displayIndex {
			return id
		}
	}
	return 0
}

func activeDisplayList() []uint32 {
	count := uint32(NumActiveDisplays())
	if count == 0 {
		return nil
	}
	ids := make([]uint32, count)
	if cgGetActiveDisplayList(count, &ids[0], &count) != 0 {
		return nil
	}
	return ids[:count]
}

func captureDisplay(id uint32, rect cgRect, colorSpace uintptr) uintptr {
	if useScreenCaptureKit {
		return captureScreenCaptureKit(id, rect, colorSpace)
	}
	img := cgDisplayCreateImageForRect(id, rect)
	if img == 0 {
		return 0
	}
	defer cgImageRelease(img)
	return cgImageCreateCopyWithColorSpace(img, colorSpace)
}

func captureScreenCaptureKit(id uint32, rect cgRect, colorSpace uintptr) uintptr {
	// Autoreleased objects must be drained on the same OS thread.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	pool := objc.ID(objc.GetClass("NSAutoreleasePool")).Send(selAlloc).Send(selInit)
	defer pool.Send(selDrain)

	contentCh := make(chan objc.ID, 1)
	contentBlock := objc.NewBlock(func(_ objc.Block, content, err objc.ID) {
		if err != 0 || content == 0 {
			contentCh <- 0
			return
		}
		contentCh <- content.Send(selRetain)
	})
	defer contentBlock.Release()
	objc.ID(objc.GetClass("SCShareableContent")).Send(selGetContent, contentBlock)

	content := <-contentCh
	if content == 0 {
		return 0
	}
	defer content.Send(selRelease)

	var target objc.ID
	displays := content.Send(selDisplays)
	for i := range objc.Send[uint](displays, selCount) {
		d := displays.Send(selObjectAtIndex, i)
		if objc.Send[uint32](d, selDisplayID) == id {
			target = d
			break
		}
	}
	if target == 0 {
		return 0
	}

	excluded := objc.ID(objc.GetClass("NSArray")).Send(selArray)
	filter := objc.ID(objc.GetClass("SCContentFilter")).Send(selAlloc).Send(selInitWithDisplay, target, excluded)
	defer filter.Send(selRelease)

	config := objc.ID(objc.GetClass("SCStreamConfiguration")).Send(selAlloc).Send(selInit)
	defer config.Send(selRelease)
	msgSendRect(config, selSetSourceRect, rect)
	config.Send(selSetWidth, uint(rect.W))
	config.Send(selSetHeight, uint(rect.H))
	config.Send(selSetShowsCursor, false)

	imageCh := make(chan uintptr, 1)
	imageBlock := objc.NewBlock(func(_ objc.Block, img uintptr, err objc.ID) {
		if err != 0 || img == 0 {
			imageCh <- 0
			return
		}
		imageCh <- cgImageCreateCopyWithColorSpace(img, colorSpace)
	})
	defer imageBlock.Release()
	objc.ID(objc.GetClass("SCScreenshotManager")).Send(selCaptureWithFilter, filter, config, imageBlock)

	return <-imageCh
}

// cgCoordinateOfDisplay converts display bounds (origin: upper-left of main display, y-axis: downward)
// into CoreGraphics coordinates (origin: lower-left of main display, y-axis: upward).
func cgCoordinateOfDisplay(r, main cgRect) cgRect {
	return cgRect{r.X, -r.Y - r.H + main.H, r.W, r.H}
}

// intersectRect returns the intersection of a and b, and false when it is empty.
func intersectRect(a, b cgRect) (cgRect, bool) {
	x0, y0 := max(a.X, b.X), max(a.Y, b.Y)
	x1, y1 := min(a.X+a.W, b.X+b.W), min(a.Y+a.H, b.Y+b.H)
	if x1 <= x0 || y1 <= y0 {
		return cgRect{}, false
	}
	return cgRect{x0, y0, x1 - x0, y1 - y0}, true
}

func roundUpEven(n int) int {
	return n + n%2
}

// argbToRGBA converts the bitmap context's xRGB pixels into RGBA with opaque alpha.
func argbToRGBA(img *image.RGBA) {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	for iy := range h {
		row := img.Pix[iy*img.Stride : iy*img.Stride+w*4]
		for j := 0; j < len(row); j += 4 {
			row[j], row[j+1], row[j+2], row[j+3] = row[j+1], row[j+2], row[j+3], 255
		}
	}
}
