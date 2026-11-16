package golang

import (
	"fmt"
	"strings"

	"github.com/tespeleta/gosmithy/model"
	"github.com/tespeleta/gosmithy/protocol/httpjson"
	"github.com/tespeleta/gosmithy/validation"
)

// optionalFor reports whether member is a non-required scalar represented by
// Optional[T]. HTTP-bound and payload members keep their pointer form because
// the protocol binding code assigns them directly.
func (g *generator) optionalFor(member, target *model.Shape) bool {
	if !g.useOptionals || member == nil || target == nil {
		return false
	}
	if member.Required() || !isOptionalScalarType(target.Type) {
		return false
	}
	if g.isHTTPBoundOrPayload(member) {
		return false
	}
	container, ok := g.m.Shape(member.Container)
	return ok && container.Type == model.TypeStructure
}

// fieldIsPlainValue reports whether member is a required scalar represented
// by its plain Go value type rather than a pointer or Optional.
func (g *generator) fieldIsPlainValue(member, target *model.Shape) bool {
	if !g.useOptionals || member == nil || target == nil {
		return false
	}
	if !member.Required() || !isOptionalScalarType(target.Type) {
		return false
	}
	if target.Type == model.TypeBigInteger || g.isHTTPBoundOrPayload(member) {
		return false
	}
	container, ok := g.m.Shape(member.Container)
	return ok && container.Type == model.TypeStructure
}

// requiredPresenceTracked reports whether member is a required scalar stored as
// a plain Go value. Its presence cannot be inferred from the value, so the
// generated JSON decoder records whether it was absent or null.
func (g *generator) requiredPresenceTracked(member, target *model.Shape) bool {
	return g.fieldIsPlainValue(member, target)
}

// missingFieldName is the unexported field recording whether a required plain
// scalar member was absent or null in the decoded JSON.
func (g *generator) missingFieldName(member *model.Shape) string {
	return "missing" + g.fieldName(member)
}

// needsRequiredPresence reports whether shape has a required plain scalar
// member whose presence the generated JSON decoder must record.
func (g *generator) needsRequiredPresence(shape *model.Shape) bool {
	if shape == nil || shape.Type != model.TypeStructure {
		return false
	}
	for _, member := range shape.Members {
		target, ok := g.m.Shape(member.Target)
		if ok && g.requiredPresenceTracked(member, target) {
			return true
		}
	}
	return false
}

func isOptionalScalarType(t model.ShapeType) bool {
	switch t {
	case model.TypeString, model.TypeBoolean,
		model.TypeByte, model.TypeShort, model.TypeInteger, model.TypeLong,
		model.TypeFloat, model.TypeDouble,
		model.TypeBigInteger, model.TypeBigDecimal,
		model.TypeTimestamp, model.TypeEnum, model.TypeIntEnum:
		return true
	default:
		return false
	}
}

func (g *generator) isHTTPBoundOrPayload(member *model.Shape) bool {
	return isHTTPStringBinding(member) || (member != nil && member.HTTPPayload())
}

func (g *generator) optionalInnerType(member, target *model.Shape, w *fileWriter) (string, bool) {
	if target.Type == model.TypeBoolean && g.isRelaxed(member) {
		return "RelaxedBool", true
	}
	return g.valueType(target, w)
}

func (g *generator) plainInnerType(member, target *model.Shape, w *fileWriter) (string, bool) {
	if target.Type == model.TypeBoolean && g.isRelaxed(member) {
		return "RelaxedBool", true
	}
	return g.valueType(target, w)
}

func (g *generator) fieldJSONTag(member, target *model.Shape) string {
	name := g.memberJSONName(member)
	switch {
	case g.optionalFor(member, target):
		return name + ",omitzero"
	case g.fieldIsPlainValue(member, target):
		return name
	default:
		return name + ",omitempty"
	}
}

// computeOptionalNeeds records whether the types package needs Optional[T].
func (g *generator) computeOptionalNeeds() {
	if !g.useOptionals {
		return
	}
	for _, shape := range g.shapes {
		if shape.Type != model.TypeStructure {
			continue
		}
		for _, member := range shape.Members {
			target, ok := g.m.Shape(member.Target)
			if ok && g.optionalFor(member, target) {
				g.needsOptional = true
				return
			}
		}
	}
}

// goVersion returns the Go language version written into the generated go.mod.
// Optional[T] omission uses encoding/json's omitzero, added in Go 1.24.
func (g *generator) goVersion() string {
	if g.useOptionals {
		return "1.24"
	}
	return "1.22"
}

// --- relaxed-input wiring -----------------------------------------------------

