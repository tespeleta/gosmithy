package golang

import (
	"fmt"
	"regexp"

	"github.com/tespeleta/gosmithy/idl"
	"github.com/tespeleta/gosmithy/model"
	"github.com/tespeleta/gosmithy/validation"
)

func (g *generator) emitValidateFile() ([]byte, error) {
	body := newFileWriter("types", "")
	for _, shape := range g.shapes {
		switch shape.Type {
		case model.TypeStructure:
			if err := g.emitStructValidator(body, shape); err != nil {
				return nil, err
			}
		case model.TypeList, model.TypeSet, model.TypeMap:
			if err := g.emitCollectionValidator(body, shape); err != nil {
				return nil, err
			}
		}
	}

	if g.needsBigIntRange {
		body.AddImport("math/big", "")
		body.Line("")
		body.Line("func mustBigInt(value string) *big.Int {")
		body.In()
		body.Line("v, ok := new(big.Int).SetString(value, 10)")
		body.Line("if !ok {")
		body.In()
		body.Line("panic(%s + value)", quoted("smithy: invalid integer range bound: "))
		body.Out()
		body.Line("}")
		body.Line("return v")
		body.Out()
		body.Line("}")
		body.Line("")
		body.Line("func bigIntLess(a *big.Int, bound string) bool {")
		body.In()
		body.Line("if a == nil {")
		body.In()
		body.Line("return false")
		body.Out()
		body.Line("}")
		body.Line("return a.Cmp(mustBigInt(bound)) < 0")
		body.Out()
		body.Line("}")
		body.Line("")
		body.Line("func bigIntGreater(a *big.Int, bound string) bool {")
		body.In()
		body.Line("if a == nil {")
		body.In()
		body.Line("return false")
		body.Out()
		body.Line("}")
		body.Line("return a.Cmp(mustBigInt(bound)) > 0")
		body.Out()
		body.Line("}")
	}
	if g.needsBigDecimalRange {
		body.Line("")
		body.Line("func decimalLess(a, b Decimal) bool {")
		body.In()
		body.Line("cmp, ok := a.Cmp(b)")
		body.Line("return ok && cmp < 0")
		body.Out()
		body.Line("}")
		body.Line("")
		body.Line("func decimalGreater(a, b Decimal) bool {")
		body.In()
		body.Line("cmp, ok := a.Cmp(b)")
		body.Line("return ok && cmp > 0")
		body.Out()
		body.Line("}")
	}

	final := newFileWriter("types", "")
	final.mergeImports(body)
	if len(g.patternOrder) > 0 {
		final.Line("var (")
		final.In()
		for i, pattern := range g.patternOrder {
			final.Line("pattern%d = regexp.MustCompile(%s)", i, quoted(pattern))
		}
		final.Out()
		final.Line(")")
		final.Line("")
	}
	final.body.Write(body.body.Bytes())
	return final.Render()
}

func (g *generator) emitStructValidator(w *fileWriter, shape *model.Shape) error {
	name, ok := g.typeName(shape)
	if !ok {
		return nil
	}
	g.addSmithyImport(w)
	w.Line("// Validate checks the constraints of %s.", name)
	w.Line("func (v *%s) Validate() error {", name)
	w.In()
	w.Line("if v == nil {")
	w.In()
	w.Line("return nil")
	w.Out()
	w.Line("}")
	w.Line("var errs smithy.ValidationErrors")
	for _, member := range shape.Members {
		if err := g.emitMemberValidation(w, shape, member); err != nil {
			return err
		}
	}
	w.Line("return errs.Err()")
	w.Out()
	w.Line("}")
	w.Line("")
	return nil
}

func (g *generator) emitMemberValidation(w *fileWriter, container *model.Shape, member *model.Shape) error {
	target, ok := g.m.Shape(member.Target)
	if !ok {
		g.diagnosticAt(validation.Danger, member.Source,
			"Member `%s` targets `%s`, which is not found in the model", member.ID, member.Target)
		return nil
	}
	switch target.Type {
	case model.TypeStructure, model.TypeList, model.TypeSet, model.TypeMap:
		if !g.hasGeneratedValidator(target) {
			g.diagnosticAt(validation.Danger, member.Source,
				"Member `%s` targets `%s`, which has no generated validator", member.ID, target.ID)
		}
	}

	field := "v." + g.fieldName(member)
	path := quoted(member.ID.Member)
	required := member.Required()
	valueChecks := g.memberValueChecks(member, target)
	hasNilPresence := g.fieldHasNilPresence(member, target)

	if required && !valueChecks {
		if hasNilPresence {
			w.Line("if %s == nil {", field)
			w.In()
			w.Line("errs.Add(%s, %s)", path, quoted("is required"))
			w.Out()
			w.Line("}")
		}
		return nil
	}
	if !required && !valueChecks {
		return nil
	}

	opened := true
	switch {
	case required && hasNilPresence:
		w.Line("if %s == nil {", field)
		w.In()
		w.Line("errs.Add(%s, %s)", path, quoted("is required"))
		w.Out()
		w.Line("} else {")
	case required:
		// Required plain scalar: no presence check is possible.
		opened = false
	case g.optionalFor(member, target):
		w.Line("if %s.HasValue() {", field)
	case hasNilPresence:
		w.Line("if %s != nil {", field)
	default:
		return nil
	}
	if opened {
		w.In()
	}
	err := g.emitMemberValue(w, member, target, field, path)
	if opened {
		w.Out()
		w.Line("}")
	}
	return err
}

