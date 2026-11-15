// Package docs generates the Markdown guide from an OpenAPI document.
package docs

import (
	"context"

	"github.com/tespeleta/gosmithy/codegen"
	"github.com/tespeleta/gosmithy/codegen/openapi"
)

const (
	// Namespace is the documentation generator namespace.
	Namespace = "docs"
	// GeneratorName is the legacy documentation option name.
	GeneratorName = Namespace
	// OptionMarkdown emits docs/README.md.
	OptionMarkdown = "markdown"
)

// New returns the Markdown documentation generator.
func New() codegen.Generator { return &generator{name: Namespace + ":" + OptionMarkdown} }

type generator struct {
	name string
}

func (g *generator) Name() string {
	if g.name != "" {
		return g.name
	}
	return GeneratorName
}

func (g *generator) Generate(ctx context.Context, req *codegen.Request) (*codegen.Response, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	doc, diagnostics := openapi.Build(req)
	response := &codegen.Response{Diagnostics: diagnostics}
	if doc == nil {
		return response, nil
	}
	links := docLinks{}
	for _, selected := range req.Target.Generators {
		switch selected {
		case "openapi:index":
			links.DescriptorHref = "openapi.json"
			links.IndexHref = "index.html"
		case "openapi:descriptor":
			if links.DescriptorHref == "" {
				links.DescriptorHref = "../openapi.json"
			}
		}
	}
	response.Files = []codegen.File{
		{Path: "docs/README.md", Content: renderMarkdown(doc, links)},
	}
	return response, nil
}

type docLinks struct {
	DescriptorHref string
	IndexHref      string
}
