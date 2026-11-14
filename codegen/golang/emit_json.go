package golang

import (
	"strings"

	"github.com/tespeleta/gosmithy/model"
)

// emitJSONFile generates the custom JSON codecs required by the protocol.
//
// encoding/json handles most generated values directly through the JSON tags
// emitted with the structs. A few Smithy semantics cannot be expressed that
// way:
//
//   - unions are a one-member JSON object, but their Go representation is an
//     interface;
//   - timestamps honor @timestampFormat, while time.Time always uses RFC 3339;
//   - bigDecimal values must stay JSON numbers without passing through
//     float64.
//
// The generator emits explicit methods for exactly the shapes that need them.
// Every other shape delegates to encoding/json, which also means custom codecs
// compose: a structure containing a union field uses the union's generated
// helper from its own UnmarshalJSON.
func (g *generator) emitJSONFile() ([]byte, error) {
	var shapes []*model.Shape
	for _, shape := range g.shapes {
		if g.needsCustomJSON(shape) {
			shapes = append(shapes, shape)
		}
	}
	if len(shapes) == 0 {
		return nil, nil
	}

	w := newFileWriter("types", "JSON codecs for unions, formatted timestamps, and big decimals. "+
		"They implement the value encoding rules of gosmithy.protocols#httpJson.")
	for i, shape := range shapes {
		if i > 0 {
			w.Line("")
		}
		switch shape.Type {
		case model.TypeStructure:
			g.emitStructJSON(w, shape)
		case model.TypeUnion:
			g.emitUnionJSON(w, shape)
		case model.TypeList, model.TypeSet:
			g.emitListJSON(w, shape)
		case model.TypeMap:
			g.emitMapJSON(w, shape)
		}
	}
	return w.Render()
}

// needsCustomJSON reports whether a shape needs generated JSON methods.
func (g *generator) needsCustomJSON(shape *model.Shape) bool {
	if shape == nil {
		return false
	}
	switch shape.Type {
	case model.TypeUnion:
		return true
	case model.TypeStructure:
		for _, member := range shape.Members {
			target, ok := g.m.Shape(member.Target)
			if !ok {
				continue
			}
			if g.memberNeedsCustomJSON(member, target) {
				return true
			}
		}
	case model.TypeList, model.TypeSet:
		member, ok := shape.GetMember("member")
		if !ok {
			return false
		}
		target, ok := g.m.Shape(member.Target)
		return ok && g.collectionElementNeedsCustomJSON(member, target)
	case model.TypeMap:
		member, ok := shape.GetMember("value")
		if !ok {
			return false
		}
		target, ok := g.m.Shape(member.Target)
		return ok && g.collectionElementNeedsCustomJSON(member, target)
	}
	return false
}

// memberNeedsCustomJSON reports whether a direct member target cannot be
// handled by encoding/json alone.
func (g *generator) memberNeedsCustomJSON(member, target *model.Shape) bool {
	switch target.Type {
	case model.TypeUnion:
		return true
	case model.TypeTimestamp:
		return g.timestampFormatFor(member, target) != model.TimestampDateTime
	case model.TypeBigDecimal:
		return true
	case model.TypeDocument:
		return true
	default:
		return false
	}
}

// collectionElementNeedsCustomJSON is memberNeedsCustomJSON, except that a
// named aggregate with its own codec can be delegated to safely.
func (g *generator) collectionElementNeedsCustomJSON(member, target *model.Shape) bool {
	switch target.Type {
	case model.TypeUnion:
		return true
	case model.TypeTimestamp:
		return g.timestampFormatFor(member, target) != model.TimestampDateTime
	case model.TypeBigDecimal:
		return true
	case model.TypeDocument:
		return true
	default:
		return false
	}
}

// timestampFormatFor resolves the effective timestamp format for a member.
func (g *generator) timestampFormatFor(member, target *model.Shape) string {
	if member != nil {
		if format, ok := member.TimestampFormat(); ok && format != "" {
			return format
		}
	}
	if target != nil {
		if format, ok := target.TimestampFormat(); ok && format != "" {
			return format
		}
	}
	return model.TimestampDateTime
}

func (g *generator) addJSONImports(w *fileWriter) {
	w.AddImport("encoding/json", "")
}

// --- structures ---------------------------------------------------------------

