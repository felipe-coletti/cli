package meta

// Name and Version are variables (not constants) so they can be injected at
// build time:
//
//	go build -ldflags "-X cli/internal/meta.Version=2.1.0 -X cli/internal/meta.Name=mycli"
var (
	Name    = "mycli"
	Version = "2.1.0-dev"
)

// Magic is written at the start of every sealed file so the CLI can
// recognise its own files. It must be exactly 4 bytes (checked at compile
// time below).
//
// It is a constant on purpose: unlike Name it cannot be overridden at build
// time, and changing it makes every file sealed with the old value
// unreadable. Pick the final value before encrypting anything you care about.
const Magic = "CLIV"

// Compile-time check: fails to build if Magic is not exactly 4 bytes.
var _ = [1]struct{}{}[len(Magic)-4]