// fieldHasNilPresence reports whether field is a pointer or nilable value that
// can be compared with nil. Required plain scalars and Optional wrappers are
// not nil-checkable.
func (g *generator) fieldHasNilPresence(member, target *model.Shape) bool {
	if g.optionalFor(member, target) || g.fieldIsPlainValue(member, target) {
		return false
	}
	return true
}

func (g *generator) fieldValueExpr(member, target *model.Shape, field string) string {
	switch {
	case g.optionalFor(member, target):
		return field + ".MustValue()"
	case g.fieldIsPlainValue(member, target):
		return field
	case fieldHasPointerWrapper(target):
		return "*" + field
	default:
		return field
	}
}

// memberValueChecks reports whether emitMemberValue will emit anything for a
// member after the presence check.
func (g *generator) memberValueChecks(member, target *model.Shape) bool {
	switch target.Type {
	case model.TypeStructure, model.TypeList, model.TypeSet, model.TypeMap:
		return g.hasGeneratedValidator(target)
	case model.TypeUnion:
		return false
	default:
		return g.constraintsFor(member, target).hasApplicable(target)
	}
}

func (g *generator) emitMemberValue(w *fileWriter, member, target *model.Shape, expr, path string) error {
	switch target.Type {
	case model.TypeList, model.TypeSet, model.TypeMap:
		if !g.hasGeneratedValidator(target) {
			g.diagnosticAt(validation.Danger, member.Source,
				"Member `%s` targets `%s`, which has no generated validator", member.ID, target.ID)
			return nil
		}
		if g.memberOverrides(member, target) {
			c := g.constraintsFor(member, target)
			if err := g.emitConstraintChecks(w, target, c, expr, path); err != nil {
				return err
			}
			w.Line("%s.validateContents(&errs, %s)", expr, path)
			return nil
		}
		w.Line("errs.AddNested(%s, %s.Validate())", path, expr)
		return nil
	case model.TypeStructure:
		if !g.hasGeneratedValidator(target) {
			g.diagnosticAt(validation.Danger, member.Source,
				"Member `%s` targets `%s`, which has no generated validator", member.ID, target.ID)
			return nil
		}
		w.Line("errs.AddNested(%s, %s.Validate())", path, expr)
		return nil
	case model.TypeUnion:
		// Unions expose no Validate method; requiredness is checked
		// through the member itself.
		return nil
	}

	c := g.constraintsFor(member, target)
	if !c.hasApplicable(target) {
		return nil
	}
	valueExpr := g.fieldValueExpr(member, target, expr)
	return g.emitConstraintChecks(w, target, c, valueExpr, path)
}

func (g *generator) emitCollectionValidator(w *fileWriter, shape *model.Shape) error {
	name, ok := g.typeName(shape)
	if !ok {
		return nil
	}
	g.addSmithyImport(w)
	w.Line("// Validate checks the constraints of %s.", name)
	w.Line("func (v %s) Validate() error {", name)
	w.In()
	w.Line("var errs smithy.ValidationErrors")
	c := g.constraintsFor(nil, shape)
	if err := g.emitConstraintChecks(w, shape, c, "v", quoted("")); err != nil {
		return err
	}
	w.Line("v.validateContents(&errs, \"\")")
	w.Line("return errs.Err()")
	w.Out()
	w.Line("}")
	w.Line("")

	w.Line("func (v %s) validateContents(errs *smithy.ValidationErrors, prefix string) {", name)
	w.In()
	if err := g.emitCollectionContents(w, shape); err != nil {
		return err
	}
	w.Out()
	w.Line("}")
	w.Line("")
	return nil
}

