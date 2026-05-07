//go:generate go run ./tools/genskills/main.go

// Package clawdag is the module root. It intentionally exports nothing —
// users of the framework import specific subpackages such as
// github.com/akennis/clawdag-go/library.
//
// The //go:generate directive above is what `go generate .` invokes from the
// repo root to refresh the skills/ distribution from skill-src/, library
// descriptions, and examples/.
package clawdag
