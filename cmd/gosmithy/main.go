// Command gosmithy is the command-line interface for the gosmithy Smithy IDL
// implementation.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tespeleta/gosmithy/idl"
	"github.com/tespeleta/gosmithy/protocol/httpjson"
)

var version = "0.1.0-dev"

// exitError requests a specific process exit code without printing an extra
// error message.
type exitError struct{ code int }

func (e *exitError) Error() string { return fmt.Sprintf("exit status %d", e.code) }

func main() {
	if err := run(os.Args[1:]); err != nil {
		var exit *exitError
		if errors.As(err, &exit) {
			os.Exit(exit.code)
		}
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
	case "validate":
		return runValidate(args[1:])
	case "model":
		return runModel(args[1:])
	case "generate":
		return runGenerate(args[1:])
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
  parse <file>...      Parse Smithy IDL or JSON AST files and print their AST as JSON
  validate <file>...   Assemble and validate a Smithy model
  model <file>...      Assemble a Smithy model and print it
  generate <file>...   Generate code, OpenAPI, or docs for a service from a Smithy model
  version              Print the gosmithy version
  help                 Print this help

Examples:
  gosmithy parse testdata/weather.smithy
  gosmithy validate testdata/weather.smithy
  gosmithy model --json testdata/weather.smithy
  gosmithy generate --service example.weather#Weather \\
      --generator go:model --module example.com/weather --out ./gen testdata/weather.smithy
  gosmithy generate --service example.library#Library \\
      --generator go:model,go:server,go:client,openapi:descriptor,openapi:index,docs:markdown \\
      --module example.com/library --out ./gen testdata/library.smithy
  gosmithy validate testdata/loader/valid/main-test.smithy
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

	files, err := parseFiles(fs.Args())
	if err != nil {
		return err
	}

	enc := newEncoder(os.Stdout)
	if len(files) == 1 {
		return enc.Encode(files[0])
	}
	return enc.Encode(files)
}

// parseFiles parses every path with the CLI's default parser options. The CLI
// assumes Smithy 2.0 when a file omits $version, matching the project's 2.0 default.
// appendBuiltinProtocols adds the protocol definitions compiled into gosmithy so
// that gosmithy.protocols#* references resolve without a flag.
func appendBuiltinProtocols(files []*idl.File) ([]*idl.File, error) {
	definition, err := httpjson.Definition()
	if err != nil {
		return nil, err
	}
	return append(files, definition), nil
}

func parseFiles(paths []string) ([]*idl.File, error) {
	files := make([]*idl.File, 0, len(paths))
	for _, path := range paths {
		src, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", path, err)
		}
		var parsed *idl.File
		if strings.EqualFold(filepath.Ext(path), ".json") {
			parsed, err = idl.ParseJSONAST(path, src)
		} else {
			parsed, err = idl.ParseWithOptions(path, string(src), idl.Options{
				Version:        idl.V2_1,
				DefaultVersion: idl.V2_0,
			})
		}
		if err != nil {
			return nil, err
		}
		files = append(files, parsed)
	}
	return files, nil
}