// computeRelaxedInput resolves the effective relaxed-input setting for every
// scalar member in the operation input closure. A member trait wins over the
// service trait; bound HTTP string members are not relaxed because their wire
// form is already a string.
func (g *generator) computeRelaxedInput(closure []model.ShapeID) {
	g.inputShapes = make(map[model.ShapeID]bool)
	for _, id := range closure {
		op, ok := g.m.Shape(id)
		if !ok || op.Type != model.TypeOperation || op.Input.IsZero() {
			continue
		}
		g.markInputShape(op.Input)
	}

	serviceRelaxed, _ := relaxedInputEnabled(g.service)
	g.relaxed = make(map[model.ShapeID]bool)
	g.needsDecimal = g.usesBigDecimal()

	for _, shape := range g.shapes {
		if shape.Type != model.TypeStructure && shape.Type != model.TypeUnion {
			continue
		}
		inInput := g.inputShapes[shape.ID]
		for _, member := range shape.Members {
			target, ok := g.m.Shape(member.Target)
			if !ok {
				continue
			}
			memberRelaxed, hasMemberTrait := relaxedInputEnabled(member)
			enabled := serviceRelaxed
			if hasMemberTrait {
				enabled = memberRelaxed
			}
			if !enabled {
				continue
			}
			if hasMemberTrait && !inInput {
				g.diagnosticAt(validation.Danger, member.Source,
					"`@relaxedInput` only applies to operation inputs; `%s` is not reachable from an operation input", member.ID)
				continue
			}
			if !inInput {
				continue
			}
			if isHTTPStringBinding(member) {
				if hasMemberTrait {
					g.diagnosticAt(validation.Danger, member.Source,
						"`@relaxedInput` does not apply to members bound to an HTTP label, query, header, or response code: `%s`", member.ID)
				}
				continue
			}
			if !isRelaxableScalarType(target.Type) {
				continue
			}
			g.relaxed[member.ID] = true
			switch target.Type {
			case model.TypeBoolean:
				g.needsRelaxedBool = true
			case model.TypeByte, model.TypeShort, model.TypeInteger, model.TypeLong,
				model.TypeFloat, model.TypeDouble, model.TypeBigInteger:
				g.needsRelaxedNumbers = true
			}
		}
	}
}

// markInputShape adds id and every shape reachable from it to inputShapes.
func (g *generator) markInputShape(id model.ShapeID) {
	if id.IsZero() || g.inputShapes[id] {
		return
	}
	g.inputShapes[id] = true
	shape, ok := g.m.Shape(id)
	if !ok {
		return
	}
	for _, member := range shape.Members {
		g.markInputShape(member.Target)
	}
}

// usesBigDecimal reports whether the service closure contains a bigDecimal
// member target.
func (g *generator) usesBigDecimal() bool {
	for _, shape := range g.shapes {
		if shape.Type == model.TypeBigDecimal {
			return true
		}
		for _, member := range shape.Members {
			target, ok := g.m.Shape(member.Target)
			if ok && target.Type == model.TypeBigDecimal {
				return true
			}
		}
	}
	return false
}

// reserveGeneratedTypes records the helper types emitted into the types
// package so user shape names cannot silently collide with them.
func (g *generator) reserveGeneratedTypes() {
	if g.needsDecimal {
		g.reserveGeneratedType("Decimal")
	}
	if g.needsRelaxedBool {
		g.reserveGeneratedType("RelaxedBool")
	}
	if g.needsOptional {
		g.reserveGeneratedType("Optional")
	}
}

func (g *generator) reserveGeneratedType(name string) {
	if previous, exists := g.packageNames[name]; exists {
		g.diagnosticAt(validation.Danger, g.service.Source,
			"Go type name %q for a generated helper collides with `%s`; rename the shape", name, previous)
		return
	}
	g.packageNames[name] = model.MustShapeID("gosmithy.generated#" + name)
}

// relaxedInputEnabled reports the effective enabled value of @relaxedInput and
// whether the trait is present. Bare applications default to true.
func relaxedInputEnabled(shape *model.Shape) (bool, bool) {
	if shape == nil {
		return false, false
	}
	trait, ok := shape.GetTrait(httpjson.RelaxedInputTrait)
	if !ok {
		return false, false
	}
	if value, ok := trait.Value.Get("enabled"); ok && value.Kind == model.NodeBoolean {
		return value.Bool, true
	}
	return true, true
}

// isRelaxed reports whether a member's effective setting is relaxed.
func (g *generator) isRelaxed(member *model.Shape) bool {
	return member != nil && g.relaxed[member.ID]
}

// isRelaxedNumberTarget reports whether t is an integer or floating-point
// target handled by the relaxed numeric helpers. BigDecimal is handled by
// Decimal.UnmarshalDecimalRelaxed instead.
func isRelaxedNumberTarget(t model.ShapeType) bool {
	switch t {
	case model.TypeByte, model.TypeShort, model.TypeInteger, model.TypeLong,
		model.TypeFloat, model.TypeDouble, model.TypeBigInteger:
		return true
	default:
		return false
	}
}

func relaxedNumberHelper(t model.ShapeType) (string, bool) {
	switch t {
	case model.TypeByte:
		return "smithy.UnmarshalRelaxedInt8", true
	case model.TypeShort:
		return "smithy.UnmarshalRelaxedInt16", true
	case model.TypeInteger:
		return "smithy.UnmarshalRelaxedInt32", true
	case model.TypeLong:
		return "smithy.UnmarshalRelaxedInt64", true
	case model.TypeFloat:
		return "smithy.UnmarshalRelaxedFloat32", true
	case model.TypeDouble:
		return "smithy.UnmarshalRelaxedFloat64", true
	case model.TypeBigInteger:
		return "smithy.UnmarshalRelaxedBigInteger", true
	default:
		return "", false
	}
}

func relaxedNumberGoType(t model.ShapeType) (string, bool) {
	switch t {
	case model.TypeByte:
		return "int8", true
	case model.TypeShort:
		return "int16", true
	case model.TypeInteger:
		return "int32", true
	case model.TypeLong:
		return "int64", true
	case model.TypeFloat:
		return "float32", true
	case model.TypeDouble:
		return "float64", true
	default:
		return "", false
	}
}

