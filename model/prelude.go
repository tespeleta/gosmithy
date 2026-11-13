package model

import (
	_ "embed"
	"fmt"

	"github.com/tespeleta/gosmithy/idl"
)

// preludeSource is the official Smithy prelude, embedded verbatim from the
// Smithy repository (Apache-2.0). It is parsed with our own parser when a
// model is assembled.
//
//go:embed prelude.smithy
var preludeSource string

// PreludePath is the filename used as the source location for prelude shapes.
const PreludePath = "smithy.api.prelude.smithy"

// PreludeNamespace is the namespace of the prelude model.
const PreludeNamespace = "smithy.api"

// ParsePrelude parses the embedded prelude. It is exported for tests.
func ParsePrelude() (*idl.File, error) {
	f, err := idl.ParseWithOptions(PreludePath, preludeSource, idl.Options{
		DefaultVersion: idl.V2_0,
	})
	if err != nil {
		return nil, fmt.Errorf("model: parsing embedded prelude: %w", err)
	}
	if f.Namespace != PreludeNamespace {
		return nil, fmt.Errorf("model: embedded prelude has namespace %q, want %q", f.Namespace, PreludeNamespace)
	}
	return f, nil
}
