package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/tespeleta/gosmithy/codegen"
	"github.com/tespeleta/gosmithy/codegen/docs"
	"github.com/tespeleta/gosmithy/codegen/golang"
	"github.com/tespeleta/gosmithy/codegen/openapi"
	"github.com/tespeleta/gosmithy/model"
	"github.com/tespeleta/gosmithy/protocol/httpjson"
	"github.com/tespeleta/gosmithy/validation"
)

// flagWasSet reports whether name was provided explicitly on the command line.
func flagWasSet(fs *flag.FlagSet, name string) bool {
	set := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			set = true
		}
	})
	return set
}

// optionFlags collects repeatable --opt key=value flags.
type optionFlags []string

func (o *optionFlags) String() string { return strings.Join(*o, ",") }

func (o *optionFlags) Set(value string) error {
	*o = append(*o, value)
	return nil
}

// generatorFlags collects repeatable --generator flags. A single flag may
// contain a comma-separated list.
type generatorFlags []string

func (g *generatorFlags) String() string { return strings.Join(*g, ",") }

func (g *generatorFlags) Set(value string) error {
	for _, part := range strings.Split(value, ",") {
		name := strings.TrimSpace(part)
		if name == "" {
			return fmt.Errorf("generator name must not be empty")
		}
		*g = append(*g, name)
	}
	return nil
}

func runGenerate(args []string) error {
	fs := flag.NewFlagSet("generate", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var generatorNames generatorFlags
	fs.Var(&generatorNames, "generator", "generator namespace or namespace:option (repeatable/comma-separated; default all)")
	service := fs.String("service", "", "service shape ID to generate")
	module := fs.String("module", "", "Go module path for the generated module")
	useOptionals := fs.Bool("useOptionals", true, "generate Optional[T] for non-required scalar members (Go only)")
	goMod := fs.Bool("goMod", false, "generate go.mod for the output (Go only)")
	out := fs.String("out", "", "output directory")
	clean := fs.Bool("clean", false, "remove the output directory before writing")
	list := fs.Bool("list-generators", false, "list registered generators and exit")
	var rawOptions optionFlags
	fs.Var(&rawOptions, "opt", "generator option key=value (repeatable)")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "Usage: gosmithy generate --service <id> --out <dir> [--generator <namespace[:option]>]... [flags] <file>...")
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
		for _, namespace := range registry.Namespaces() {
			fmt.Fprintf(os.Stdout, "%s: %s\n", namespace, strings.Join(registry.Options(namespace), ", "))
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
	selections, err := registry.Resolve([]string(generatorNames))
	if err != nil {
		return err
	}
	names := make([]string, 0, len(selections))
	generators := make([]codegen.Generator, 0, len(selections))
	for _, selection := range selections {
		names = append(names, selection.Name)
		generators = append(generators, selection.Generator)
	}
	if needsGoModule(names) && *module == "" {
		fs.Usage()
		return fmt.Errorf("generate requires --module when a Go generator is selected")
	}
	if *module != "" {
		if existing, ok := options["module"]; ok && existing != *module {
			return fmt.Errorf("--module %q conflicts with --opt module=%q", *module, existing)
		}
		options["module"] = *module
	}
	if flagWasSet(fs, "useOptionals") {
		optionalValue := strconv.FormatBool(*useOptionals)
		if existing, ok := options["useOptionals"]; ok && existing != optionalValue {
			return fmt.Errorf("--useOptionals=%s conflicts with --opt useOptionals=%s", optionalValue, existing)
		}
		options["useOptionals"] = optionalValue
	}
	if flagWasSet(fs, "goMod") {
		goModValue := strconv.FormatBool(*goMod)
		if existing, ok := options["goMod"]; ok && existing != goModValue {
			return fmt.Errorf("--goMod=%s conflicts with --opt goMod=%s", goModValue, existing)
		}
		options["goMod"] = goModValue
	}

	files, err := parseFiles(fs.Args())
	if err != nil {
		return err
	}

	var selectedProtocol *codegen.Protocol
	if needsHTTPProtocol(names) {
		selectedProtocol = &codegen.Protocol{ID: httpjson.ID}
	}

	// Built-in protocol definitions are always assembled so @httpJson resolves
	// even for model-only generation.
	files, err = appendBuiltinProtocols(files)
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
		fmt.Fprintln(os.Stdout, event.String())
		if event.Severity == validation.Danger {
			hasDanger = true
		}
	}
	if hasDanger {
		return &exitError{code: 1}
	}

	req := &codegen.Request{
		Model:    m,
		Service:  serviceID,
		Version:  model.V2_0,
		Protocol: selectedProtocol,
		Target: codegen.Target{
			Name:       strings.Join(names, "+"),
			Generators: names,
		},
		Options: options,
		Files:   fs.Args(),
	}
	runner := &codegen.Runner{
		OutDir: *out,
		Clean:  *clean,
		Output: os.Stdout,
	}
	_, err = runner.Run(context.Background(), req, generators)
	return err
}

func needsGoModule(names []string) bool {
	for _, name := range names {
		if strings.HasPrefix(name, golang.Namespace+":") {
			return true
		}
	}
	return false
}

func needsHTTPProtocol(names []string) bool {
	for _, name := range names {
		switch {
		case name == golang.Namespace+":"+golang.OptionServer:
			return true
		case name == golang.Namespace+":"+golang.OptionClient:
			return true
		case strings.HasPrefix(name, openapi.Namespace+":"):
			return true
		case strings.HasPrefix(name, docs.Namespace+":"):
			return true
		}
	}
	return false
}

func newGeneratorRegistry() *codegen.Registry {
	registry := codegen.NewRegistry()
	registry.RegisterNamespace(golang.Namespace, map[string]codegen.Factory{
		golang.OptionModel:  func() codegen.Generator { return golang.NewModel() },
		golang.OptionServer: func() codegen.Generator { return golang.NewServer() },
		golang.OptionClient: func() codegen.Generator { return golang.NewClient() },
	})
	registry.RegisterNamespace(openapi.Namespace, map[string]codegen.Factory{
		openapi.OptionDescriptor: func() codegen.Generator { return openapi.NewDescriptor() },
		openapi.OptionIndex:      func() codegen.Generator { return openapi.NewIndex() },
	})
	registry.RegisterNamespace(docs.Namespace, map[string]codegen.Factory{
		docs.OptionMarkdown: func() codegen.Generator { return docs.New() },
	})
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