// emitRelaxedNumberAssign decodes raw into dest using the helper for target.
// When address is true, dest is a pointer field or sparse element.
func (g *generator) emitRelaxedNumberAssign(w *fileWriter, target *model.Shape, rawExpr, dest string, address bool, fail string) {
	if target.Type == model.TypeBigInteger {
		w.AddImport("math/big", "")
		g.addSmithyImport(w)
		w.Line("if string(%s) == \"null\" {", rawExpr)
		w.In()
		w.Line("%s = nil", dest)
		w.Out()
		w.Line("} else {")
		w.In()
		w.Line("value := new(big.Int)")
		w.Line("if err := smithy.UnmarshalRelaxedBigInteger(%s, value); err != nil {", rawExpr)
		w.In()
		w.Line("%s", fail)
		w.Out()
		w.Line("}")
		w.Line("%s = value", dest)
		w.Out()
		w.Line("}")
		return
	}
	goType, ok := relaxedNumberGoType(target.Type)
	helper, okHelper := relaxedNumberHelper(target.Type)
	if !ok || !okHelper {
		w.Line("_ = raw")
		return
	}
	g.addSmithyImport(w)
	w.Line("if string(%s) == \"null\" {", rawExpr)
	w.In()
	if address {
		w.Line("%s = nil", dest)
	}
	w.Out()
	w.Line("} else {")
	w.In()
	w.Line("var value %s", goType)
	w.Line("if err := %s(%s, &value); err != nil {", helper, rawExpr)
	w.In()
	w.Line("%s", fail)
	w.Out()
	w.Line("}")
	if address {
		w.Line("%s = &value", dest)
	} else {
		w.Line("%s = value", dest)
	}
	w.Out()
	w.Line("}")
}

// emitRelaxedNumberCall decodes raw into an existing numeric value variable.
// It is used by union decoders, where the wrapper stores the value.
func (g *generator) emitRelaxedNumberCall(w *fileWriter, target *model.Shape, rawExpr, dest string, fail string) {
	helper, ok := relaxedNumberHelper(target.Type)
	if !ok {
		w.Line("_ = raw")
		return
	}
	g.addSmithyImport(w)
	w.Line("if string(%s) != \"null\" {", rawExpr)
	w.In()
	w.Line("if err := %s(%s, &%s); err != nil {", helper, rawExpr, dest)
	w.In()
	w.Line("%s", fail)
	w.Out()
	w.Line("}")
	w.Out()
	w.Line("}")
}

// emitRelaxedDecimalAssign decodes raw into a Decimal destination.
func (g *generator) emitRelaxedDecimalAssign(w *fileWriter, rawExpr, dest string, address bool, fail string) {
	w.Line("if string(%s) == \"null\" {", rawExpr)
	w.In()
	if address {
		w.Line("%s = nil", dest)
	}
	w.Out()
	w.Line("} else {")
	w.In()
	w.Line("var value Decimal")
	w.Line("if err := UnmarshalDecimalRelaxed(%s, &value); err != nil {", rawExpr)
	w.In()
	w.Line("%s", fail)
	w.Out()
	w.Line("}")
	if address {
		w.Line("%s = &value", dest)
	} else {
		w.Line("%s = value", dest)
	}
	w.Out()
	w.Line("}")
}

// needsRelaxedStructJSON reports whether shape needs an UnmarshalJSON method
// solely because it has relaxed numeric or decimal members. Boolean members
// use RelaxedBool and need no custom method.
func (g *generator) needsRelaxedStructJSON(shape *model.Shape) bool {
	if shape == nil || shape.Type != model.TypeStructure || g.needsCustomJSON(shape) {
		return false
	}
	for _, member := range shape.Members {
		target, ok := g.m.Shape(member.Target)
		if !ok || !g.isRelaxed(member) {
			continue
		}
		if isRelaxedNumberTarget(target.Type) || target.Type == model.TypeBigDecimal {
			return true
		}
	}
	return false
}

// emitOptionalUnmarshal decodes raw into an Optional[T] field.
func (g *generator) emitOptionalUnmarshal(w *fileWriter, member, target *model.Shape, rawExpr, dest, fail string) {
	format := ""
	if target.Type == model.TypeTimestamp {
		format = g.timestampFormatFor(member, target)
	}
	switch {
	case target.Type == model.TypeTimestamp && format != model.TimestampDateTime:
		g.addSmithyImport(w)
		w.Line("if string(%s) == \"null\" {", rawExpr)
		w.In()
		w.Line("%s.SetNull()", dest)
		w.Out()
		w.Line("} else {")
		w.In()
		w.Line("var text string")
		w.Line("if err := json.Unmarshal(%s, &text); err != nil {", rawExpr)
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
		w.Line("%s.Set(parsed)", dest)
		w.Out()
		w.Line("}")
	case g.isRelaxed(member) && target.Type == model.TypeBigInteger:
		w.AddImport("math/big", "")
		g.addSmithyImport(w)
		w.Line("if string(%s) == \"null\" {", rawExpr)
		w.In()
		w.Line("%s.SetNull()", dest)
		w.Out()
		w.Line("} else {")
		w.In()
		w.Line("value := new(big.Int)")
		w.Line("if err := smithy.UnmarshalRelaxedBigInteger(%s, value); err != nil {", rawExpr)
		w.In()
		w.Line("%s", fail)
		w.Out()
		w.Line("}")
		w.Line("%s.Set(value)", dest)
		w.Out()
		w.Line("}")
	case g.isRelaxed(member) && isRelaxedNumberTarget(target.Type):
		goType, _ := relaxedNumberGoType(target.Type)
		helper, _ := relaxedNumberHelper(target.Type)
		g.addSmithyImport(w)
		w.Line("if string(%s) == \"null\" {", rawExpr)
		w.In()
		w.Line("%s.SetNull()", dest)
		w.Out()
		w.Line("} else {")
		w.In()
		w.Line("var value %s", goType)
		w.Line("if err := %s(%s, &value); err != nil {", helper, rawExpr)
		w.In()
		w.Line("%s", fail)
		w.Out()
		w.Line("}")
		w.Line("%s.Set(value)", dest)
		w.Out()
		w.Line("}")
	case g.isRelaxed(member) && target.Type == model.TypeBigDecimal:
		w.Line("if string(%s) == \"null\" {", rawExpr)
		w.In()
		w.Line("%s.SetNull()", dest)
		w.Out()
		w.Line("} else {")
		w.In()
		w.Line("var value Decimal")
		w.Line("if err := UnmarshalDecimalRelaxed(%s, &value); err != nil {", rawExpr)
		w.In()
		w.Line("%s", fail)
		w.Out()
		w.Line("}")
		w.Line("%s.Set(value)", dest)
		w.Out()
		w.Line("}")
	default:
		w.AddImport("encoding/json", "")
		w.Line("if err := json.Unmarshal(%s, &%s); err != nil {", rawExpr, dest)
		w.In()
		w.Line("%s", fail)
		w.Out()
		w.Line("}")
	}
}

