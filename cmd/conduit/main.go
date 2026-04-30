package main

import (
	"fmt"
	"os"

	"github.com/jabreeflor/conduit/internal/tui"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--version", "version":
			fmt.Printf("conduit %s\n", version)
			return
		}
	}

	if err := tui.RunInteractive(); err != nil {
		fmt.Fprintf(os.Stderr, "conduit: %v\n", err)
		os.Exit(1)
	}
}
