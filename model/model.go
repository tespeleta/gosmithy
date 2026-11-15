package model

import (
	"encoding/json"
	"sort"

	"github.com/tespeleta/gosmithy/idl"
)

// Use records a `use` statement from one file.
type Use struct {
	ID     ShapeID
	Source idl.SourceLocation
}

// Model is a frozen, assembled Smithy semantic model.
//
// Shapes are stored with fully-qualified IDs and are iterated in lexical order.
// All members are first-class member shapes and are present in the shape map.
type Model struct {
	metadata    map[string]Node
	shapes      map[ShapeID]*Shape
	uses        []Use
	diagnostics []Diagnostic
}

// NewModel returns an empty model.
func NewModel() *Model {
	return &Model{
		metadata: make(map[string]Node),
		shapes:   make(map[ShapeID]*Shape),
	}
}

// Shape returns the shape with the given ID.
func (m *Model) Shape(id ShapeID) (*Shape, bool) {
	s, ok := m.shapes[id]
	return s, ok
}

// HasShape reports whether the model contains the shape.
func (m *Model) HasShape(id ShapeID) bool {
	_, ok := m.shapes[id]
	return ok
}

// Shapes returns all shapes in deterministic lexical order.
func (m *Model) Shapes() []*Shape {
	out := make([]*Shape, 0, len(m.shapes))
	for _, s := range m.shapes {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID.String() < out[j].ID.String() })
	return out
}

// ShapesInNamespace returns the shapes whose ID has the given namespace, in
// deterministic order.
func (m *Model) ShapesInNamespace(namespace string) []*Shape {
	var out []*Shape
	for _, s := range m.Shapes() {
		if s.ID.Namespace == namespace {
			out = append(out, s)
		}
	}
	return out
}

// Services returns all service shapes in deterministic order.
func (m *Model) Services() []*Shape {
	var out []*Shape
	for _, s := range m.Shapes() {
		if s.Type == TypeService {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID.String() < out[j].ID.String() })
	return out
}

// Metadata returns the model metadata for a key.
func (m *Model) Metadata(key string) (Node, bool) {
	n, ok := m.metadata[key]
	return n, ok
}

