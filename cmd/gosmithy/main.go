// Command gosmithy is the command-line interface for the gosmithy Smithy IDL
// implementation.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/tespeleta/gosmithy/idl"
)

var version = "0.1.0-dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "gosmithy: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		printUsage(os.Stderr)
		return fmt.Errorf("a command is required")
	}

	switch args[0] {
	case "version":
		fmt.Printf("gosmithy %s\n", version)
		return nil
	case "parse":
		return runParse(args[1:])
	case "help", "-h", "--help":
		printUsage(os.Stdout)
		return nil
	default:
		printUsage(os.Stderr)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func printUsage(w *os.File) {
	fmt.Fprintf(w, `Usage: gosmithy <command> [arguments]

Commands:
  parse <file>...   Parse Smithy IDL files and print their AST as JSON
  version           Print the gosmithy version
  help              Print this help

Examples:
  gosmithy parse testdata/weather.smithy
  gosmithy parse testdata/loader/valid/main-test.smithy
`)
}

func runParse(args []string) error {
	fs := flag.NewFlagSet("parse", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "Usage: gosmithy parse <file>...")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() == 0 {
		fs.Usage()
		return fmt.Errorf("parse requires at least one file")
	}

	files := make([]*idl.File, 0, fs.NArg())
	for _, path := range fs.Args() {
		src, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading %s: %w", path, err)
		}
		parsed, err := idl.Parse(path, string(src))
		if err != nil {
			return err
		}
		files = append(files, parsed)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if len(files) == 1 {
		return enc.Encode(files[0])
	}
	return enc.Encode(files)
}