func (g *generator) emitStructJSON(w *fileWriter, shape *model.Shape) {
	name, ok := g.typeName(shape)
	if !ok {
		return
	}
	g.addJSONImports(w)
	w.Line("// MarshalJSON implements json.Marshaler using the httpJson value encoding.")
	w.Line("func (v %s) MarshalJSON() ([]byte, error) {", name)
	w.In()
	w.Line("fields := make(map[string]json.RawMessage, %d)", len(shape.Members))
	for _, member := range shape.Members {
		target, ok := g.m.Shape(member.Target)
		if !ok {
			continue
		}
		g.emitMarshalMember(w, member, target)
	}
	w.Line("return json.Marshal(fields)")
	w.Out()
	w.Line("}")
	w.Line("")

	g.addJSONImports(w)
	w.Line("// UnmarshalJSON implements json.Unmarshaler using the httpJson value encoding.")
	w.Line("func (v *%s) UnmarshalJSON(data []byte) error {", name)
	w.In()
	w.Line("var fields map[string]json.RawMessage")
	w.Line("if err := json.Unmarshal(data, &fields); err != nil {")
	w.In()
	w.Line("return err")
	w.Out()
	w.Line("}")
	w.Line("for key, raw := range fields {")
	w.In()
	w.Line("switch key {")
	for _, member := range shape.Members {
		target, ok := g.m.Shape(member.Target)
		if !ok {
			continue
		}
		w.Line("case %s:", quoted(g.memberJSONName(member)))
		w.In()
		address := target.Type == model.TypeTimestamp && fieldHasPointerWrapper(target)
		g.emitUnmarshalValue(w, member, target, "v."+g.fieldName(member), address, "return err")
		w.Out()
	}
	w.Line("}")
	w.Out()
	w.Line("}")
	w.Line("return nil")
	w.Out()
	w.Line("}")
}

func (g *generator) emitMarshalMember(w *fileWriter, member, target *model.Shape) {
	field := "v." + g.fieldName(member)
	w.Line("if %s != nil {", field)
	w.In()
	g.emitMarshalValue(w, member, target, targetValueExpr(target, field), "fields["+quoted(g.memberJSONName(member))+"]")
	w.Out()
	w.Line("}")
}

// --- unions -------------------------------------------------------------------

func (g *generator) emitUnionJSON(w *fileWriter, shape *model.Shape) {
	name, ok := g.typeName(shape)
	if !ok {
		return
	}
	for _, member := range shape.Members {
		wrapper := g.unionWrapperName(shape, name, member)
		target, ok := g.m.Shape(member.Target)
		if !ok {
			continue
		}
		g.addJSONImports(w)
		w.Line("// MarshalJSON encodes the %s union member.", g.rawMemberName(member))
		w.Line("func (v *%s) MarshalJSON() ([]byte, error) {", wrapper)
		w.In()
		w.Line("if v == nil {")
		w.In()
		w.Line("return []byte(\"null\"), nil")
		w.Out()
		w.Line("}")
		g.emitUnionMarshal(w, member, target, "v.Value")
		w.Out()
		w.Line("}")
		w.Line("")
	}

	g.addJSONImports(w)
	w.AddImport("fmt", "")
	w.Line("// Unmarshal%s decodes a one-member JSON object into a %s union.", name, name)
	w.Line("func Unmarshal%s(data []byte) (%s, error) {", name, name)
	w.In()
	w.Line("if string(data) == \"null\" {")
	w.In()
	w.Line("return nil, nil")
	w.Out()
	w.Line("}")
	w.Line("var fields map[string]json.RawMessage")
	w.Line("if err := json.Unmarshal(data, &fields); err != nil {")
	w.In()
	w.Line("return nil, err")
	w.Out()
	w.Line("}")
	w.Line("if len(fields) != 1 {")
	w.In()
	w.Line("return nil, fmt.Errorf(%s, len(fields))", quoted(name+": expected exactly one union member, found %d"))
	w.Out()
	w.Line("}")
	w.Line("for key, raw := range fields {")
	w.In()
	w.Line("switch key {")
	for _, member := range shape.Members {
		wrapper := g.unionWrapperName(shape, name, member)
		target, ok := g.m.Shape(member.Target)
		if !ok {
			continue
		}
		w.Line("case %s:", quoted(g.memberJSONName(member)))
		w.In()
		g.emitUnionUnmarshal(w, member, target, wrapper)
		w.Out()
	}
	w.Line("default:")
	w.In()
	w.Line("return nil, fmt.Errorf(%s, key)", quoted(name+": unknown union member %q"))
	w.Out()
	w.Line("}")
	w.Out()
	w.Line("}")
	w.Line("return nil, fmt.Errorf(%s)", quoted(name+": empty union"))
	w.Out()
	w.Line("}")
}

