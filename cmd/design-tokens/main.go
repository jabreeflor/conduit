// Command design-tokens compiles design/tokens.yaml into platform outputs.
//
// Usage:
//
//	design-tokens                       # compile all targets
//	design-tokens -target=css           # only CSS
//	design-tokens -source=path -out=dir # custom paths
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jabreeflor/conduit/internal/design"
)

func main() {
	source := flag.String("source", "design/tokens.yaml", "path to tokens source")
	outDir := flag.String("out", "design/dist", "output directory root")
	target := flag.String("target", "all", "which target to emit: all|css|swift|textual")
	flag.Parse()

	tk, err := design.Load(*source)
	if err != nil {
		fail(err)
	}

	switch *target {
	case "all":
		if err := emitAll(tk, *outDir); err != nil {
			fail(err)
		}
	case "css":
		if err := writeCSS(tk, *outDir); err != nil {
			fail(err)
		}
	case "swift":
		if err := writeSwift(tk, *outDir); err != nil {
			fail(err)
		}
	case "textual":
		if err := writeTextual(tk, *outDir); err != nil {
			fail(err)
		}
	default:
		fail(fmt.Errorf("unknown target %q", *target))
	}
}

func emitAll(tk *design.Tokens, outDir string) error {
	if err := writeCSS(tk, outDir); err != nil {
		return err
	}
	if err := writeSwift(tk, outDir); err != nil {
		return err
	}
	return writeTextual(tk, outDir)
}

func writeCSS(tk *design.Tokens, outDir string) error {
	return writeFile(filepath.Join(outDir, "web", "tokens.css"), []byte(design.EmitCSS(tk)))
}

func writeSwift(tk *design.Tokens, outDir string) error {
	return writeFile(filepath.Join(outDir, "apple", "Tokens.swift"), []byte(design.EmitSwift(tk)))
}

func writeTextual(tk *design.Tokens, outDir string) error {
	for _, mode := range tk.Modes() {
		out, err := design.EmitTextual(tk, mode)
		if err != nil {
			return err
		}
		path := filepath.Join(outDir, "tui", "theme-"+mode+".json")
		if err := writeFile(path, []byte(out)); err != nil {
			return err
		}
	}
	return nil
}

func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return err
	}
	fmt.Printf("wrote %s (%d bytes)\n", path, len(data))
	return nil
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "design-tokens:", err)
	os.Exit(1)
}