// emitPlainUnmarshal decodes raw into a required plain scalar field.
func (g *generator) emitPlainUnmarshal(w *fileWriter, member, target *model.Shape, rawExpr, dest, fail string) {
	format := ""
	if target.Type == model.TypeTimestamp {
		format = g.timestampFormatFor(member, target)
	}
	switch {
	case target.Type == model.TypeTimestamp && format != model.TimestampDateTime:
		g.addSmithyImport(w)
		w.Line("var text string")
		w.Line("if err := json.Unmarshal(%s, &text); err != nil {", rawExpr)
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
		w.Line("%s = parsed", dest)
	case g.isRelaxed(member) && isRelaxedNumberTarget(target.Type):
		g.emitRelaxedNumberAssign(w, target, rawExpr, dest, false, fail)
	case g.isRelaxed(member) && target.Type == model.TypeBigDecimal:
		g.emitRelaxedDecimalAssign(w, rawExpr, dest, false, fail)
	default:
		w.AddImport("encoding/json", "")
		w.Line("if err := json.Unmarshal(%s, &%s); err != nil {", rawExpr, dest)
		w.In()
		w.Line("%s", fail)
		w.Out()
		w.Line("}")
	}
}

// emitStructUnmarshal writes the UnmarshalJSON method for a structure that has
// custom member codecs, relaxed numeric members, or required scalar presence to
// record. It decodes the object into raw member values and then decodes each
// present member explicitly, which preserves custom codecs and lets the method
// record whether a required plain scalar was absent or null.
func (g *generator) emitStructUnmarshal(w *fileWriter, shape *model.Shape) {
	name, ok := g.typeName(shape)
	if !ok {
		return
	}
	w.AddImport("encoding/json", "")
	if len(shape.Members) > 0 {
		w.AddImport("fmt", "")
	}
	w.Line("// UnmarshalJSON implements json.Unmarshaler using the httpJson value encoding.")
	w.Line("func (v *%s) UnmarshalJSON(data []byte) error {", name)
	w.In()
	w.Line("var fields map[string]json.RawMessage")
	w.Line("if err := json.Unmarshal(data, &fields); err != nil {")
	w.In()
	w.Line("return err")
	w.Out()
	w.Line("}")
	for _, member := range shape.Members {
		target, ok := g.m.Shape(member.Target)
		if !ok {
			continue
		}
		field := g.fieldName(member)
		jsonName := quoted(g.memberJSONName(member))
		fail := fmt.Sprintf("return fmt.Errorf(%q, err)", memberErrorFormat(g.memberJSONName(member)))
		if g.requiredPresenceTracked(member, target) {
			missing := g.missingFieldName(member)
			w.Line("v.%s = true", missing)
			w.Line("if raw, ok := fields[%s]; ok && string(raw) != \"null\" {", jsonName)
			w.In()
			w.Line("v.%s = false", missing)
			g.emitUnmarshalValue(w, member, target, "raw", "v."+field, fieldHasPointerWrapper(target), fail)
			w.Out()
			w.Line("}")
			continue
		}
		w.Line("if raw, ok := fields[%s]; ok {", jsonName)
		w.In()
		g.emitUnmarshalValue(w, member, target, "raw", "v."+field, fieldHasPointerWrapper(target), fail)
		w.Out()
		w.Line("}")
	}
	w.Line("return nil")
	w.Out()
	w.Line("}")
}

// memberErrorFormat returns the fmt.Errorf format used for a member path. A
// percent sign in a @jsonName is escaped so it cannot become a format verb.
func memberErrorFormat(jsonName string) string {
	return "member " + strings.ReplaceAll(jsonName, "%", "%%") + ": %w"
}

func isRelaxableScalarType(t model.ShapeType) bool {
	switch t {
	case model.TypeBoolean,
		model.TypeByte, model.TypeShort, model.TypeInteger, model.TypeLong,
		model.TypeFloat, model.TypeDouble,
		model.TypeBigInteger, model.TypeBigDecimal:
		return true
	default:
		return false
	}
}

