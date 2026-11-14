package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/tespeleta/gosmithy/codegen"
	"github.com/tespeleta/gosmithy/codegen/golang"
	"github.com/tespeleta/gosmithy/model"
	"github.com/tespeleta/gosmithy/validation"
)

// optionFlags collects repeatable --opt key=value flags.
type optionFlags []string

func (o *optionFlags) String() string { return strings.Join(*o, ",") }

func (o *optionFlags) Set(value string) error {
	*o = append(*o, value)
	return nil
}

func runGenerate(args []string) error {
	fs := flag.NewFlagSet("generate", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	generatorName := fs.String("generator", golang.GeneratorName, "code generator to run")
	service := fs.String("service", "", "service shape ID to generate")
	module := fs.String("module", "", "Go module path for the generated module")
	out := fs.String("out", "", "output directory")
	list := fs.Bool("list-generators", false, "list registered generators and exit")
	var rawOptions optionFlags
	fs.Var(&rawOptions, "opt", "generator option key=value (repeatable)")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "Usage: gosmithy generate --service <id> --module <path> --out <dir> [flags] <file>...")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	registry := newGeneratorRegistry()
	if *list {
		for _, name := range registry.Names() {
			fmt.Fprintln(os.Stdout, name)
		}
		return nil
	}

	if fs.NArg() == 0 {
		fs.Usage()
		return fmt.Errorf("generate requires at least one file")
	}
	if *service == "" {
		fs.Usage()
		return fmt.Errorf("generate requires --service")
	}
	if *module == "" {
		fs.Usage()
		return fmt.Errorf("generate requires --module")
	}
	if *out == "" {
		fs.Usage()
		return fmt.Errorf("generate requires --out")
	}

	serviceID, err := model.ParseShapeID(*service)
	if err != nil {
		return fmt.Errorf("invalid --service %q: %w", *service, err)
	}
	if !serviceID.IsAbsolute() {
		return fmt.Errorf("invalid --service %q: want an absolute shape ID such as example.weather#Weather", *service)
	}

	options, err := parseGeneratorOptions(rawOptions)
	if err != nil {
		return err
	}
	if existing, ok := options["module"]; ok && existing != *module {
		return fmt.Errorf("--module %q conflicts with --opt module=%q", *module, existing)
	}
	options["module"] = *module

	files, err := parseFiles(fs.Args())
	if err != nil {
		return err
	}
	m, err := model.Assemble(files, model.Options{})
	if err != nil {
		return err
	}

	events := validation.Validate(m)
	hasDanger := false
	for _, event := range events {
		fmt.Fprintln(os.Stderr, event.String())
		if event.Severity == validation.Danger {
			hasDanger = true
		}
	}
	if hasDanger {
		return &exitError{code: 1}
	}

	gen, ok := registry.Get(*generatorName)
	if !ok {
		return fmt.Errorf("unknown generator %q (available: %s)", *generatorName, registry.String())
	}
	req := &codegen.Request{
		Model:   m,
		Service: serviceID,
		Version: model.V2_0,
		Target: codegen.Target{
			Name:       *generatorName,
			Generators: []string{*generatorName},
		},
		Options: options,
		Files:   fs.Args(),
	}
	runner := &codegen.Runner{
		OutDir: *out,
		Output: os.Stdout,
	}
	_, err = runner.Run(context.Background(), req, []codegen.Generator{gen})
	return err
}

func newGeneratorRegistry() *codegen.Registry {
	registry := codegen.NewRegistry()
	registry.Register(golang.GeneratorName, func() codegen.Generator { return golang.New() })
	return registry
}

func parseGeneratorOptions(values []string) (map[string]string, error) {
	options := make(map[string]string, len(values))
	for _, value := range values {
		key, val, ok := strings.Cut(value, "=")
		if !ok || strings.TrimSpace(key) == "" {
			return nil, fmt.Errorf("invalid --opt %q: want key=value", value)
		}
		options[strings.TrimSpace(key)] = val
	}
	return options, nil
}
