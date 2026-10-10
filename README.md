
screenshot
==========

[![Build](https://github.com/vcaesar/screenshot/actions/workflows/build.yml/badge.svg)](https://github.com/vcaesar/screenshot/actions/workflows/build.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/vcaesar/screenshot.svg)](https://pkg.go.dev/github.com/vcaesar/screenshot)
[![](https://img.shields.io/badge/license-MIT-428F7E.svg?style=flat)](https://github.com/vcaesar/screenshot/blob/master/LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/vcaesar/screenshot)](https://goreportcard.com/report/github.com/vcaesar/screenshot)

* Go library to capture desktop screen.
* Multiple display supported.
* Supported GOOS: windows, darwin, linux, freebsd, openbsd, and netbsd.
* `cgo` free except for GOOS=darwin (build with `-tags purego` to avoid `cgo` on darwin too).

install
=======

```bash
go get github.com/vcaesar/screenshot
```

example
=======

* sample program [`examples/main.go`](examples/main.go)

	```go
	package main

	import (
		"fmt"
		"image"
		"image/png"
		"os"

		"github.com/vcaesar/screenshot"
	)

	// save *image.RGBA to filePath with PNG format.
	func save(img *image.RGBA, filePath string) {
		file, err := os.Create(filePath)
		if err != nil {
			panic(err)
		}
		defer file.Close()
		err = png.Encode(file, img)
		if err != nil {
			panic(err)
		}
	}

	func main() {
		// Capture each displays.
		n := screenshot.NumActiveDisplays()
		if n <= 0 {
			panic("Active display not found")
		}

		var all image.Rectangle = image.Rect(0, 0, 0, 0)

		for i := 0; i < n; i++ {
			bounds := screenshot.GetDisplayBounds(i)
			all = bounds.Union(all)

			img, err := screenshot.CaptureRect(bounds)
			if err != nil {
				panic(err)
			}
			fileName := fmt.Sprintf("%d_%dx%d.png", i, bounds.Dx(), bounds.Dy())
			save(img, fileName)

			fmt.Printf("#%d : %v \"%s\"\n", i, bounds, fileName)
		}

		// Capture all desktop region into an image.
		fmt.Printf("%v\n", all)
		img, err := screenshot.Capture(all.Min.X, all.Min.Y, all.Dx(), all.Dy())
		if err != nil {
			panic(err)
		}
		save(img, "all.png")
	}
	```

* output example

	```bash
	$ go run main.go
	#0 : (0,0)-(1280,800) "0_1280x800.png"
	#1 : (-293,-1440)-(2267,0) "1_2560x1440.png"
	#2 : (-1373,-1812)-(-293,108) "2_1080x1920.png"
	(-1373,-1812)-(2267,800)
	$ ls -1
	0_1280x800.png
	1_2560x1440.png
	2_1080x1920.png
	all.png
	main.go
	```

coordinate
=================
Y-axis is downward direction in this library. The origin of coordinate is upper-left corner of main display. This means coordinate system is similar to Windows OS

license
=======

MIT Licence

author
======

kbinani
