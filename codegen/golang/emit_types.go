package golang

import (
	"fmt"
	"strings"

	"github.com/tespeleta/gosmithy/model"
)

func (g *generator) emitTypesFile() ([]byte, error) {
	doc := fmt.Sprintf(
		"Package types contains Go data types and validators generated from the Smithy service %s.\n\n"+
			"Member presence is represented by pointers or nil slices/maps. Requiredness and "+
			"constraints are enforced by the Validate methods; callers should not build cyclic values.\n\n"+
			"Known limitations: @default and @clientOptional are not applied, @length counts Unicode "+
			"code points for strings and bytes for blobs, @pattern uses Go's RE2 regular "+
			"expressions rather than ECMAScript, and @range on bigInteger/bigDecimal is not generated.",
		g.service.ID,
	)
	w := newFileWriter("types", doc)

	for i, shape := range g.shapes {
		if i > 0 {
			w.Line("")
		}
		switch shape.Type {
		case model.TypeStructure:
			g.emitStructure(w, shape)
		case model.TypeList, model.TypeSet:
			g.emitList(w, shape)
		case model.TypeMap:
			g.emitMap(w, shape)
		}
	}
	return w.Render()
}

func (g *generator) emitStructure(w *fileWriter, shape *model.Shape) {
	name, ok := g.typeName(shape)
	if !ok {
		return
	}
	g.writeShapeDoc(w, shape)
	if len(shape.Members) == 0 {
		w.Line("type %s struct{}", name)
	} else {
		w.Line("type %s struct {", name)
		w.In()
		for _, member := range shape.Members {
			g.writeMemberDoc(w, member)
			typ, ok := g.memberFieldType(member, w)
			if !ok || typ == "" {
				typ = "any"
			}
			w.Line("%s %s `json:%q`", g.fieldName(member), typ, g.memberJSONName(member)+",omitempty")
		}
		w.Out()
		w.Line("}")
	}
	w.Line("")
	if shape.IsErrorStructure() {
		g.emitErrorMethods(w, shape)
	}
}

func (g *generator) emitList(w *fileWriter, shape *model.Shape) {
	name, ok := g.typeName(shape)
	if !ok {
		return
	}
	g.writeShapeDoc(w, shape)
	elem, ok := g.collectionElementValueType(shape, w)
	if !ok || elem == "" {
		elem = "any"
	}
	w.Line("type %s []%s", name, elem)
}

func (g *generator) emitMap(w *fileWriter, shape *model.Shape) {
	name, ok := g.typeName(shape)
	if !ok {
		return
	}
	g.writeShapeDoc(w, shape)
	value, ok := g.mapValueValueType(shape, w)
	if !ok || value == "" {
		value = "any"
	}
	w.Line("type %s map[string]%s", name, value)
}

func (g *generator) emitErrorMethods(w *fileWriter, shape *model.Shape) {
	name, _ := g.typeName(shape)
	code := shape.ID.Name
	fault, ok := shape.ErrorType()
	if !ok || fault == "" {
		fault = "client"
	}

	messageField := ""
	for _, member := range shape.Members {
		if !strings.EqualFold(member.ID.Member, "message") {
			continue
		}
		target, ok := g.m.Shape(member.Target)
		if ok && g.isStringLike(target) {
			messageField = g.fieldName(member)
			break
		}
	}

	w.Line("// Error implements error.")
	w.Line("func (e *%s) Error() string {", name)
	w.In()
	if messageField == "" {
		w.Line("return %s", quoted(code))
	} else {
		w.Line("if e != nil && e.%s != nil && *e.%s != \"\" {", messageField, messageField)
		w.In()
		w.Line("return string(*e.%s)", messageField)
		w.Out()
		w.Line("}")
		w.Line("return %s", quoted(code))
	}
	w.Out()
	w.Line("}")
	w.Line("")
	w.Line("// ErrorCode returns the Smithy error code.")
	w.Line("func (e *%s) ErrorCode() string { return %s }", name, quoted(code))
	w.Line("")
	w.Line("// ErrorFault returns the Smithy error fault (client or server).")
	w.Line("func (e *%s) ErrorFault() string { return %s }", name, quoted(fault))
}

func (g *generator) writeShapeDoc(w *fileWriter, shape *model.Shape) {
	doc := g.shapeDocumentation(shape)
	if doc != "" {
		w.Comment(doc)
	}
	if dep, ok := deprecatedLine(shape); ok {
		w.Line("//")
		w.Line("// %s", dep)
	}
}

func (g *generator) writeMemberDoc(w *fileWriter, member *model.Shape) {
	doc := g.memberDocumentation(member)
	if doc != "" {
		w.Comment(doc)
	}
	if dep, ok := deprecatedLine(member); ok {
		w.Line("//")
		w.Line("// %s", dep)
	}
}

func (g *generator) shapeDocumentation(shape *model.Shape) string {
	if doc, ok := shape.Documentation(); ok && doc != "" {
		return doc
	}
	return shape.Doc
}

func (g *generator) memberDocumentation(member *model.Shape) string {
	if doc, ok := member.Documentation(); ok && doc != "" {
		return doc
	}
	return member.Doc
}

// deprecatedLine renders @deprecated using Go's "Deprecated:" convention.
func deprecatedLine(shape *model.Shape) (string, bool) {
	trait, ok := shape.GetTrait(model.TraitDeprecated)
	if !ok {
		return "", false
	}
	message, _ := trait.Value.GetString("message")
	since, _ := trait.Value.GetString("since")
	line := "Deprecated: this shape is deprecated."
	if message != "" {
		line = "Deprecated: " + message
	}
	if since != "" {
		line += " (since " + since + ")"
	}
	return line, true
}
