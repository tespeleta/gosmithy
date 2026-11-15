package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/tespeleta/gosmithy/model"
)

func runModel(args []string) error {
	fs := flag.NewFlagSet("model", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	jsonOut := fs.Bool("json", false, "print the model as debug JSON")
	includePrelude := fs.Bool("prelude", true, "include prelude shapes in the output")
	includeProtocols := fs.Bool("protocols", true, "assemble the built-in wire protocol definitions")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "Usage: gosmithy model [flags] <file>...")
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
		return fmt.Errorf("model requires at least one file")
	}

	files, err := parseFiles(fs.Args())
	if err != nil {
		return err
	}
	if *includeProtocols {
		files, err = appendBuiltinProtocols(files)
		if err != nil {
			return err
		}
	}
	m, err := model.Assemble(files, model.Options{})
	if err != nil {
		return err
	}

	if *jsonOut {
		var data []byte
		if *includePrelude {
			data, err = m.MarshalJSON()
		} else {
			data, err = m.MarshalJSONWithoutPrelude()
		}
		if err != nil {
			return err
		}
		fmt.Fprintln(os.Stdout, string(data))
		return nil
	}

	shapeCount := 0
	for _, shape := range m.Shapes() {
		if !*includePrelude && shape.IsPrelude() {
			continue
		}
		shapeCount++
	}
	services := m.Services()
	fmt.Printf("%d shapes", shapeCount)
	if len(services) > 0 {
		fmt.Printf(", %d services", len(services))
	}
	fmt.Println()
	for _, service := range services {
		if !*includePrelude && service.IsPrelude() {
			continue
		}
		fmt.Printf("  service %s\n", service.ID)
	}
	return nil
}
