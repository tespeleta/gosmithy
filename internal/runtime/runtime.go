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

// Files returns the runtime files to emit, keyed by output path relative to the
// generated module root, for example "internal/smithy/validate.go".
func Files() (map[string][]byte, error) {
	entries, err := fs.ReadDir(templates, "templates")
	if err != nil {
		return nil, fmt.Errorf("runtime: reading templates: %w", err)
	}
	out := make(map[string][]byte, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
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