func (g *generator) emitUnionMarshal(w *fileWriter, member, target *model.Shape, valueExpr string) {
	switch target.Type {
	case model.TypeTimestamp:
		format := g.timestampFormatFor(member, target)
		if format == model.TimestampDateTime {
			w.Line("return json.Marshal(map[string]any{%s: %s})", quoted(g.memberJSONName(member)), valueExpr)
			return
		}
		g.addSmithyImport(w)
		w.Line("formatted, err := smithy.FormatTimestamp(%s, %s)", valueExpr, quoted(format))
		w.Line("if err != nil {")
		w.In()
		w.Line("return nil, err")
		w.Out()
		w.Line("}")
		w.Line("return json.Marshal(map[string]any{%s: formatted})", quoted(g.memberJSONName(member)))
	case model.TypeBigDecimal:
		g.addSmithyImport(w)
		w.Line("formatted, err := smithy.MarshalBigDecimal(%s)", valueExpr)
		w.Line("if err != nil {")
		w.In()
		w.Line("return nil, err")
		w.Out()
		w.Line("}")
		w.Line("return json.Marshal(map[string]json.RawMessage{%s: formatted})", quoted(g.memberJSONName(member)))
	default:
		w.Line("return json.Marshal(map[string]any{%s: %s})", quoted(g.memberJSONName(member)), valueExpr)
	}
}

func (g *generator) emitUnionUnmarshal(w *fileWriter, member, target *model.Shape, wrapper string) {
	switch target.Type {
	case model.TypeUnion:
		nested, ok := g.typeName(target)
		if !ok {
			w.Line("return nil, fmt.Errorf(%s, key)", quoted("union "+wrapper+": unknown member %q"))
			return
		}
		w.Line("value, err := Unmarshal%s(raw)", nested)
		w.Line("if err != nil {")
		w.In()
		w.Line("return nil, err")
		w.Out()
		w.Line("}")
		w.Line("return &%s{Value: value}, nil", wrapper)
	case model.TypeTimestamp:
		format := g.timestampFormatFor(member, target)
		if format == model.TimestampDateTime {
			w.AddImport("time", "")
			w.Line("var value time.Time")
			w.Line("if err := json.Unmarshal(raw, &value); err != nil {")
			w.In()
			w.Line("return nil, err")
			w.Out()
			w.Line("}")
			w.Line("return &%s{Value: value}, nil", wrapper)
			return
		}
		g.addSmithyImport(w)
		w.Line("var text string")
		w.Line("if err := json.Unmarshal(raw, &text); err != nil {")
		w.In()
		w.Line("return nil, err")
		w.Out()
		w.Line("}")
		w.Line("value, err := smithy.ParseTimestamp(text, %s)", quoted(format))
		w.Line("if err != nil {")
		w.In()
		w.Line("return nil, err")
		w.Out()
		w.Line("}")
		w.Line("return &%s{Value: value}, nil", wrapper)
	case model.TypeBigDecimal:
		g.addSmithyImport(w)
		w.Line("value, err := smithy.UnmarshalBigDecimal(raw, 256)")
		w.Line("if err != nil {")
		w.In()
		w.Line("return nil, err")
		w.Out()
		w.Line("}")
		w.Line("return &%s{Value: value}, nil", wrapper)
	case model.TypeDocument:
		g.addSmithyImport(w)
		w.Line("value, err := smithy.UnmarshalDocument(raw)")
		w.Line("if err != nil {")
		w.In()
		w.Line("return nil, err")
		w.Out()
		w.Line("}")
		w.Line("return &%s{Value: value}, nil", wrapper)
	default:
		valueType, ok := g.unionValueType(member, w)
		if !ok || valueType == "" {
			valueType = "any"
		}
		base := strings.TrimPrefix(valueType, "*")
		w.Line("var value %s", base)
		w.Line("if err := json.Unmarshal(raw, &value); err != nil {")
		w.In()
		w.Line("return nil, err")
		w.Out()
		w.Line("}")
		if strings.HasPrefix(valueType, "*") {
			w.Line("return &%s{Value: &value}, nil", wrapper)
		} else {
			w.Line("return &%s{Value: value}, nil", wrapper)
		}
	}
}

// --- collections --------------------------------------------------------------

