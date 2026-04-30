package localmodel

import (
	"context"
	"flag"
	"fmt"
	"io"
)

// RunCLI is the entry point for `conduit models`.
func RunCLI(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: conduit models install --manifest <artifact-url> --sha256 <sha256> [--name file.gguf]")
		return flag.ErrHelp
	}
	switch args[0] {
	case "install":
		return runInstall(ctx, args[1:], stdout, stderr)
	default:
		return fmt.Errorf("unknown models command %q", args[0])
	}
}

func runInstall(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("conduit models install", flag.ContinueOnError)
	fs.SetOutput(stderr)
	name := fs.String("name", "model.gguf", "managed model filename")
	url := fs.String("manifest", "", "model artifact URL")
	sha := fs.String("sha256", "", "expected SHA-256 checksum")
	root := fs.String("root", "", "Conduit root directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *url == "" || *sha == "" {
		return fmt.Errorf("usage: conduit models install --manifest <artifact-url> --sha256 <sha256> [--name file.gguf]")
	}

	installer := Installer{
		RootDir: *root,
		Progress: func(event Event) {
			if event.Stage == "runtime_adopted" || event.Stage == "verified" {
				fmt.Fprintf(stdout, "%s: %s\n", event.Stage, event.Path)
			}
		},
	}
	result, err := installer.Install(ctx, InstallSpec{
		Runtime: RuntimeSpec{
			Name:        "ollama",
			Kind:        RuntimeOllama,
			BinaryNames: []string{"ollama"},
		},
		Models: []ModelSpec{{
			Name:    *name,
			Runtime: RuntimeOllama,
			Artifact: Artifact{
				Name:   *name,
				URL:    *url,
				SHA256: *sha,
			},
		}},
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "managed root: %s\n", result.ManagedRoot)
	return nil
}