// isHTTPStringBinding reports whether a member's value is carried as an HTTP
// string rather than a JSON body value.
func isHTTPStringBinding(member *model.Shape) bool {
	if member == nil {
		return false
	}
	if _, ok := member.HTTPQuery(); ok {
		return true
	}
	if _, ok := member.HTTPHeader(); ok {
		return true
	}
	if _, ok := member.HTTPPrefixHeaders(); ok {
		return true
	}
	if member.HTTPQueryParams() || member.HTTPLabel() || member.HTTPResponseCode() {
		return true
	}
	return false
}

// --- generated wire types -----------------------------------------------------

// emitOptionalFile writes types.Optional, the presence wrapper used for
// non-required scalar structure members.
func (g *generator) emitOptionalFile() ([]byte, error) {
	w := newFileWriter("types", "Optional wraps a non-required scalar member so that absent, explicit null, and a value are distinct.")
	w.AddImport("bytes", "")
	w.AddImport("encoding/json", "")

	w.Line("// Optional is an absent, null, or present scalar value.")
	w.Line("type Optional[T any] struct {")
	w.In()
	w.Line("value T")
	w.Line("set   bool")
	w.Line("null  bool")
	w.Out()
	w.Line("}")
	w.Line("")
	w.Line("// NewOptional returns a present Optional.")
	w.Line("func NewOptional[T any](value T) Optional[T] {")
	w.In()
	w.Line("return Optional[T]{value: value, set: true}")
	w.Out()
	w.Line("}")
	w.Line("")
	w.Line("// NewOptionalNull returns an Optional that was explicitly null.")
	w.Line("func NewOptionalNull[T any]() Optional[T] {")
	w.In()
	w.Line("var zero T")
	w.Line("return Optional[T]{value: zero, set: true, null: true}")
	w.Out()
	w.Line("}")
	w.Line("")
	w.Line("// NewOptionalAbsent returns an absent Optional.")
	w.Line("func NewOptionalAbsent[T any]() Optional[T] { return Optional[T]{} }")
	w.Line("")
	w.Line("// IsSet reports whether the member was present, including an explicit null.")
	w.Line("func (o Optional[T]) IsSet() bool { return o.set }")
	w.Line("")
	w.Line("// IsAbsent reports whether the member was not present.")
	w.Line("func (o Optional[T]) IsAbsent() bool { return !o.set }")
	w.Line("")
	w.Line("// IsNull reports whether the member was present and explicitly null.")
	w.Line("func (o Optional[T]) IsNull() bool { return o.set && o.null }")
	w.Line("")
	w.Line("// HasValue reports whether the member was present with a non-null value.")
	w.Line("func (o Optional[T]) HasValue() bool { return o.set && !o.null }")
	w.Line("")
	w.Line("// Value returns the wrapped value and whether it is present and non-null.")
	w.Line("func (o Optional[T]) Value() (T, bool) { return o.value, o.set && !o.null }")
	w.Line("")
	w.Line("// ValueOr returns the wrapped value, or fallback when absent or null.")
	w.Line("func (o Optional[T]) ValueOr(fallback T) T {")
	w.In()
	w.Line("if o.set && !o.null {")
	w.In()
	w.Line("return o.value")
	w.Out()
	w.Line("}")
	w.Line("return fallback")
	w.Out()
	w.Line("}")
	w.Line("")
	w.Line("// MustValue returns the wrapped value and panics when absent or null.")
	w.Line("func (o Optional[T]) MustValue() T {")
	w.In()
	w.Line("if !o.set {")
	w.In()
	w.Line("panic(%s)", quoted("optional value is absent"))
	w.Out()
	w.Line("}")
	w.Line("if o.null {")
	w.In()
	w.Line("panic(%s)", quoted("optional value is null"))
	w.Out()
	w.Line("}")
	w.Line("return o.value")
	w.Out()
	w.Line("}")
	w.Line("")
	w.Line("// Ptr returns a pointer to a copy of the value, or nil when absent or null.")
	w.Line("func (o Optional[T]) Ptr() *T {")
	w.In()
	w.Line("if !o.set || o.null {")
	w.In()
	w.Line("return nil")
	w.Out()
	w.Line("}")
	w.Line("value := o.value")
	w.Line("return &value")
	w.Out()
	w.Line("}")
	w.Line("")
	w.Line("// Set marks the Optional present with value.")
	w.Line("func (o *Optional[T]) Set(value T) {")
	w.In()
	w.Line("o.value = value")
	w.Line("o.set = true")
	w.Line("o.null = false")
	w.Out()
	w.Line("}")
	w.Line("")
	w.Line("// SetNull marks the Optional present and explicitly null.")
	w.Line("func (o *Optional[T]) SetNull() {")
	w.In()
	w.Line("var zero T")
	w.Line("o.value = zero")
	w.Line("o.set = true")
	w.Line("o.null = true")
	w.Out()
	w.Line("}")
	w.Line("")
	w.Line("// Unset marks the Optional absent.")
	w.Line("func (o *Optional[T]) Unset() {")
	w.In()
	w.Line("var zero T")
	w.Line("o.value = zero")
	w.Line("o.set = false")
	w.Line("o.null = false")
	w.Out()
	w.Line("}")
	w.Line("")
	w.Line("// IsZero lets encoding/json omit absent fields with the omitzero tag.")
	w.Line("func (o Optional[T]) IsZero() bool { return !o.set }")
	w.Line("")
	w.Line("// MarshalJSON encodes absent/null as null and otherwise encodes the value.")
	w.Line("func (o Optional[T]) MarshalJSON() ([]byte, error) {")
	w.In()
	w.Line("if !o.set || o.null {")
	w.In()
	w.Line("return []byte(\"null\"), nil")
	w.Out()
	w.Line("}")
	w.Line("return json.Marshal(o.value)")
	w.Out()
	w.Line("}")
	w.Line("")
	w.Line("// UnmarshalJSON records presence and distinguishes null from a value.")
	w.Line("func (o *Optional[T]) UnmarshalJSON(data []byte) error {")
	w.In()
	w.Line("text := bytes.TrimSpace(data)")
	w.Line("if bytes.Equal(text, []byte(\"null\")) {")
	w.In()
	w.Line("o.SetNull()")
	w.Line("return nil")
	w.Out()
	w.Line("}")
	w.Line("var value T")
	w.Line("if err := json.Unmarshal(text, &value); err != nil {")
	w.In()
	w.Line("var zero T")
	w.Line("o.value = zero")
	w.Line("o.set = false")
	w.Line("o.null = false")
	w.Line("return err")
	w.Out()
	w.Line("}")
	w.Line("o.Set(value)")
	w.Line("return nil")
	w.Out()
	w.Line("}")
	return w.Render()
}

