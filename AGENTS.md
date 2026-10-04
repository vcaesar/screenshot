# AGENTS.md

`github.com/vcaesar/screenshot`: single-package Go library (fork of kbinani/screenshot) that captures the desktop as `*image.RGBA` on darwin, windows, linux, freebsd, openbsd and netbsd.

## Commands

- Build check (as CI does, per OS): `GOOS=<darwin|windows|linux|freebsd|openbsd|netbsd> go build ./example/main.go`
- Vet the other platforms from macOS: `GOOS=linux go vet ./...`, `GOOS=windows go vet ./...`
- Test: `go test ./...` (or `go test -run TestCaptureRect .`). Needs a real display, see Testing.
- Benchmark: `go test -bench BenchmarkCaptureRect -run '^$' .`
- Lint: `golangci-lint run` (CI runs latest golangci-lint with `only-new-issues: true`; there's no config file, so it uses the defaults)
- Run example: `go run ./example` (writes `<i>_<w>x<h>.png` and `all.png` to the current directory)

## Architecture

- `screenshot.go`: shared API (`CaptureDisplay`, `CaptureRect`, `ErrUnsupported`, `createImage`).
- Each platform must provide exactly one `Capture`, `NumActiveDisplays` and `GetDisplayBounds`. Which files compile is decided by `//go:build` tags, not filename suffixes:
  - `darwin.go`: `cgo && darwin`. Uses ScreenCaptureKit when min macOS > 14.4, otherwise `CGDisplayCreateImageForRect`.
  - `windows.go`: `Capture` plus Win32 helpers. `windows_ge1.21.go` / `windows_lt1.21.go` hold `NumActiveDisplays`/`GetDisplayBounds`, split by Go version (`runtime.Pinner` on go1.21+).
  - `nix.go`: X11/xinerama display enumeration. `nix_xwindow.go`: `captureXinerama` (X11 + MIT-SHM).
  - `nix_dbus_available.go` (linux/openbsd/netbsd): `Capture` switches on `XDG_SESSION_TYPE == "wayland"` → `captureDbus` in `nix_wayland.go` (xdg-desktop-portal Screenshot), else X11.
  - `nix_dbus_unavailable.go` (freebsd): `Capture` uses X11 only.
  - `unsupported.go`: stubs returning `ErrUnsupported`/0. Covers s390x, ppc64le, darwin without cgo, and any other OS.
- `example/main.go`: the program CI builds.

## Gotchas

- Changing a build tag means checking the whole tag matrix: every GOOS/arch must end up with exactly one definition of each public func. Build all six GOOS targets after any tag edit.
- darwin is the only cgo platform. Cross-compiling with `GOOS=darwin` from another OS (CGO disabled) silently selects `unsupported.go`, so darwin code only gets checked on a mac.
- Wayland capture asks the portal for a full-screen PNG (non-interactive) and then crops it with `draw.Draw`. Display enumeration still uses X11/xinerama, even under Wayland.
- Coordinates are relative to the primary display's upper-left corner, with Y pointing down (Windows-style), on every platform.
- The nix and windows display funcs swallow errors/panics (`recover`) and return 0 or an empty `image.Rectangle` rather than an error.
- README imports and badges still point to `kbinani/screenshot`. Use `github.com/vcaesar/screenshot` in code.

## Testing

- `screenshot_test.go` holds the only test and benchmark. It captures display 0, so it fails on headless CI, without X11/Wayland, or on macOS without Screen Recording permission. CI does not run tests, only builds.