func (g *generator) emitCollectionContents(w *fileWriter, shape *model.Shape) error {
	switch shape.Type {
	case model.TypeList, model.TypeSet:
		member, ok := shape.GetMember("member")
		if !ok {
			g.diagnosticAt(validation.Danger, shape.Source, "List `%s` has no member", shape.ID)
			return nil
		}
		target, ok := g.m.Shape(member.Target)
		if !ok {
			g.diagnosticAt(validation.Danger, member.Source,
				"Member `%s` targets `%s`, which is not found in the model", member.ID, member.Target)
			return nil
		}
		if !g.needsElementValidation(member, target) {
			return nil
		}
		w.AddImport("fmt", "")
		w.Line("for i := range v {")
		w.In()
		w.Line("elem := v[i]")
		w.Line("path := fmt.Sprintf(\"%%s[%%d]\", prefix, i)")
		sparse := shape.IsSparse()
		if sparse {
			w.Line("if elem != nil {")
			w.In()
		}
		err := g.emitElementValue(w, member, target, "elem", "path", sparse)
		if sparse {
			w.Out()
			w.Line("}")
		}
		w.Out()
		w.Line("}")
		return err
	case model.TypeMap:
		member, ok := shape.GetMember("value")
		if !ok {
			g.diagnosticAt(validation.Danger, shape.Source, "Map `%s` has no value member", shape.ID)
			return nil
		}
		target, ok := g.m.Shape(member.Target)
		if !ok {
			g.diagnosticAt(validation.Danger, member.Source,
				"Member `%s` targets `%s`, which is not found in the model", member.ID, member.Target)
			return nil
		}
		if !g.needsElementValidation(member, target) {
			return nil
		}
		w.AddImport("fmt", "")
		w.Line("for k, elem := range v {")
		w.In()
		w.Line("path := fmt.Sprintf(\"%%s[%%v]\", prefix, k)")
		sparse := shape.IsSparse()
		if sparse {
			w.Line("if elem != nil {")
			w.In()
		}
		err := g.emitElementValue(w, member, target, "elem", "path", sparse)
		if sparse {
			w.Out()
			w.Line("}")
		}
		w.Out()
		w.Line("}")
		return err
	default:
		return nil
	}
}

func (g *generator) needsElementValidation(member, target *model.Shape) bool {
	switch target.Type {
	case model.TypeStructure, model.TypeList, model.TypeSet, model.TypeMap:
		return true
	case model.TypeUnion, model.TypeEnum, model.TypeIntEnum:
		return false
	default:
		return g.constraintsFor(member, target).hasApplicable(target)
	}
}

func (g *generator) emitElementValue(w *fileWriter, member, target *model.Shape, expr, path string, sparse bool) error {
	switch target.Type {
	case model.TypeStructure:
		if !g.hasGeneratedValidator(target) {
			g.diagnosticAt(validation.Danger, member.Source,
				"Element target `%s` has no generated validator", target.ID)
			return nil
		}
		w.Line("errs.AddNested(%s, %s.Validate())", path, expr)
		return nil
	case model.TypeList, model.TypeSet, model.TypeMap:
		if !g.hasGeneratedValidator(target) {
			g.diagnosticAt(validation.Danger, member.Source,
				"Element target `%s` has no generated validator", target.ID)
			return nil
		}
		if g.memberOverrides(member, target) {
			c := g.constraintsFor(member, target)
			valueExpr := g.elementValueExpr(target, expr, sparse)
			if err := g.emitConstraintChecks(w, target, c, valueExpr, path); err != nil {
				return err
			}
			w.Line("%s.validateContents(errs, %s)", expr, path)
			return nil
		}
		w.Line("errs.AddNested(%s, %s.Validate())", path, expr)
		return nil
	case model.TypeUnion:
		return nil
	}

	c := g.constraintsFor(member, target)
	if !c.hasApplicable(target) {
		return nil
	}
	valueExpr := g.elementValueExpr(target, expr, sparse)
	return g.emitConstraintChecks(w, target, c, valueExpr, path)
}

// elementValueExpr returns an expression that yields the value of a collection
// element, dereferencing pointerized elements when needed.
func (g *generator) elementValueExpr(target *model.Shape, expr string, sparse bool) string {
	if !sparse {
		return expr
	}
	switch target.Type {
	case model.TypeBlob, model.TypeDocument, model.TypeBigInteger:
		return expr
	case model.TypeBigDecimal:
		if sparse {
			return "*" + expr
		}
		return expr
	case model.TypeList, model.TypeSet, model.TypeMap:
		if _, named := g.typeNames[target.ID]; named {
			return "*" + expr
		}
		return expr
	default:
		return "*" + expr
	}
}