func (g *generator) emitListJSON(w *fileWriter, shape *model.Shape) {
	name, ok := g.typeName(shape)
	if !ok {
		return
	}
	member, ok := shape.GetMember("member")
	if !ok {
		return
	}
	target, ok := g.m.Shape(member.Target)
	if !ok {
		return
	}
	sparse := shape.IsSparse()

	g.addJSONImports(w)
	w.Line("// MarshalJSON implements json.Marshaler using the httpJson value encoding.")
	w.Line("func (v %s) MarshalJSON() ([]byte, error) {", name)
	w.In()
	w.Line("items := make([]json.RawMessage, len(v))")
	w.Line("for i := range v {")
	w.In()
	if sparse {
		w.Line("if v[i] == nil {")
		w.In()
		w.Line("items[i] = json.RawMessage(\"null\")")
		w.Line("continue")
		w.Out()
		w.Line("}")
	}
	g.emitMarshalValue(w, member, target, collectionValueExpr(target, "v[i]", sparse), "items[i]")
	w.Out()
	w.Line("}")
	w.Line("return json.Marshal(items)")
	w.Out()
	w.Line("}")
	w.Line("")

	w.Line("// UnmarshalJSON implements json.Unmarshaler using the httpJson value encoding.")
	w.Line("func (v *%s) UnmarshalJSON(data []byte) error {", name)
	w.In()
	w.Line("if string(data) == \"null\" {")
	w.In()
	w.Line("*v = nil")
	w.Line("return nil")
	w.Out()
	w.Line("}")
	w.Line("var items []json.RawMessage")
	w.Line("if err := json.Unmarshal(data, &items); err != nil {")
	w.In()
	w.Line("return err")
	w.Out()
	w.Line("}")
	w.Line("out := make(%s, len(items))", name)
	w.Line("for i, raw := range items {")
	w.In()
	if sparse {
		w.Line("if string(raw) == \"null\" {")
		w.In()
		w.Line("continue")
		w.Out()
		w.Line("}")
	}
	g.emitUnmarshalValue(w, member, target, "out[i]", sparse, "return err")
	w.Out()
	w.Line("}")
	w.Line("*v = out")
	w.Line("return nil")
	w.Out()
	w.Line("}")
}

func (g *generator) emitMapJSON(w *fileWriter, shape *model.Shape) {
	name, ok := g.typeName(shape)
	if !ok {
		return
	}
	member, ok := shape.GetMember("value")
	if !ok {
		return
	}
	target, ok := g.m.Shape(member.Target)
	if !ok {
		return
	}
	sparse := shape.IsSparse()

	g.addJSONImports(w)
	w.Line("// MarshalJSON implements json.Marshaler using the httpJson value encoding.")
	w.Line("func (v %s) MarshalJSON() ([]byte, error) {", name)
	w.In()
	w.Line("items := make(map[string]json.RawMessage, len(v))")
	w.Line("for key, elem := range v {")
	w.In()
	if sparse {
		w.Line("if elem == nil {")
		w.In()
		w.Line("items[key] = json.RawMessage(\"null\")")
		w.Line("continue")
		w.Out()
		w.Line("}")
	}
	g.emitMarshalValue(w, member, target, collectionValueExpr(target, "elem", sparse), "items[key]")
	w.Out()
	w.Line("}")
	w.Line("return json.Marshal(items)")
	w.Out()
	w.Line("}")
	w.Line("")

	w.Line("// UnmarshalJSON implements json.Unmarshaler using the httpJson value encoding.")
	w.Line("func (v *%s) UnmarshalJSON(data []byte) error {", name)
	w.In()
	w.Line("if string(data) == \"null\" {")
	w.In()
	w.Line("*v = nil")
	w.Line("return nil")
	w.Out()
	w.Line("}")
	w.Line("var items map[string]json.RawMessage")
	w.Line("if err := json.Unmarshal(data, &items); err != nil {")
	w.In()
	w.Line("return err")
	w.Out()
	w.Line("}")
	w.Line("out := make(%s, len(items))", name)
	w.Line("for key, raw := range items {")
	w.In()
	if sparse {
		w.Line("if string(raw) == \"null\" {")
		w.In()
		w.Line("continue")
		w.Out()
		w.Line("}")
	}
	g.emitUnmarshalValue(w, member, target, "out[key]", sparse, "return err")
	w.Out()
	w.Line("}")
	w.Line("*v = out")
	w.Line("return nil")
	w.Out()
	w.Line("}")
}

// --- value helpers ------------------------------------------------------------