// emitDecimalFile writes types.Decimal, the exact wire representation of a
// Smithy bigDecimal.
func (g *generator) emitDecimalFile() ([]byte, error) {
	w := newFileWriter("types", "Decimal is the exact wire representation of a Smithy bigDecimal. "+
		"It preserves the JSON number literal instead of converting it to a binary float.")
	w.AddImport("bytes", "")
	w.AddImport("encoding/json", "")
	w.AddImport("fmt", "")
	w.AddImport("math/big", "")
	w.AddImport("strconv", "")

	w.Line("// Decimal is a fixed-point decimal value stored as its JSON number literal.")
	w.Line("// It preserves the original digits, including trailing zeros and exponents.")
	w.Line("type Decimal json.Number")
	w.Line("")
	w.Line("// NewDecimal validates value as a JSON number literal and returns it as a Decimal.")
	w.Line("func NewDecimal(value string) (Decimal, error) {")
	w.In()
	w.Line("if !isJSONNumberLiteral(value) {")
	w.In()
	w.Line("return \"\", fmt.Errorf(%s, value)", quoted("smithy: invalid decimal %q"))
	w.Out()
	w.Line("}")
	w.Line("return Decimal(value), nil")
	w.Out()
	w.Line("}")
	w.Line("")
	w.Line("// MustDecimal is NewDecimal for a literal that is known to be valid.")
	w.Line("func MustDecimal(value string) Decimal {")
	w.In()
	w.Line("d, err := NewDecimal(value)")
	w.Line("if err != nil {")
	w.In()
	w.Line("panic(err)")
	w.Out()
	w.Line("}")
	w.Line("return d")
	w.Out()
	w.Line("}")
	w.Line("")
	w.Line("// DecimalFromInt64 returns the decimal representation of value.")
	w.Line("func DecimalFromInt64(value int64) Decimal {")
	w.In()
	w.Line("return Decimal(strconv.FormatInt(value, 10))")
	w.Out()
	w.Line("}")
	w.Line("")
	w.Line("// DecimalFromBigInt returns the decimal representation of value.")
	w.Line("func DecimalFromBigInt(value *big.Int) Decimal {")
	w.In()
	w.Line("if value == nil {")
	w.In()
	w.Line("panic(%s)", quoted("smithy: nil big.Int"))
	w.Out()
	w.Line("}")
	w.Line("return Decimal(value.String())")
	w.Out()
	w.Line("}")
	w.Line("")
	w.Line("// String returns the stored decimal literal.")
	w.Line("func (d Decimal) String() string { return string(d) }")
	w.Line("")
	for _, line := range strings.Split(decimalCmpSource, "\n") {
		w.Line("%s", line)
	}
	w.Line("")
	w.Line("// MarshalJSON emits the decimal as a bare JSON number.")
	w.Line("func (d Decimal) MarshalJSON() ([]byte, error) {")
	w.In()
	w.Line("if d == \"\" {")
	w.In()
	w.Line("return nil, fmt.Errorf(%s)", quoted("smithy: empty decimal"))
	w.Out()
	w.Line("}")
	w.Line("if !isJSONNumberLiteral(string(d)) {")
	w.In()
	w.Line("return nil, fmt.Errorf(%s, string(d))", quoted("smithy: invalid decimal %q"))
	w.Out()
	w.Line("}")
	w.Line("return []byte(d), nil")
	w.Out()
	w.Line("}")
	w.Line("")
	w.Line("// UnmarshalJSON decodes a JSON number. Quoted strings are rejected; use")
	w.Line("// UnmarshalDecimalRelaxed for the relaxed-input path.")
	w.Line("func (d *Decimal) UnmarshalJSON(data []byte) error {")
	w.In()
	w.Line("text := bytes.TrimSpace(data)")
	w.Line("if bytes.Equal(text, []byte(\"null\")) {")
	w.In()
	w.Line("*d = \"\"")
	w.Line("return nil")
	w.Out()
	w.Line("}")
	w.Line("if len(text) > 0 && text[0] == '\"' {")
	w.In()
	w.Line("return fmt.Errorf(%s)", quoted("smithy: decimal must be a JSON number"))
	w.Out()
	w.Line("}")
	w.Line("if !isJSONNumberLiteral(string(text)) {")
	w.In()
	w.Line("return fmt.Errorf(%s, string(text))", quoted("smithy: invalid decimal %q"))
	w.Out()
	w.Line("}")
	w.Line("*d = Decimal(text)")
	w.Line("return nil")
	w.Out()
	w.Line("}")
	w.Line("")
	w.Line("// UnmarshalDecimalRelaxed decodes a JSON number or a quoted decimal string.")
	w.Line("func UnmarshalDecimalRelaxed(data []byte, d *Decimal) error {")
	w.In()
	w.Line("text := bytes.TrimSpace(data)")
	w.Line("if bytes.Equal(text, []byte(\"null\")) {")
	w.In()
	w.Line("*d = \"\"")
	w.Line("return nil")
	w.Out()
	w.Line("}")
	w.Line("if len(text) > 0 && text[0] == '\"' {")
	w.In()
	w.Line("var value string")
	w.Line("if err := json.Unmarshal(text, &value); err != nil {")
	w.In()
	w.Line("return err")
	w.Out()
	w.Line("}")
	w.Line("text = []byte(value)")
	w.Out()
	w.Line("}")
	w.Line("if !isJSONNumberLiteral(string(text)) {")
	w.In()
	w.Line("return fmt.Errorf(%s, string(text))", quoted("smithy: invalid decimal %q"))
	w.Out()
	w.Line("}")
	w.Line("*d = Decimal(text)")
	w.Line("return nil")
	w.Out()
	w.Line("}")
	w.Line("")
	w.Line("// isJSONNumberLiteral reports whether value is a single JSON number literal.")
	w.Line("func isJSONNumberLiteral(value string) bool {")
	w.In()
	w.Line("i := 0")
	w.Line("if i < len(value) && value[i] == '-' {")
	w.In()
	w.Line("i++")
	w.Out()
	w.Line("}")
	w.Line("if i >= len(value) {")
	w.In()
	w.Line("return false")
	w.Out()
	w.Line("}")
	w.Line("switch {")
	w.Line("case value[i] == '0':")
	w.In()
	w.Line("i++")
	w.Out()
	w.Line("case value[i] >= '1' && value[i] <= '9':")
	w.In()
	w.Line("for i < len(value) && isASCIIDigit(value[i]) {")
	w.In()
	w.Line("i++")
	w.Out()
	w.Line("}")
	w.Out()
	w.Line("default:")
	w.In()
	w.Line("return false")
	w.Out()
	w.Line("}")
	w.Line("if i < len(value) && value[i] == '.' {")
	w.In()
	w.Line("i++")
	w.Line("if i >= len(value) || !isASCIIDigit(value[i]) {")
	w.In()
	w.Line("return false")
	w.Out()
	w.Line("}")
	w.Line("for i < len(value) && isASCIIDigit(value[i]) {")
	w.In()
	w.Line("i++")
	w.Out()
	w.Line("}")
	w.Out()
	w.Line("}")
	w.Line("if i < len(value) && (value[i] == 'e' || value[i] == 'E') {")
	w.In()
	w.Line("i++")
	w.Line("if i < len(value) && (value[i] == '+' || value[i] == '-') {")
	w.In()
	w.Line("i++")
	w.Out()
	w.Line("}")
	w.Line("if i >= len(value) || !isASCIIDigit(value[i]) {")
	w.In()
	w.Line("return false")
	w.Out()
	w.Line("}")
	w.Line("for i < len(value) && isASCIIDigit(value[i]) {")
	w.In()
	w.Line("i++")
	w.Out()
	w.Line("}")
	w.Out()
	w.Line("}")
	w.Line("return i == len(value)")
	w.Out()
	w.Line("}")
	w.Line("")
	w.Line("func isASCIIDigit(c byte) bool { return c >= '0' && c <= '9' }")
	return w.Render()
}

