package golang

import (
	"fmt"

	"github.com/tespeleta/gosmithy/model"
	"github.com/tespeleta/gosmithy/validation"
)

func (g *generator) emitUnionsFile() ([]byte, error) {
	w := newFileWriter("types", "")
	first := true
	for _, shape := range g.shapes {
		if shape.Type != model.TypeUnion {
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
		g.emitUnion(w, shape, name)
	}
	return w.Render()
}

func (g *generator) emitUnion(w *fileWriter, shape *model.Shape, name string) {
	g.writeShapeDoc(w, shape)
	marker := markerName(name)
	w.Line("type %s interface {", name)
	w.In()
	w.Line("%s()", marker)
	w.Out()
	w.Line("}")
	w.Line("")

	for _, member := range shape.Members {
		wrapper := g.unionWrapperName(shape, name, member)
		typ, ok := g.unionValueType(member, w)
		if !ok || typ == "" {
			typ = "any"
		}
		g.writeMemberDoc(w, member)
		w.Line("type %s struct {", wrapper)
		w.In()
		w.Line("Value %s", typ)
		w.Out()
		w.Line("}")
		w.Line("")
		w.Line("func (*%s) %s() {}", wrapper, marker)
		w.Line("")
	}
}

// unionWrapperName returns a package-unique wrapper struct name for a union
// member.
func (g *generator) unionWrapperName(union *model.Shape, unionName string, member *model.Shape) string {
	base := unionName + pascalCase(g.rawMemberName(member))
	name := base
	for i := 2; g.packageNameTaken(name); i++ {
		g.diagnosticAt(validation.Danger, member.Source,
			"Go union wrapper name %q for `%s` is already used; use the service `rename` map to disambiguate",
			name, member.ID)
		name = fmt.Sprintf("%s_%d", base, i)
	}
	g.packageNames[name] = union.ID
	return name
}

func (g *generator) packageNameTaken(name string) bool {
	if _, exists := g.packageNames[name]; exists {
		return true
	}
	if _, exists := g.constantNames[name]; exists {
		return true
	}
	return false
}
