package codegen

import (
	"context"
	"reflect"
	"testing"
)

func TestRegistry(t *testing.T) {
	r := NewRegistry()
	r.Register("b", func() Generator { return GeneratorFunc(nil) })
	r.Register("a", func() Generator { return GeneratorFunc(nil) })

	if got, want := r.Names(), []string{"a", "b"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Names = %v, want %v", got, want)
	}
	gen, ok := r.Get("a")
	if !ok || gen == nil {
		t.Fatalf("Get(a) = %v, %v", gen, ok)
	}
	if _, ok := r.Get("missing"); ok {
		t.Errorf("Get(missing) succeeded")
	}
	if got, want := r.String(), "a, b"; got != want {
		t.Errorf("String = %q, want %q", got, want)
	}
}

func TestRegistryPanics(t *testing.T) {
	cases := []struct {
		name string
		fn   func(*Registry)
	}{
		{"empty", func(r *Registry) { r.Register("", func() Generator { return GeneratorFunc(nil) }) }},
		{"nil", func(r *Registry) { r.Register("x", nil) }},
		{"duplicate", func(r *Registry) {
			r.Register("x", func() Generator { return GeneratorFunc(nil) })
			r.Register("x", func() Generator { return GeneratorFunc(nil) })
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatalf("Register did not panic")
				}
			}()
			tc.fn(NewRegistry())
		})
	}
}

func TestGeneratorFunc(t *testing.T) {
	called := false
	var g Generator = GeneratorFunc(func(context.Context, *Request) (*Response, error) {
		called = true
		return &Response{}, nil
	})
	if _, err := g.Generate(context.Background(), &Request{}); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("GeneratorFunc was not called")
	}
}

func TestRegistryNamespacesAndResolution(t *testing.T) {
	r := NewRegistry()
	gen := func() Generator { return GeneratorFunc(nil) }
	r.RegisterNamespace("go", map[string]Factory{"model": gen, "server": gen, "client": gen})
	r.RegisterNamespace("openapi", map[string]Factory{"descriptor": gen, "index": gen})
	r.RegisterNamespace("docs", map[string]Factory{"markdown": gen})

	if got, want := r.Namespaces(), []string{"docs", "go", "openapi"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Namespaces = %v, want %v", got, want)
	}
	if got, want := r.Options("go"), []string{"client", "model", "server"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Options(go) = %v, want %v", got, want)
	}

	selections, err := r.Resolve([]string{"go", "openapi:descriptor"})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, selection := range selections {
		names = append(names, selection.Name)
	}
	want := []string{"go:client", "go:model", "go:server", "openapi:descriptor"}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("Resolve names = %v, want %v", names, want)
	}

	all, err := r.Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(all), 6; got != want {
		t.Errorf("Resolve(nil) = %d, want %d", got, want)
	}
	if _, err := r.Resolve([]string{"missing"}); err == nil {
		t.Error("Resolve(missing) succeeded")
	}
	if _, err := r.Resolve([]string{"go:missing"}); err == nil {
		t.Error("Resolve(go:missing) succeeded")
	}
	if _, err := r.Resolve([]string{"go:"}); err == nil {
		t.Error("Resolve(go:) succeeded")
	}
}