func (g *generator) emitConstraintChecks(w *fileWriter, target *model.Shape, c constraints, expr, pathExpr string) error {
	if target == nil {
		return nil
	}

	if c.lengthMin != nil || c.lengthMax != nil {
		lengthExpr, ok := lengthExpression(target, g.stringValueExpr(target, expr))
		if ok {
			_, unit := lengthNoun(target)
			if target.Type == model.TypeString {
				w.AddImport("unicode/utf8", "")
			}
			if c.lengthMin != nil {
				w.Line("if %s < %d {", lengthExpr, *c.lengthMin)
				w.In()
				w.Line("errs.Add(%s, %s)", pathExpr, quoted(lengthMessage(true, *c.lengthMin, unit)))
				w.Out()
				w.Line("}")
			}
			if c.lengthMax != nil {
				w.Line("if %s > %d {", lengthExpr, *c.lengthMax)
				w.In()
				w.Line("errs.Add(%s, %s)", pathExpr, quoted(lengthMessage(false, *c.lengthMax, unit)))
				w.Out()
				w.Line("}")
			}
		}
	}

	if c.rangeMin != nil {
		switch target.Type {
		case model.TypeBigInteger:
			g.needsBigIntRange = true
		case model.TypeBigDecimal:
			g.needsBigDecimalRange = true
		}
		condition, err := rangeCondition(target, expr, c.rangeMin, true)
		if err != nil {
			return fmt.Errorf("golang: @range on `%s`: %w", target.ID, err)
		}
		w.Line("if %s {", condition)
		w.In()
		w.Line("errs.Add(%s, %s)", pathExpr, quoted(rangeMessage(target, c.rangeMin, true)))
		w.Out()
		w.Line("}")
	}
	if c.rangeMax != nil {
		switch target.Type {
		case model.TypeBigInteger:
			g.needsBigIntRange = true
		case model.TypeBigDecimal:
			g.needsBigDecimalRange = true
		}
		condition, err := rangeCondition(target, expr, c.rangeMax, false)
		if err != nil {
			return fmt.Errorf("golang: @range on `%s`: %w", target.ID, err)
		}
		w.Line("if %s {", condition)
		w.In()
		w.Line("errs.Add(%s, %s)", pathExpr, quoted(rangeMessage(target, c.rangeMax, false)))
		w.Out()
		w.Line("}")
	}

	if c.pattern != nil && target.Type == model.TypeString {
		name, err := g.registerPattern(*c.pattern, target.ID, target.Source)
		if err != nil {
			return err
		}
		w.AddImport("regexp", "")
		w.Line("if !%s.MatchString(%s) {", name, g.stringValueExpr(target, expr))
		w.In()
		w.Line("errs.Add(%s, %s)", pathExpr, quoted("must match pattern `"+*c.pattern+"`"))
		w.Out()
		w.Line("}")
	}

	if c.uniqueItems && (target.Type == model.TypeList || target.Type == model.TypeSet) {
		w.AddImport("reflect", "")
		w.AddImport("fmt", "")
		w.Line("for i := 0; i < len(%s); i++ {", expr)
		w.In()
		w.Line("for j := i + 1; j < len(%s); j++ {", expr)
		w.In()
		w.Line("if reflect.DeepEqual((%s)[i], (%s)[j]) {", expr, expr)
		w.In()
		w.Line("errs.Add(fmt.Sprintf(\"%%s[%%d]\", %s, i), %s)", pathExpr, quoted("must contain unique items"))
		w.Line("break")
		w.Out()
		w.Line("}")
		w.Out()
		w.Line("}")
		w.Out()
		w.Line("}")
	}
	return nil
}

func (g *generator) registerPattern(pattern string, owner model.ShapeID, source idl.SourceLocation) (string, error) {
	if name, ok := g.patterns[pattern]; ok {
		return name, nil
	}
	if _, err := regexp.Compile(pattern); err != nil {
		g.diagnosticAt(validation.Danger, source,
			"@pattern %q on `%s` is not supported by Go's RE2 regular expressions: %v", pattern, owner, err)
		return "", fmt.Errorf("golang: invalid @pattern %q on `%s`: %w", pattern, owner, err)
	}
	name := fmt.Sprintf("pattern%d", len(g.patternOrder))
	g.patterns[pattern] = name
	g.patternOrder = append(g.patternOrder, pattern)
	return name, nil
}

// stringValueExpr converts named string types (including @enum string shapes)
// to string where a constraint helper requires the predeclared type.
func (g *generator) stringValueExpr(target *model.Shape, expr string) string {
	if target.Type == model.TypeString {
		if _, named := g.typeNames[target.ID]; named {
			return "string(" + expr + ")"
		}
	}
	return expr
}

func (g *generator) addSmithyImport(w *fileWriter) {
	w.AddImport(g.module+"/internal/smithy", "")
}