// MetadataKeys returns the metadata keys in lexical order.
func (m *Model) MetadataKeys() []string {
	keys := make([]string, 0, len(m.metadata))
	for k := range m.metadata {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Uses returns the use statements of the model in deterministic order.
func (m *Model) Uses() []Use { return append([]Use(nil), m.uses...) }

// Diagnostics returns the diagnostics collected while assembling the model.
func (m *Model) Diagnostics() []Diagnostic {
	out := make([]Diagnostic, len(m.diagnostics))
	copy(out, m.diagnostics)
	return out
}

// AddDiagnostic appends an assembly diagnostic. It is exported for the
// validation package and tests; normal users should call validation.Validate.
func (m *Model) AddDiagnostic(d Diagnostic) {
	m.diagnostics = append(m.diagnostics, d)
}

func (m *Model) addShape(s *Shape) {
	m.shapes[s.ID] = s
}

// MarshalJSON renders a deterministic debug JSON view of the model. It is not
// a Smithy JSON AST and is not intended for interchange.
func (m *Model) MarshalJSON() ([]byte, error) {
	return json.Marshal(m.toMap(true))
}

// MarshalJSONWithoutPrelude renders the debug JSON view with prelude shapes
// omitted.
func (m *Model) MarshalJSONWithoutPrelude() ([]byte, error) {
	return json.Marshal(m.toMap(false))
}

func (m *Model) toMap(includePrelude bool) map[string]any {
	metadata := make(map[string]any, len(m.metadata))
	for k, v := range m.metadata {
		metadata[k] = v
	}

	shapes := make(map[string]any, len(m.shapes))
	for _, s := range m.Shapes() {
		if !includePrelude && s.IsPrelude() {
			continue
		}
		shapes[s.ID.String()] = s.toMap()
	}

	out := map[string]any{
		"shapes": shapes,
	}
	if len(metadata) > 0 {
		out["metadata"] = metadata
	}
	return out
}

func (s *Shape) toMap() map[string]any {
	out := map[string]any{
		"type": string(s.Type),
	}
	if len(s.Traits) > 0 {
		traits := make(map[string]any, len(s.Traits))
		for _, id := range s.Traits.SortedIDs() {
			t := s.Traits[id]
			traits[id.String()] = t.Value
		}
		out["traits"] = traits
	}
	if len(s.Members) > 0 {
		members := make(map[string]any, len(s.Members))
		for _, m := range s.Members {
			member := map[string]any{
				"target": m.Target.String(),
			}
			if len(m.Traits) > 0 {
				traits := make(map[string]any, len(m.Traits))
				for _, id := range m.Traits.SortedIDs() {
					t := m.Traits[id]
					traits[id.String()] = t.Value
				}
				member["traits"] = traits
			}
			if m.Elided {
				member["elided"] = true
			}
			members[m.ID.Member] = member
		}
		out["members"] = members
	}
	if s.IsMember() {
		out["target"] = s.Target.String()
		out["container"] = s.Container.String()
	}
	if !s.ForResource.IsZero() {
		out["for"] = s.ForResource.String()
	}
	if len(s.Mixins) > 0 {
		mixins := make([]string, 0, len(s.Mixins))
		for _, m := range s.Mixins {
			mixins = append(mixins, m.String())
		}
		out["mixins"] = mixins
	}
	if s.Type == TypeOperation {
		out["input"] = s.Input.String()
		out["output"] = s.Output.String()
		if len(s.Errors) > 0 {
			errors := make([]string, 0, len(s.Errors))
			for _, e := range s.Errors {
				errors = append(errors, e.String())
			}
			out["errors"] = errors
		}
	}
	if s.Service != nil {
		service := map[string]any{}
		if s.Service.Version != "" {
			service["version"] = s.Service.Version
		}
		if len(s.Service.Operations) > 0 {
			service["operations"] = shapeIDStrings(s.Service.Operations)
		}
		if len(s.Service.Resources) > 0 {
			service["resources"] = shapeIDStrings(s.Service.Resources)
		}
		if len(s.Service.Errors) > 0 {
			service["errors"] = shapeIDStrings(s.Service.Errors)
		}
		if len(s.Service.Rename) > 0 {
			rename := make(map[string]string, len(s.Service.Rename))
			for id, name := range s.Service.Rename {
				rename[id.String()] = name
			}
			service["rename"] = rename
		}
		out["service"] = service
	}
	if s.Resource != nil {
		resource := map[string]any{}
		if len(s.Resource.Identifiers) > 0 {
			resource["identifiers"] = shapeIDMap(s.Resource.Identifiers)
		}
		if len(s.Resource.Properties) > 0 {
			resource["properties"] = shapeIDMap(s.Resource.Properties)
		}
		if !s.Resource.Create.IsZero() {
			resource["create"] = s.Resource.Create.String()
		}
		if !s.Resource.Read.IsZero() {
			resource["read"] = s.Resource.Read.String()
		}
		if !s.Resource.Update.IsZero() {
			resource["update"] = s.Resource.Update.String()
		}
		if !s.Resource.Delete.IsZero() {
			resource["delete"] = s.Resource.Delete.String()
		}
		if !s.Resource.List.IsZero() {
			resource["list"] = s.Resource.List.String()
		}
		if !s.Resource.Put.IsZero() {
			resource["put"] = s.Resource.Put.String()
		}
		if len(s.Resource.Operations) > 0 {
			resource["operations"] = shapeIDStrings(s.Resource.Operations)
		}
		if len(s.Resource.CollectionOperations) > 0 {
			resource["collectionOperations"] = shapeIDStrings(s.Resource.CollectionOperations)
		}
		if len(s.Resource.Resources) > 0 {
			resource["resources"] = shapeIDStrings(s.Resource.Resources)
		}
		out["resource"] = resource
	}
	if s.Source.Filename != "" || s.Source.Line != 0 {
		out["source"] = map[string]any{
			"filename": s.Source.Filename,
			"line":     s.Source.Line,
			"column":   s.Source.Column,
		}
	}
	return out
}

func shapeIDStrings(ids []ShapeID) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, id.String())
	}
	return out
}

func shapeIDMap(m map[string]ShapeID) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v.String()
	}
	return out
}
