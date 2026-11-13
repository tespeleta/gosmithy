package idl

import "encoding/json"

// MarshalJSON renders a debug JSON view of the parsed IDL. It is intended for
// diagnostics and the `gosmithy parse` command; it is not a Smithy JSON AST.
func (f *File) MarshalJSON() ([]byte, error) {
	return json.Marshal(f.toMap())
}

func (f *File) toMap() map[string]any {
	m := map[string]any{
		"filename":  f.Path,
		"version":   f.Version.String(),
		"namespace": f.Namespace,
		"source":    locMap(f.Source),
	}
	if len(f.Controls) > 0 {
		controls := make([]any, 0, len(f.Controls))
		for _, c := range f.Controls {
			controls = append(controls, map[string]any{
				"name":   c.Name,
				"value":  nodeJSON(c.Value),
				"source": locMap(c.Source),
			})
		}
		m["controls"] = controls
	}
	if len(f.Metadata) > 0 {
		md := make([]any, 0, len(f.Metadata))
		for _, entry := range f.Metadata {
			md = append(md, map[string]any{
				"key":    entry.Key,
				"value":  nodeJSON(entry.Value),
				"source": locMap(entry.Source),
			})
		}
		m["metadata"] = md
	}
	if len(f.Uses) > 0 {
		uses := make([]any, 0, len(f.Uses))
		for _, u := range f.Uses {
			uses = append(uses, map[string]any{
				"id":     u.ID,
				"source": locMap(u.Source),
			})
		}
		m["uses"] = uses
	}

	shapes := make([]any, 0, len(f.Shapes))
	for _, stmt := range f.Shapes {
		switch s := stmt.(type) {
		case *ShapeStmt:
			shapes = append(shapes, shapeJSON(s))
		case *ApplyStmt:
			shapes = append(shapes, map[string]any{
				"apply":  s.Target.ID,
				"traits": traitsJSON(s.Traits),
				"source": locMap(s.Source),
			})
		}
	}
	m["shapes"] = shapes
	return m
}

func shapeJSON(s *ShapeStmt) map[string]any {
	m := map[string]any{
		"kind":   string(s.Kind),
		"name":   s.Name,
		"id":     s.ID,
		"source": locMap(s.Source),
	}
	if s.Doc != "" {
		m["doc"] = s.Doc
	}
	if len(s.Traits) > 0 {
		m["traits"] = traitsJSON(s.Traits)
	}
	if len(s.Mixins) > 0 {
		m["mixins"] = refsJSON(s.Mixins)
	}
	if len(s.Members) > 0 {
		members := make([]any, 0, len(s.Members))
		for _, member := range s.Members {
			members = append(members, memberJSON(member))
		}
		m["members"] = members
	}
	if s.Body != nil {
		m["body"] = nodeJSON(*s.Body)
	}
	if s.Input != nil {
		m["input"] = inlineIOJSON(s.Input)
	}
	if s.Output != nil {
		m["output"] = inlineIOJSON(s.Output)
	}
	if len(s.Errors) > 0 {
		m["errors"] = refsJSON(s.Errors)
	}
	if s.ForResource != "" {
		m["for"] = s.ForResource
	}
	return m
}

func memberJSON(m Member) map[string]any {
	out := map[string]any{
		"name":   m.Name,
		"source": locMap(m.Source),
	}
	if m.Target.ID != "" {
		out["target"] = m.Target.ID
	}
	if m.Target.InlineList != nil {
		out["target"] = targetJSON(m.Target)
	}
	if m.Target.InlineMap != nil {
		out["target"] = targetJSON(m.Target)
	}
	if m.Elided {
		out["elided"] = true
	}
	if m.Index != nil {
		out["index"] = *m.Index
	}
	if m.Value != nil {
		out["value"] = nodeJSON(*m.Value)
	}
	if len(m.Traits) > 0 {
		out["traits"] = traitsJSON(m.Traits)
	}
	if m.Doc != "" {
		out["doc"] = m.Doc
	}
	return out
}

func targetJSON(t Target) any {
	switch {
	case t.InlineList != nil:
		return map[string]any{"list": targetJSON(t.InlineList.Element)}
	case t.InlineMap != nil:
		return map[string]any{
			"map": map[string]any{
				"key":   targetJSON(t.InlineMap.Key),
				"value": targetJSON(t.InlineMap.Value),
			},
		}
	default:
		return t.ID
	}
}

func inlineIOJSON(io *InlineIO) map[string]any {
	if io.Target != nil {
		return map[string]any{"target": io.Target.ID}
	}
	out := map[string]any{"source": locMap(io.Source)}
	if len(io.Traits) > 0 {
		out["traits"] = traitsJSON(io.Traits)
	}
	if len(io.Mixins) > 0 {
		out["mixins"] = refsJSON(io.Mixins)
	}
	if len(io.Members) > 0 {
		members := make([]any, 0, len(io.Members))
		for _, member := range io.Members {
			members = append(members, memberJSON(member))
		}
		out["members"] = members
	}
	if io.ForResource != "" {
		out["for"] = io.ForResource
	}
	if io.Doc != "" {
		out["doc"] = io.Doc
	}
	return out
}

func traitsJSON(traits []Trait) []any {
	out := make([]any, 0, len(traits))
	for _, t := range traits {
		out = append(out, map[string]any{
			"name":       t.Name,
			"value":      nodeJSON(t.Value),
			"annotation": t.Annotation,
			"source":     locMap(t.Source),
		})
	}
	return out
}

func refsJSON(refs []ShapeRef) []any {
	out := make([]any, 0, len(refs))
	for _, r := range refs {
		out = append(out, r.ID)
	}
	return out
}

func nodeJSON(n Node) any {
	switch n.Kind {
	case NodeNull:
		return nil
	case NodeBoolean:
		return n.Bool
	case NodeNumber:
		return json.Number(n.Num)
	case NodeString:
		return n.Str
	case NodeShapeID:
		return map[string]any{"shapeId": n.Str}
	case NodeArray:
		arr := make([]any, 0, len(n.Arr))
		for _, child := range n.Arr {
			arr = append(arr, nodeJSON(child))
		}
		return arr
	case NodeObject:
		obj := make(map[string]any, len(n.Obj))
		for _, entry := range n.Obj {
			obj[entry.Key] = nodeJSON(entry.Value)
		}
		return obj
	default:
		return nil
	}
}

func locMap(loc SourceLocation) map[string]any {
	return map[string]any{
		"line":   loc.Line,
		"column": loc.Column,
		"start":  loc.Start,
		"end":    loc.End,
	}
}