// emitRelaxedBoolFile writes types.RelaxedBool, the wire representation of a
// Boolean member whose input decoding is relaxed.
func (g *generator) emitRelaxedBoolFile() ([]byte, error) {
	w := newFileWriter("types", "RelaxedBool is the wire representation of a Boolean member whose input decoding is relaxed. "+
		"It accepts a JSON boolean or the quoted strings \"true\" and \"false\" and always encodes a bare boolean.")
	w.AddImport("bytes", "")
	w.AddImport("encoding/json", "")
	w.AddImport("fmt", "")

	w.Line("// RelaxedBool accepts true, false, \"true\", and \"false\" on input.")
	w.Line("type RelaxedBool bool")
	w.Line("")
	w.Line("// UnmarshalJSON accepts a JSON boolean or a quoted boolean string.")
	w.Line("func (b *RelaxedBool) UnmarshalJSON(data []byte) error {")
	w.In()
	w.Line("text := bytes.TrimSpace(data)")
	w.Line("if len(text) > 0 && text[0] == '\"' {")
	w.In()
	w.Line("var value string")
	w.Line("if err := json.Unmarshal(text, &value); err != nil {")
	w.In()
	w.Line("return err")
	w.Out()
	w.Line("}")
	w.Line("switch value {")
	w.Line("case \"true\":")
	w.In()
	w.Line("*b = true")
	w.Out()
	w.Line("case \"false\":")
	w.In()
	w.Line("*b = false")
	w.Out()
	w.Line("default:")
	w.In()
	w.Line("return fmt.Errorf(%s, value)", quoted("smithy: invalid boolean %q"))
	w.Out()
	w.Line("}")
	w.Line("return nil")
	w.Out()
	w.Line("}")
	w.Line("if bytes.Equal(text, []byte(\"null\")) {")
	w.In()
	w.Line("*b = false")
	w.Line("return nil")
	w.Out()
	w.Line("}")
	w.Line("var value bool")
	w.Line("if err := json.Unmarshal(text, &value); err != nil {")
	w.In()
	w.Line("return err")
	w.Out()
	w.Line("}")
	w.Line("*b = RelaxedBool(value)")
	w.Line("return nil")
	w.Out()
	w.Line("}")
	w.Line("")
	w.Line("// MarshalJSON always emits a bare JSON boolean.")
	w.Line("func (b RelaxedBool) MarshalJSON() ([]byte, error) {")
	w.In()
	w.Line("if b {")
	w.In()
	w.Line("return []byte(\"true\"), nil")
	w.Out()
	w.Line("}")
	w.Line("return []byte(\"false\"), nil")
	w.Out()
	w.Line("}")
	w.Line("")
	w.Line("// Value returns the boolean value.")
	w.Line("func (b RelaxedBool) Value() bool { return bool(b) }")
	return w.Render()
}

