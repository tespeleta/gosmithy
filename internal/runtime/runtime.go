// Package runtime owns the source templates that gosmithy copies into a
// generated module's internal/smithy package.
package runtime

import (
	"embed"
	"fmt"
	"io/fs"
)

//go:embed templates/*.go
var templates embed.FS

// Files returns the runtime files for model-only generation. It is equivalent
// to FilesFor(false).
func Files() (map[string][]byte, error) { return FilesFor(false) }

// FilesFor returns the runtime files to emit, keyed by output path relative to
// the generated module root, for example "internal/smithy/validate.go".
//
// The JSON and timestamp helpers are always included because generated type
// codecs use them. The HTTP protocol helpers are included only when protocol
// is true. Relaxed numeric helpers are added separately by RelaxedFiles so
// that generated output for models without relaxed members is unchanged.
func FilesFor(protocol bool) (map[string][]byte, error) {
	entries, err := fs.ReadDir(templates, "templates")
	if err != nil {
		return nil, fmt.Errorf("runtime: reading templates: %w", err)
	}
	out := make(map[string][]byte, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if entry.Name() == "http.go" && !protocol {
			continue
		}
		if entry.Name() == "relaxed.go" {
			continue
		}
		data, err := fs.ReadFile(templates, "templates/"+entry.Name())
		if err != nil {
			return nil, fmt.Errorf("runtime: reading %s: %w", entry.Name(), err)
		}
		out["internal/smithy/"+entry.Name()] = data
	}
	return out, nil
}

// RelaxedFiles returns the runtime file with the relaxed numeric decoders. It
// is emitted only when the service uses relaxed input for a numeric member.
func RelaxedFiles() (map[string][]byte, error) {
	data, err := fs.ReadFile(templates, "templates/relaxed.go")
	if err != nil {
		return nil, fmt.Errorf("runtime: reading relaxed.go: %w", err)
	}
	return map[string][]byte{"internal/smithy/relaxed.go": data}, nil
}
