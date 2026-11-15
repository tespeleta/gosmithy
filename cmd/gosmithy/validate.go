package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/tespeleta/gosmithy/model"
	"github.com/tespeleta/gosmithy/validation"
)

// newEncoder returns a deterministic, indented JSON encoder.
func newEncoder(w *os.File) *json.Encoder {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc
}

func runValidate(args []string) error {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	minSeverity := fs.String("min-severity", "note", "minimum severity to report: note, warning, or danger")
	jsonOut := fs.Bool("json", false, "print validation events as JSON")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "Usage: gosmithy validate [flags] <file>...")
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
		return fmt.Errorf("validate requires at least one file")
	}

	threshold, ok := validation.ParseSeverity(*minSeverity)
	if !ok {
		return fmt.Errorf("invalid -min-severity %q: want note, warning, or danger", *minSeverity)
	}

	files, err := parseFiles(fs.Args())
	if err != nil {
		return err
	}
	files, err = appendBuiltinProtocols(files)
	if err != nil {
		return err
	}
	m, err := model.Assemble(files, model.Options{})
	if err != nil {
		return err
	}

	events := validation.Validate(m)
	filtered := make([]validation.Event, 0, len(events))
	hasDanger := false
	for _, event := range events {
		if event.Severity < threshold {
			continue
		}
		filtered = append(filtered, event)
		if event.Severity == validation.Danger {
			hasDanger = true
		}
	}

	if *jsonOut {
		if err := newEncoder(os.Stdout).Encode(eventsJSON(filtered)); err != nil {
			return err
		}
	} else {
		for _, event := range filtered {
			fmt.Fprintln(os.Stdout, event.String())
		}
	}

	if hasDanger {
		return &exitError{code: 1}
	}
	return nil
}

func eventsJSON(events []validation.Event) []map[string]any {
	out := make([]map[string]any, 0, len(events))
	for _, event := range events {
		item := map[string]any{
			"id":       event.ID,
			"severity": event.Severity.String(),
			"message":  event.Message,
		}
		if !event.ShapeID.IsZero() {
			item["shapeId"] = event.ShapeID.String()
		}
		if event.Source.Filename != "" || event.Source.Line != 0 {
			item["source"] = map[string]any{
				"filename": event.Source.Filename,
				"line":     event.Source.Line,
				"column":   event.Source.Column,
			}
		}
		out = append(out, item)
	}
	return out
}