// targetValueExpr turns a member field expression into the value expression
// used when marshaling. Pointer-wrapped simple values are dereferenced so
// custom timestamp and big-decimal helpers receive the value, not the pointer.
func targetValueExpr(target *model.Shape, field string) string {
	switch target.Type {
	case model.TypeTimestamp, model.TypeBigDecimal:
		if fieldHasPointerWrapper(target) {
			return "*" + field
		}
	}
	return field
}

// collectionValueExpr turns a collection element expression into the value
// expression used when marshaling. Timestamp helpers receive a time.Time, so a
// sparse element (a *time.Time) is dereferenced; every other element type is
// already in the form encoding/json or the custom helper expects.
func collectionValueExpr(target *model.Shape, expr string, sparse bool) string {
	if sparse && target.Type == model.TypeTimestamp {
		return "*" + expr
	}
	return expr
}

// emitMarshalValue marshals expr into dest, which must be a
// json.RawMessage-valued location. It declares b and err in the current block.
func (g *generator) emitMarshalValue(w *fileWriter, member, target *model.Shape, expr, dest string) {
	switch target.Type {
	case model.TypeTimestamp:
		format := g.timestampFormatFor(member, target)
		if format == model.TimestampDateTime {
			w.Line("b, err := json.Marshal(%s)", expr)
			break
		}
		g.addSmithyImport(w)
		w.Line("formatted, err := smithy.FormatTimestamp(%s, %s)", expr, quoted(format))
		w.Line("if err != nil {")
		w.In()
		w.Line("return nil, err")
		w.Out()
		w.Line("}")
		w.Line("b, err := json.Marshal(formatted)")
	case model.TypeBigDecimal:
		g.addSmithyImport(w)
		w.Line("b, err := smithy.MarshalBigDecimal(%s)", expr)
	default:
		w.Line("b, err := json.Marshal(%s)", expr)
	}
	w.Line("if err != nil {")
	w.In()
	w.Line("return nil, err")
	w.Out()
	w.Line("}")
	w.Line("%s = b", dest)
}

// emitUnmarshalValue decodes raw into dest. When address is true, dest must be
// a pointer variable (a *time.Time field or sparse element). fail is the
// statement to execute on error, for example "return err" or "return nil, err".
func (g *generator) emitUnmarshalValue(w *fileWriter, member, target *model.Shape, dest string, address bool, fail string) {
	switch target.Type {
	case model.TypeUnion:
		union, ok := g.typeName(target)
		if !ok {
			w.Line("_ = raw")
			return
		}
		w.Line("value, err := Unmarshal%s(raw)", union)
		w.Line("if err != nil {")
		w.In()
		w.Line("%s", fail)
		w.Out()
		w.Line("}")
		w.Line("%s = value", dest)
	case model.TypeTimestamp:
		format := g.timestampFormatFor(member, target)
		if format == model.TimestampDateTime {
			w.Line("if err := json.Unmarshal(raw, &%s); err != nil {", dest)
			w.In()
			w.Line("%s", fail)
			w.Out()
			w.Line("}")
			return
		}
		g.addSmithyImport(w)
		w.Line("var text string")
		w.Line("if err := json.Unmarshal(raw, &text); err != nil {")
		w.In()
		w.Line("%s", fail)
		w.Out()
		w.Line("}")
		w.Line("parsed, err := smithy.ParseTimestamp(text, %s)", quoted(format))
		w.Line("if err != nil {")
		w.In()
		w.Line("%s", fail)
		w.Out()
		w.Line("}")
		if address {
			w.Line("%s = &parsed", dest)
		} else {
			w.Line("%s = parsed", dest)
		}
	case model.TypeBigDecimal:
		g.addSmithyImport(w)
		w.Line("value, err := smithy.UnmarshalBigDecimal(raw, 256)")
		w.Line("if err != nil {")
		w.In()
		w.Line("%s", fail)
		w.Out()
		w.Line("}")
		w.Line("%s = value", dest)
	case model.TypeDocument:
		g.addSmithyImport(w)
		w.Line("value, err := smithy.UnmarshalDocument(raw)")
		w.Line("if err != nil {")
		w.In()
		w.Line("%s", fail)
		w.Out()
		w.Line("}")
		w.Line("%s = value", dest)
	default:
		w.Line("if err := json.Unmarshal(raw, &%s); err != nil {", dest)
		w.In()
		w.Line("%s", fail)
		w.Out()
		w.Line("}")
	}
}
