package golang

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/tespeleta/gosmithy/model"
	"github.com/tespeleta/gosmithy/validation"
)

type enumValue struct {
	name       string
	value      string
	doc        string
	deprecated bool
}

func (g *generator) emitEnumsFile() ([]byte, error) {
	w := newFileWriter("types", "")
	first := true
	for _, shape := range g.shapes {
		if !isEnumLike(shape) {
			continue
		}
		name, ok := g.typeName(shape)
		if !ok {
			continue
		}
		if !first {
			w.Line("")
		}
		first = false
		g.emitEnum(w, shape, name)
	}
	return w.Render()
}

func isEnumLike(shape *model.Shape) bool {
	if shape.Type == model.TypeEnum || shape.Type == model.TypeIntEnum {
		return true
	}
	return shape.Type == model.TypeString && shape.HasTrait(model.TraitEnum)
}

func (g *generator) emitEnum(w *fileWriter, shape *model.Shape, name string) {
	g.writeShapeDoc(w, shape)
	underlying := "string"
	if shape.Type == model.TypeIntEnum {
		underlying = "int32"
	}
	w.Line("type %s %s", name, underlying)

	values := g.enumValues(shape)
	declared := make([]string, 0, len(values))
	w.Line("")
	if len(values) > 0 {
		w.Line("const (")
		w.In()
		for _, value := range values {
			constName := g.enumConstant(shape, name, value)
			declared = append(declared, constName)
			if value.doc != "" {
				w.Comment(value.doc)
			}
			if value.deprecated {
				w.Line("// Deprecated: this enum value is deprecated.")
			}
			if shape.Type == model.TypeIntEnum {
				w.Line("%s %s = %s", constName, name, value.value)
			} else {
				w.Line("%s %s = %s", constName, name, quoted(value.value))
			}
		}
		w.Out()
		w.Line(")")
		w.Line("")
	}

	w.Line("// Values returns all known values for %s.", name)
	w.Line("func (%s) Values() []%s {", name, name)
	w.In()
	if len(declared) == 0 {
		w.Line("return nil")
	} else {
		w.Line("return []%s{", name)
		w.In()
		for _, constName := range declared {
			w.Line("%s,", constName)
		}
		w.Out()
		w.Line("}")
	}
	w.Out()
	w.Line("}")
	w.Line("")
	w.Line("// Valid reports whether e is one of the known values of %s.", name)
	w.Line("func (e %s) Valid() bool {", name)
	w.In()
	if len(declared) > 0 {
		w.Line("switch e {")
		w.Line("case %s:", strings.Join(declared, ", "))
		w.In()
		w.Line("return true")
		w.Out()
		w.Line("}")
	}
	w.Line("return false")
	w.Out()
	w.Line("}")
}

// enumConstant picks a unique exported constant name for a value. Preferred
// names are <Enum><ValueName>; collisions fall back to the raw value and then
// to a numeric suffix.
func (g *generator) enumConstant(shape *model.Shape, enumName string, value enumValue) string {
	base := enumConstantName(enumName, value.name)
	if base == enumName {
		base = enumConstantName(enumName, "Value")
	}
	candidates := []string{base}
	if value.value != "" {
		candidates = append(candidates, enumConstantName(enumName, value.value))
	}
	for _, candidate := range candidates {
		if !g.constantTaken(candidate) {
			g.constantNames[candidate] = shape.ID
			return candidate
		}
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s_%d", base, i)
		if !g.constantTaken(candidate) {
			g.constantNames[candidate] = shape.ID
			return candidate
		}
	}
}

func (g *generator) constantTaken(name string) bool {
	if name == "" {
		return true
	}
	if _, exists := g.packageNames[name]; exists {
		return true
	}
	if _, exists := g.constantNames[name]; exists {
		return true
	}
	return false
}

func (g *generator) enumValues(shape *model.Shape) []enumValue {
	switch shape.Type {
	case model.TypeEnum:
		values := make([]enumValue, 0, len(shape.Members))
		for _, member := range shape.Members {
			value, ok := member.EnumValue()
			if !ok {
				continue
			}
			text, ok := value.AsString()
			if !ok {
				continue
			}
			values = append(values, enumValue{
				name:       g.rawMemberName(member),
				value:      text,
				doc:        g.memberDocumentation(member),
				deprecated: member.IsDeprecated(),
			})
		}
		return values
	case model.TypeIntEnum:
		values := make([]enumValue, 0, len(shape.Members))
		for _, member := range shape.Members {
			value, ok := member.EnumValue()
			if !ok {
				continue
			}
			n, ok := value.AsInt()
			if !ok {
				g.diagnosticAt(validation.Danger, member.Source,
					"intEnum member `%s` has a value that is not an integer", member.ID)
				continue
			}
			values = append(values, enumValue{
				name:       g.rawMemberName(member),
				value:      strconv.FormatInt(n, 10),
				doc:        g.memberDocumentation(member),
				deprecated: member.IsDeprecated(),
			})
		}
		return values
	case model.TypeString:
		trait, ok := shape.GetTrait(model.TraitEnum)
		if !ok || trait.Value.Kind != model.NodeArray {
			return nil
		}
		values := make([]enumValue, 0, len(trait.Value.Arr))
		for _, entry := range trait.Value.Arr {
			if entry.Kind != model.NodeObject {
				continue
			}
			text, ok := entry.GetString("value")
			if !ok {
				continue
			}
			name, _ := entry.GetString("name")
			if name == "" {
				name = text
			}
			doc, _ := entry.GetString("documentation")
			deprecated := false
			if v, ok := entry.Get("deprecated"); ok && v.Kind == model.NodeBoolean {
				deprecated = v.Bool
			}
			values = append(values, enumValue{
				name:       name,
				value:      text,
				doc:        doc,
				deprecated: deprecated,
			})
		}
		return values
	default:
		return nil
	}
}
