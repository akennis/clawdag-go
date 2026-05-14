//go:build ignore

// genlibdesc generates skills/*/references/library.md from library op description constants.
// Run via: go run ./tools/genlibdesc/main.go
// Or via: go generate ./library/...
package main

import (
	"fmt"
	"os"
	"path/filepath"

	dagailib "github.com/akennis/sparsi-go/library"
)

func main() {
	content := "# Available Library Ops\n\n" + dagailib.AllDescriptions() + "\n"

	targets := []string{
		"skills/sparsi-design/references/library.md",
		"skills/sparsi-codegen/references/library.md",
	}

	for _, rel := range targets {
		if err := os.MkdirAll(filepath.Dir(rel), 0755); err != nil {
			fmt.Fprintf(os.Stderr, "mkdir %s: %v\n", filepath.Dir(rel), err)
			os.Exit(1)
		}
		if err := os.WriteFile(rel, []byte(content), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "write %s: %v\n", rel, err)
			os.Exit(1)
		}
		fmt.Printf("wrote %s\n", rel)
	}
}
