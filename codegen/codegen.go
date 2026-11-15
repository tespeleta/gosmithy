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
	"errors"
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

// Target is a bundle of generators that write into one output tree.
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

// Selection is one resolved generator option.
type Selection struct {
	// Namespace is the generator package namespace, such as "go".
	Namespace string
	// Option is the option within the namespace, such as "model".
	Option string
	// Name is the canonical "namespace:option" name.
	Name string
	// Generator is the instantiated generator.
	Generator Generator
}

// Registry maps generator option names to factories. A registry is safe for
// concurrent use after registration.
type Registry struct {
	mu         sync.RWMutex
	factories  map[string]Factory
	namespaces map[string]map[string]Factory
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{
		factories:  make(map[string]Factory),
		namespaces: make(map[string]map[string]Factory),
	}
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

// RegisterNamespace registers a generator namespace and its options. Options
// are addressable as "namespace:option"; selecting the namespace alone selects
// every option. Registration panics on empty names, nil factories, or
// duplicates.
func (r *Registry) RegisterNamespace(namespace string, options map[string]Factory) {
	if namespace == "" {
		panic("codegen: empty generator namespace")
	}
	if len(options) == 0 {
		panic("codegen: generator namespace " + namespace + " has no options")
	}
	r.mu.RLock()
	_, exists := r.namespaces[namespace]
	r.mu.RUnlock()
	if exists {
		panic("codegen: generator namespace " + namespace + " is already registered")
	}
	names := make([]string, 0, len(options))
	for name := range options {
		if name == "" {
			panic("codegen: empty option in generator namespace " + namespace)
		}
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		r.Register(namespace+":"+name, options[name])
	}
	r.mu.Lock()
	r.namespaces[namespace] = options
	r.mu.Unlock()
}

// Namespaces returns the registered namespaces in lexical order.
func (r *Registry) Namespaces() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.namespaces))
	for name := range r.namespaces {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Options returns one namespace's options in lexical order.
func (r *Registry) Options(namespace string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	options := r.namespaces[namespace]
	names := make([]string, 0, len(options))
	for name := range options {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Available returns a human-readable namespace/option listing.
func (r *Registry) Available() string {
	namespaces := r.Namespaces()
	parts := make([]string, 0, len(namespaces))
	for _, namespace := range namespaces {
		options := r.Options(namespace)
		parts = append(parts, namespace+"("+strings.Join(options, ", ")+")")
	}
	return strings.Join(parts, ", ")
}

// Resolve expands generator selections into concrete generator instances.
// An empty selection selects every namespace and every option. A bare
// namespace selects all of its options; a "namespace:option" selects one.
func (r *Registry) Resolve(selections []string) ([]Selection, error) {
	if len(selections) == 0 {
		selections = r.Namespaces()
	}
	order := make([]string, 0)
	seen := make(map[string]bool)
	add := func(namespace, option string) error {
		name := namespace + ":" + option
		if _, exists := r.factories[name]; !exists {
			return fmt.Errorf("unknown generator option %q", name)
		}
		if seen[name] {
			return nil
		}
		seen[name] = true
		order = append(order, name)
		return nil
	}
	for _, raw := range selections {
		selection := strings.TrimSpace(raw)
		if selection == "" {
			return nil, errors.New("codegen: empty generator selection")
		}
		if strings.Contains(selection, ":") {
			namespace, option, ok := strings.Cut(selection, ":")
			if !ok || namespace == "" || option == "" || strings.Contains(option, ":") {
				return nil, fmt.Errorf("codegen: invalid generator selection %q; want namespace:option", raw)
			}
			if _, ok := r.namespaces[namespace]; !ok {
				return nil, fmt.Errorf("codegen: unknown generator namespace %q (available: %s)", namespace, r.Available())
			}
			if err := add(namespace, option); err != nil {
				return nil, fmt.Errorf("codegen: %w (available: %s)", err, r.Available())
			}
			continue
		}
		options, ok := r.namespaces[selection]
		if !ok {
			return nil, fmt.Errorf("codegen: unknown generator %q (available: %s)", selection, r.Available())
		}
		names := make([]string, 0, len(options))
		for option := range options {
			names = append(names, option)
		}
		sort.Strings(names)
		for _, option := range names {
			if err := add(selection, option); err != nil {
				return nil, fmt.Errorf("codegen: %w", err)
			}
		}
	}
	resolved := make([]Selection, 0, len(order))
	for _, name := range order {
		namespace, option, _ := strings.Cut(name, ":")
		gen, ok := r.Get(name)
		if !ok {
			return nil, fmt.Errorf("codegen: unknown generator %q", name)
		}
		resolved = append(resolved, Selection{
			Namespace: namespace,
			Option:    option,
			Name:      name,
			Generator: gen,
		})
	}
	return resolved, nil
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