// decimalCmpSource is emitted into types/decimal.go. It keeps the raw literal
// as the wire representation and parses on demand for exact comparison.
const decimalCmpSource = `// Cmp compares d and other numerically. ok is false when either value is not
// a valid decimal literal.
func (d Decimal) Cmp(other Decimal) (int, bool) {
	an, ac, ae, aok := decimalParts(string(d))
	bn, bc, be, bok := decimalParts(string(other))
	if !aok || !bok {
		return 0, false
	}
	as := decimalSign(an, ac)
	bs := decimalSign(bn, bc)
	if as != bs {
		if as < bs {
			return -1, true
		}
		return 1, true
	}
	if as == 0 {
		return 0, true
	}
	// Compare the position of the most significant digit before scaling. This
	// keeps values such as 1e999999999 from allocating 10^999999999.
	aScale := new(big.Int).Add(ae, big.NewInt(int64(len(ac.String()))))
	bScale := new(big.Int).Add(be, big.NewInt(int64(len(bc.String()))))
	if scaleCmp := aScale.Cmp(bScale); scaleCmp != 0 {
		cmp := scaleCmp
		if as < 0 {
			cmp = -cmp
		}
		return cmp, true
	}
	// With equal scales the exponent difference is bounded by the coefficient
	// lengths, so this multiplication is proportional to the input size.
	diff := new(big.Int).Sub(ae, be)
	var cmp int
	switch diff.Sign() {
	case 1:
		cmp = new(big.Int).Mul(ac, decimalPow10(diff.Int64())).Cmp(bc)
	case -1:
		cmp = ac.Cmp(new(big.Int).Mul(bc, decimalPow10(new(big.Int).Neg(diff).Int64())))
	default:
		cmp = ac.Cmp(bc)
	}
	if as < 0 {
		cmp = -cmp
	}
	return cmp, true
}

func decimalSign(negative bool, coefficient *big.Int) int {
	if coefficient.Sign() == 0 {
		return 0
	}
	if negative {
		return -1
	}
	return 1
}

func decimalPow10(n int64) *big.Int {
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(n), nil)
}

// decimalParts parses a JSON number literal into a sign, a non-negative
// coefficient, and a base-10 exponent.
func decimalParts(value string) (negative bool, coefficient *big.Int, exponent *big.Int, ok bool) {
	exponent = new(big.Int)
	i := 0
	if i < len(value) && value[i] == '-' {
		negative = true
		i++
	}
	if i >= len(value) {
		return false, nil, nil, false
	}
	digits := make([]byte, 0, len(value))
	switch {
	case value[i] == '0':
		digits = append(digits, value[i])
		i++
	case value[i] >= '1' && value[i] <= '9':
		for i < len(value) && isASCIIDigit(value[i]) {
			digits = append(digits, value[i])
			i++
		}
	default:
		return false, nil, nil, false
	}
	fractionDigits := 0
	if i < len(value) && value[i] == '.' {
		i++
		if i >= len(value) || !isASCIIDigit(value[i]) {
			return false, nil, nil, false
		}
		for i < len(value) && isASCIIDigit(value[i]) {
			digits = append(digits, value[i])
			fractionDigits++
			i++
		}
	}
	if i < len(value) && (value[i] == 'e' || value[i] == 'E') {
		i++
		exponentNegative := false
		if i < len(value) && (value[i] == '+' || value[i] == '-') {
			exponentNegative = value[i] == '-'
			i++
		}
		start := i
		for i < len(value) && isASCIIDigit(value[i]) {
			i++
		}
		if start == i {
			return false, nil, nil, false
		}
		parsed, ok := new(big.Int).SetString(value[start:i], 10)
		if !ok {
			return false, nil, nil, false
		}
		if exponentNegative {
			parsed.Neg(parsed)
		}
		exponent = parsed
	}
	if i != len(value) {
		return false, nil, nil, false
	}
	if fractionDigits > 0 {
		exponent.Sub(exponent, big.NewInt(int64(fractionDigits)))
	}
	if len(digits) <= 18 {
		parsed, err := strconv.ParseUint(string(digits), 10, 64)
		if err != nil {
			return false, nil, nil, false
		}
		coefficient = new(big.Int).SetUint64(parsed)
	} else {
		coefficient = new(big.Int)
		if _, ok := coefficient.SetString(string(digits), 10); !ok {
			return false, nil, nil, false
		}
	}
	return negative, coefficient, exponent, true
}
`
