// Package codegen defines the plugin contract used by gosmithy code
// generators.
//
// A generator receives one request that describes the assembled model and the
// requested output, and returns the files it wants to write. Generators never
// touch the filesystem: the codegen.Runner merges their responses, detects
// conflicts, and writes the output tree. Keeping file I/O in one place makes
// generators deterministic and easy to test.
package codegen

import (
	"context"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"sync"

	"github.com/tespeleta/gosmithy/model"
	"github.com/tespeleta/gosmithy/validation"
)

// Generator is the interface a codegen target implements.
type Generator interface {
	Generate(ctx context.Context, req *Request) (*Response, error)
}

// NamedGenerator is an optional interface a generator implements to report a
// stable name. The runner uses it in error messages; unnamed generators are
// identified by their position in the run.
type NamedGenerator interface {
	Name() string
}

// GeneratorFunc adapts a function to the Generator interface.
type GeneratorFunc func(ctx context.Context, req *Request) (*Response, error)

// Generate calls f.
func (f GeneratorFunc) Generate(ctx context.Context, req *Request) (*Response, error) {
	return f(ctx, req)
}

// Request is a single code generation request. The same request is passed to
// every generator in a run.
type Request struct {
	// Model is the assembled, frozen semantic model.
	Model *model.Model
	// Service is the service whose closure should be generated.
	Service model.ShapeID
	// Version is the Smithy IDL version targeted by the generator.
	Version model.Version
	// Target names the target being generated and lists its generators.
	Target Target
	// Protocol is the selected wire protocol, or nil for model-only
	// generation.
	Protocol *Protocol
	// Options holds generator-specific key/value options.
	Options map[string]string
	// Files lists the source files the model was assembled from. It is
	// intended for diagnostics only.
	Files []string
}

// Target is a bundle of generators that write into one module. Milestone 3
// registers a single generator named "go".
type Target struct {
	Name       string
	Generators []string
}

// Protocol identifies a wire protocol implementation. The protocol's trait
// shape ID is enough for a generator to select its implementation; the CLI
// assembles the protocol definition model with the user's files.
type Protocol struct {
	ID model.ShapeID
}

// Response is the set of files and diagnostics a generator produces.
type Response struct {
	Files       []File
	Diagnostics []Diagnostic
}

// File is one generated file. Path is slash-separated and relative to the
// runner's output directory.
type File struct {
	Path    string
	Content []byte
	// Mode is the file mode. A zero Mode means 0644.
	Mode fs.FileMode
}

// Diagnostic is a problem reported by a generator. DANGER diagnostics fail a
// run; NOTE and WARNING diagnostics are reported but do not prevent output.
type Diagnostic struct {
	Severity validation.Severity
	Message  string
	Path     string
	Line     int
	Column   int
}

// String formats a diagnostic like the rest of the CLI:
//
//	file.smithy:3:5: DANGER: unsupported shape
func (d Diagnostic) String() string {
	if d.Path == "" {
		return fmt.Sprintf("%s: %s", d.Severity, d.Message)
	}
	if d.Line == 0 {
		return fmt.Sprintf("%s: %s: %s", d.Path, d.Severity, d.Message)
	}
	return fmt.Sprintf("%s:%d:%d: %s: %s", d.Path, d.Line, d.Column, d.Severity, d.Message)
}

// Factory creates a fresh generator instance.
type Factory func() Generator

// Registry maps generator names to factories. A registry is safe for
// concurrent use after registration.
type Registry struct {
	mu        sync.RWMutex
	factories map[string]Factory
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{factories: make(map[string]Factory)}
}

// Register adds a generator factory. It panics when name is empty, f is nil,
// or the name is already registered: those are programming errors, not runtime
// conditions.
func (r *Registry) Register(name string, f Factory) {
	if name == "" {
		panic("codegen: empty generator name")
	}
	if f == nil {
		panic("codegen: nil generator factory for " + name)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.factories[name]; exists {
		panic("codegen: generator " + name + " is already registered")
	}
	r.factories[name] = f
}

// Get instantiates the named generator.
func (r *Registry) Get(name string) (Generator, bool) {
	r.mu.RLock()
	f, ok := r.factories[name]
	r.mu.RUnlock()
	if !ok {
		return nil, false
	}
	return f(), true
}

// Names returns the registered generator names in lexical order.
func (r *Registry) Names() []string {
	r.mu.RLock()
	names := make([]string, 0, len(r.factories))
	for name := range r.factories {
		names = append(names, name)
	}
	r.mu.RUnlock()
	sort.Strings(names)
	return names
}

// String returns a comma-separated list of names, for error messages.
func (r *Registry) String() string {
	return strings.Join(r.Names(), ", ")
}
