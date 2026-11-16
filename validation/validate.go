package validation

import (
	"fmt"
	"math/big"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/tespeleta/gosmithy/idl"
	"github.com/tespeleta/gosmithy/model"
)

// Event IDs emitted by gosmithy. They intentionally mirror the names suggested
// by the milestone plan.
const (
	ModelDuplicateShapeId    = "Model.DuplicateShapeId"
	ModelUnresolvedShape     = "Model.UnresolvedShape"
	ModelInvalidMemberTarget = "Model.InvalidMemberTarget"
	ModelInvalidMember       = "Model.InvalidMember"
	ModelUnresolvedTrait     = "Model.UnresolvedTrait"
	ModelNotATrait           = "Model.NotATrait"
	ModelInvalidTraitValue   = "Model.InvalidTraitValue"
	ModelInvalidOperationIo  = "Model.InvalidOperationIo"
	ModelInvalidOperationErr = "Model.InvalidOperationError"
	ModelInvalidReference    = "Model.InvalidReference"
	ModelInvalidResource     = "Model.InvalidResource"
	ModelInvalidMixin        = "Model.InvalidMixin"
	ModelInvalidEnum         = "Model.InvalidEnum"
	ModelInvalidTraitTarget  = "Model.InvalidTraitTarget"
	ModelInvalidShapeType    = "Model.InvalidShapeType"
	ModelConflict            = "Model.Conflict"
	ModelMetadataConflict    = "Model.MetadataConflict"
	SyntacticShapeIdTarget   = "SyntacticShapeIdTarget"
)

// relaxedInputTrait is the gosmithy.protocols#relaxedInput trait ID. The
// protocol package owns the definition; validation only needs the ID for the
// target checks gosmithy adds on top of the trait's selector.
var relaxedInputTrait = model.MustShapeID("gosmithy.protocols#relaxedInput")

// Validate runs every structural validator over the model and returns all
// events, including diagnostics produced during assembly, sorted
// deterministically by source location.
func Validate(m *model.Model) []Event {
	events := m.Diagnostics()
	events = append(events, validateTargets(m)...)
	events = append(events, validateOperations(m)...)
	events = append(events, validateReferences(m)...)
	events = append(events, validateMixins(m)...)
	events = append(events, validateEnums(m)...)
	events = append(events, validateTraitValues(m)...)
	events = append(events, validateRangeBounds(m)...)
	events = append(events, validateJSONNames(m)...)
	events = append(events, validateTraitConstraints(m)...)
	events = append(events, validateTraitTargets(m)...)
	events = append(events, validateRelaxedInputTargets(m)...)
	events = append(events, validateSyntheticShapes(m)...)
	events = append(events, validateSyntacticShapeIDs(m)...)
	model.SortDiagnostics(events)
	return events
}

// --- targets ------------------------------------------------------------------

func validateTargets(m *model.Model) []Event {
	var events []Event
	for _, s := range m.Shapes() {
		if s.IsMember() {
			if s.Target.IsZero() {
				continue
			}
			target, ok := m.Shape(s.Target)
			if !ok {
				events = append(events, makeEvent(ModelUnresolvedShape, s.Source, s.ID,
					"Target `%s` is not found in the model", s.Target))
				continue
			}
			if reason := illegalMemberTarget(target); reason != "" {
				events = append(events, makeEvent(ModelInvalidMemberTarget, s.Source, s.ID,
					"Member `%s` targets `%s`, which %s", s.ID, target.ID, reason))
			}
			continue
		}

		switch s.Type {
		case model.TypeList, model.TypeSet:
			if _, ok := s.GetMember("member"); !ok {
				events = append(events, makeEvent(ModelInvalidMember, s.Source, s.ID,
					"Missing required member of shape `%s`: member", s.ID))
			}
		case model.TypeMap:
			key, hasKey := s.GetMember("key")
			_, hasValue := s.GetMember("value")
			switch {
			case !hasKey && !hasValue:
				events = append(events, makeEvent(ModelInvalidMember, s.Source, s.ID,
					"Missing required members of shape `%s`: key, value", s.ID))
			case !hasKey:
				events = append(events, makeEvent(ModelInvalidMember, s.Source, s.ID,
					"Missing required member of shape `%s`: key", s.ID))
			case !hasValue:
				events = append(events, makeEvent(ModelInvalidMember, s.Source, s.ID,
					"Missing required member of shape `%s`: value", s.ID))
			}
			if hasKey {
				if target, ok := m.Shape(key.Target); ok && target.Type != model.TypeString && target.Type != model.TypeEnum {
					events = append(events, makeEvent(ModelInvalidMemberTarget, key.Source, s.ID,
						"Map key member `%s` targets `%s`, but map keys must target string shapes", key.ID, target.ID))
				}
			}
		case model.TypeStructure, model.TypeUnion, model.TypeEnum, model.TypeIntEnum:
			seen := make(map[string]*model.Shape)
			for _, member := range s.Members {
				name := member.ID.Member
				lower := strings.ToLower(name)
				if previous, ok := seen[lower]; ok {
					events = append(events, makeEvent(ModelInvalidMember, member.Source, member.ID,
						"Member name `%s` conflicts with `%s` (member names are case-insensitive)", name, previous.ID.Member))
					continue
				}
				seen[lower] = member
			}
		}
	}
	return events
}

func illegalMemberTarget(target *model.Shape) string {
	switch target.Type {
	case model.TypeOperation:
		return "is an operation; operations cannot be used as member targets"
	case model.TypeResource:
		return "is a resource; resources cannot be used as member targets"
	case model.TypeService:
		return "is a service; services cannot be used as member targets"
	case model.TypeMember:
		return "is a member; member shapes cannot be used as member targets"
	}
	if target.IsTraitDefinition() {
		return "is a trait definition; traits cannot be used as member targets"
	}
	if target.IsMixin() {
		return "is a mixin; mixins cannot be used as member targets"
	}
	return ""
}

// --- operations ---------------------------------------------------------------

func validateOperations(m *model.Model) []Event {
	var events []Event
	for _, s := range m.Shapes() {
		if s.Type != model.TypeOperation {
			continue
		}
		events = append(events, validateOperationIO(m, s, "input", s.Input)...)
		events = append(events, validateOperationIO(m, s, "output", s.Output)...)
		for _, errID := range s.Errors {
			shape, ok := m.Shape(errID)
			if !ok {
				events = append(events, makeEvent(ModelUnresolvedShape, s.Source, s.ID,
					"Operation `%s` error `%s` is not found in the model", s.ID, errID))
				continue
			}
			if shape.IsMixin() {
				events = append(events, makeEvent(ModelInvalidOperationErr, s.Source, s.ID,
					"Operation `%s` error `%s` must not target a mixin", s.ID, shape.ID))
				continue
			}
			if !shape.IsErrorStructure() {
				events = append(events, makeEvent(ModelInvalidOperationErr, s.Source, s.ID,
					"Operation `%s` error `%s` must target a structure with the @error trait", s.ID, shape.ID))
			}
		}
	}
	return events
}

func validateOperationIO(m *model.Model, s *model.Shape, kind string, id model.ShapeID) []Event {
	if id.IsZero() {
		return []Event{makeEvent(ModelInvalidOperationIo, s.Source, s.ID,
			"Operation `%s` %s is not set", s.ID, kind)}
	}
	shape, ok := m.Shape(id)
	if !ok {
		return []Event{makeEvent(ModelUnresolvedShape, s.Source, s.ID,
			"Operation `%s` %s `%s` is not found in the model", s.ID, kind, id)}
	}
	if shape.IsMixin() {
		return []Event{makeEvent(ModelInvalidOperationIo, s.Source, s.ID,
			"Operation `%s` %s must not target the mixin `%s`", s.ID, kind, shape.ID)}
	}
	if shape.Type != model.TypeStructure && !shape.IsUnit() {
		return []Event{makeEvent(ModelInvalidOperationIo, s.Source, s.ID,
			"Operation `%s` %s `%s` must target a structure or smithy.api#Unit, but found a %s shape",
			s.ID, kind, shape.ID, shape.Type)}
	}
	return nil
}

// --- service and resource references -----------------------------------------

func validateReferences(m *model.Model) []Event {
	var events []Event
	for _, s := range m.Shapes() {
		switch s.Type {
		case model.TypeService:
			if s.Service == nil {
				continue
			}
			for _, id := range s.Service.Operations {
				events = append(events, validateRef(m, s, "operation", id, model.TypeOperation, ModelInvalidReference)...)
			}
			for _, id := range s.Service.Resources {
				events = append(events, validateRef(m, s, "resource", id, model.TypeResource, ModelInvalidReference)...)
			}
			for _, id := range s.Service.Errors {
				events = append(events, validateServiceError(m, s, id)...)
			}
		case model.TypeResource:
			if s.Resource == nil {
				continue
			}
			r := s.Resource
			for _, entry := range []struct {
				kind string
				id   model.ShapeID
			}{
				{"create", r.Create}, {"read", r.Read}, {"update", r.Update},
				{"delete", r.Delete}, {"list", r.List}, {"put", r.Put},
			} {
				if !entry.id.IsZero() {
					events = append(events, validateRef(m, s, entry.kind, entry.id, model.TypeOperation, ModelInvalidResource)...)
				}
			}
			for _, id := range r.Operations {
				events = append(events, validateRef(m, s, "operation", id, model.TypeOperation, ModelInvalidResource)...)
			}
			for _, id := range r.CollectionOperations {
				events = append(events, validateRef(m, s, "collection operation", id, model.TypeOperation, ModelInvalidResource)...)
			}
			for _, id := range r.Resources {
				events = append(events, validateRef(m, s, "resource", id, model.TypeResource, ModelInvalidResource)...)
			}
			for _, name := range sortedKeys(r.Identifiers) {
				events = append(events, validateResourceProperty(m, s, "identifier", name, r.Identifiers[name])...)
			}
			for _, name := range sortedKeys(r.Properties) {
				events = append(events, validateResourceProperty(m, s, "property", name, r.Properties[name])...)
			}
		}
		if !s.ForResource.IsZero() {
			resource, ok := m.Shape(s.ForResource)
			if !ok {
				events = append(events, makeEvent(ModelUnresolvedShape, s.Source, s.ID,
					"Target `%s` of the `for` production is not found in the model", s.ForResource))
			} else if resource.Type != model.TypeResource {
				events = append(events, makeEvent(ModelInvalidReference, s.Source, s.ID,
					"The target of the `for` production must be a resource shape, but found a %s shape: %s",
					resource.Type, resource.ID))
			}
		}
	}
	return events
}

func validateRef(m *model.Model, owner *model.Shape, kind string, id model.ShapeID, want model.ShapeType, eventID string) []Event {
	if id.IsZero() {
		return nil
	}
	shape, ok := m.Shape(id)
	if !ok {
		return []Event{makeEvent(ModelUnresolvedShape, owner.Source, owner.ID,
			"%s `%s` references `%s`, which is not found in the model", capitalize(kind), owner.ID, id)}
	}
	if shape.Type != want {
		return []Event{makeEvent(eventID, owner.Source, owner.ID,
			"%s `%s` %s `%s` must target a %s shape, but found a %s shape",
			capitalize(kind), owner.ID, kind, shape.ID, want, shape.Type)}
	}
	return nil
}

func validateServiceError(m *model.Model, service *model.Shape, id model.ShapeID) []Event {
	if id.IsZero() {
		return nil
	}
	shape, ok := m.Shape(id)
	if !ok {
		return []Event{makeEvent(ModelUnresolvedShape, service.Source, service.ID,
			"Service `%s` error `%s` is not found in the model", service.ID, id)}
	}
	if !shape.IsErrorStructure() {
		return []Event{makeEvent(ModelInvalidOperationErr, service.Source, service.ID,
			"Service `%s` error `%s` must target a structure with the @error trait", service.ID, shape.ID)}
	}
	return nil
}

func validateResourceProperty(m *model.Model, resource *model.Shape, kind, name string, id model.ShapeID) []Event {
	if id.IsZero() {
		return nil
	}
	target, ok := m.Shape(id)
	if !ok {
		return []Event{makeEvent(ModelUnresolvedShape, resource.Source, resource.ID,
			"Resource `%s` %s `%s` targets `%s`, which is not found in the model", resource.ID, kind, name, id)}
	}
	if reason := illegalMemberTarget(target); reason != "" {
		return []Event{makeEvent(ModelInvalidResource, resource.Source, resource.ID,
			"Resource `%s` %s `%s` targets `%s`, which %s", resource.ID, kind, name, target.ID, reason)}
	}
	if kind == "identifier" {
		switch target.Type {
		case model.TypeString, model.TypeEnum:
		default:
			return []Event{makeEvent(ModelInvalidResource, resource.Source, resource.ID,
				"Resource `%s` identifier `%s` targets `%s`, but resource identifiers must target a string or enum shape",
				resource.ID, name, target.ID)}
		}
	}
	return nil
}

// --- mixins -------------------------------------------------------------------

func validateMixins(m *model.Model) []Event {
	var events []Event
	for _, s := range m.Shapes() {
		if s.IsMember() {
			continue
		}
		if len(s.Mixins) > 0 {
			var invalid []string
			for _, id := range s.Mixins {
				mixin, ok := m.Shape(id)
				if !ok {
					events = append(events, makeEvent(ModelUnresolvedShape, s.Source, s.ID,
						"Mixin `%s` is not found in the model", id))
					continue
				}
				if mixin.Type != s.Type {
					invalid = append(invalid, fmt.Sprintf("`%s`", mixin.ID))
					continue
				}
				if !mixin.IsMixin() {
					events = append(events, makeEvent(ModelInvalidMixin, s.Source, s.ID,
						"Shape `%s` is not a mixin, but it is used as a mixin by `%s`", mixin.ID, s.ID))
				}
			}
			if len(invalid) > 0 {
				events = append(events, makeEvent(ModelInvalidMixin, s.Source, s.ID,
					"Mixins may only be mixed into shapes of the same type. The following mixins were applied to the %s shape `%s` which are not %s shapes: [%s]",
					s.Type, s.ID, s.Type, strings.Join(invalid, ", ")))
			}
		}

		if s.IsMixin() {
			switch s.Type {
			case model.TypeResource:
				if resourceHasProperties(s.Resource) {
					events = append(events, makeEvent(ModelInvalidMixin, s.Source, s.ID,
						"Resource shapes with the mixin trait may not define any properties. Resource mixin shape `%s` defines one or more properties.", s.ID))
				}
			case model.TypeOperation:
				if (!s.Input.IsZero() && s.Input != model.TraitUnit) || (!s.Output.IsZero() && s.Output != model.TraitUnit) {
					events = append(events, makeEvent(ModelInvalidMixin, s.Source, s.ID,
						"Operation shapes with the mixin trait MUST target `%s` for their input and output. Operation mixin shape `%s` defines one or both of these properties.", model.TraitUnit, s.ID))
				}
			}
		}
	}
	events = append(events, validateMixinCycles(m)...)
	return events
}

func validateMixinCycles(m *model.Model) []Event {
	state := make(map[model.ShapeID]int) // 0 unvisited, 1 visiting, 2 done
	var events []Event
	reported := make(map[model.ShapeID]bool)
	var visit func(s *model.Shape)
	visit = func(s *model.Shape) {
		switch state[s.ID] {
		case 1:
			if !reported[s.ID] {
				reported[s.ID] = true
				events = append(events, makeEvent(ModelInvalidMixin, s.Source, s.ID,
					"Mixin cycle detected: `%s` is mixed into itself directly or transitively", s.ID))
			}
			return
		case 2:
			return
		}
		state[s.ID] = 1
		for _, id := range s.Mixins {
			if mixin, ok := m.Shape(id); ok {
				visit(mixin)
			}
		}
		state[s.ID] = 2
	}
	for _, s := range m.Shapes() {
		if !s.IsMember() {
			visit(s)
		}
	}
	return events
}

func resourceHasProperties(r *model.ResourceProps) bool {
	if r == nil {
		return false
	}
	return len(r.Identifiers) > 0 || len(r.Properties) > 0 ||
		!r.Create.IsZero() || !r.Read.IsZero() || !r.Update.IsZero() ||
		!r.Delete.IsZero() || !r.List.IsZero() || !r.Put.IsZero() ||
		len(r.Operations) > 0 || len(r.CollectionOperations) > 0 || len(r.Resources) > 0
}

// --- enums --------------------------------------------------------------------

func validateEnums(m *model.Model) []Event {
	var events []Event
	for _, s := range m.Shapes() {
		switch s.Type {
		case model.TypeEnum:
			if len(s.Members) == 0 {
				events = append(events, makeEvent(ModelInvalidEnum, s.Source, s.ID,
					"enum must have at least one entry"))
				continue
			}
			seen := make(map[string]model.ShapeID)
			for _, member := range s.Members {
				value, ok := member.EnumValue()
				if !ok {
					events = append(events, makeEvent(ModelInvalidEnum, member.Source, member.ID,
						"enum members must have an enumValue trait"))
					continue
				}
				text, ok := value.AsString()
				if !ok {
					events = append(events, makeEvent(ModelInvalidEnum, member.Source, member.ID,
						"enum members can only be assigned string values, but found: %s", value.String()))
					continue
				}
				if previous, ok := seen[text]; ok {
					events = append(events, makeEvent(ModelInvalidEnum, member.Source, member.ID,
						"enum value %q conflicts with `%s`", text, previous))
					continue
				}
				seen[text] = member.ID
			}
		case model.TypeIntEnum:
			if len(s.Members) == 0 {
				events = append(events, makeEvent(ModelInvalidEnum, s.Source, s.ID,
					"intEnum shapes must have at least one member"))
				continue
			}
			seen := make(map[string]model.ShapeID)
			for _, member := range s.Members {
				value, ok := member.EnumValue()
				if !ok {
					events = append(events, makeEvent(ModelInvalidEnum, member.Source, member.ID,
						"intEnum members must have an @enumValue trait"))
					continue
				}
				if value.Kind != model.NodeNumber {
					events = append(events, makeEvent(ModelInvalidEnum, member.Source, member.ID,
						"intEnum members require integer values, but found: %s", value.String()))
					continue
				}
				if !isIntegerLexeme(value.Num) {
					events = append(events, makeEvent(ModelInvalidEnum, member.Source, member.ID,
						"intEnum members do not support floating point values"))
					continue
				}
				if !fitsInt32(value.Num) {
					events = append(events, makeEvent(ModelInvalidEnum, member.Source, member.ID,
						"intEnum members must fit within an integer, but found: %s", value.Num))
					continue
				}
				if previous, ok := seen[value.Num]; ok {
					events = append(events, makeEvent(ModelInvalidEnum, member.Source, member.ID,
						"intEnum value %s conflicts with `%s`", value.Num, previous))
					continue
				}
				seen[value.Num] = member.ID
			}
		}
	}
	return events
}

// --- trait values and targets -------------------------------------------------

func validateTraitValues(m *model.Model) []Event {
	var events []Event
	for _, s := range m.Shapes() {
		for _, traitID := range s.Traits.SortedIDs() {
			trait := s.Traits[traitID]
			def, ok := m.Shape(traitID)
			if !ok || !def.IsTraitDefinition() {
				continue
			}
			if msg := checkTraitValue(m, def, trait.Value, 0); msg != "" {
				events = append(events, makeEvent(ModelInvalidTraitValue, trait.Source, s.ID,
					"Trait `%s` has an invalid value: %s", traitID, msg))
			}
		}
	}
	return events
}

func checkTraitValue(m *model.Model, def *model.Shape, value model.Node, depth int) string {
	if depth > 32 {
		return "trait value is nested too deeply"
	}
	if def.Type == model.TypeDocument {
		return ""
	}
	if value.Kind == model.NodeNull {
		return fmt.Sprintf("expected a value of type %s, but found null", def.Type)
	}
	switch def.Type {
	case model.TypeStructure, model.TypeUnion:
		if value.Kind != model.NodeObject {
			return fmt.Sprintf("expected an object value, but found %s", value.Kind)
		}
		if def.Type == model.TypeUnion && len(value.Obj) != 1 {
			return "expected an object with exactly one member for a union trait"
		}
		for _, entry := range value.Obj {
			member, ok := def.GetMember(entry.Key)
			if !ok {
				return fmt.Sprintf("unknown member `%s` of trait structure `%s`", entry.Key, def.ID)
			}
			target, ok := m.Shape(member.Target)
			if !ok {
				continue
			}
			if msg := checkTraitValue(m, target, entry.Value, depth+1); msg != "" {
				return fmt.Sprintf("member `%s`: %s", entry.Key, msg)
			}
		}
		for _, member := range def.Members {
			if !member.Required() {
				continue
			}
			if _, ok := value.Get(member.ID.Member); !ok {
				return fmt.Sprintf("missing required member `%s` of trait structure `%s`", member.ID.Member, def.ID)
			}
		}
		return ""
	case model.TypeList, model.TypeSet:
		if value.Kind != model.NodeArray {
			return fmt.Sprintf("expected an array value, but found %s", value.Kind)
		}
		member, ok := def.GetMember("member")
		if !ok {
			return ""
		}
		target, ok := m.Shape(member.Target)
		if !ok {
			return ""
		}
		for i, item := range value.Arr {
			if msg := checkTraitValue(m, target, item, depth+1); msg != "" {
				return fmt.Sprintf("element %d: %s", i, msg)
			}
		}
		return ""
	case model.TypeMap:
		if value.Kind != model.NodeObject {
			return fmt.Sprintf("expected an object value, but found %s", value.Kind)
		}
		member, ok := def.GetMember("value")
		if !ok {
			return ""
		}
		target, ok := m.Shape(member.Target)
		if !ok {
			return ""
		}
		for _, entry := range value.Obj {
			if msg := checkTraitValue(m, target, entry.Value, depth+1); msg != "" {
				return fmt.Sprintf("value for key `%s`: %s", entry.Key, msg)
			}
		}
		return ""
	case model.TypeString, model.TypeBlob, model.TypeTimestamp:
		if value.Kind != model.NodeString {
			return fmt.Sprintf("expected a string value, but found %s", value.Kind)
		}
		return ""
	case model.TypeEnum:
		if value.Kind != model.NodeString {
			return fmt.Sprintf("expected a string enum value, but found %s", value.Kind)
		}
		allowed := enumValues(m, def)
		if len(allowed) > 0 && !allowed[value.Str] {
			return fmt.Sprintf("%q is not a valid value of enum `%s`", value.Str, def.ID)
		}
		return ""
	case model.TypeIntEnum:
		if value.Kind != model.NodeNumber || !isIntegerLexeme(value.Num) {
			return fmt.Sprintf("expected an integer enum value, but found %s", value.String())
		}
		return ""
	case model.TypeBoolean:
		if value.Kind != model.NodeBoolean {
			return fmt.Sprintf("expected a boolean value, but found %s", value.Kind)
		}
		return ""
	case model.TypeByte, model.TypeShort, model.TypeInteger, model.TypeLong:
		if value.Kind != model.NodeNumber || !isIntegerLexeme(value.Num) {
			return fmt.Sprintf("expected an integer value, but found %s", value.String())
		}
		if !fitsInteger(value.Num, def.Type) {
			return fmt.Sprintf("integer value %s is out of range for %s", value.Num, def.Type)
		}
		return ""
	case model.TypeBigInteger:
		if value.Kind != model.NodeNumber || !isIntegerLexeme(value.Num) {
			return fmt.Sprintf("expected an integer value, but found %s", value.String())
		}
		return ""
	case model.TypeFloat, model.TypeDouble, model.TypeBigDecimal:
		if value.Kind != model.NodeNumber {
			return fmt.Sprintf("expected a numeric value, but found %s", value.Kind)
		}
		return ""
	case model.TypeMember:
		return ""
	default:
		return fmt.Sprintf("unsupported trait shape type %s", def.Type)
	}
}

// validateTraitConstraints applies a trait definition's own @range, @length,
// and @pattern constraints to the values supplied when the trait is applied.
// This catches cases such as @http(code: 42), where the code member of the
// http trait definition has @range(min: 100, max: 999).
func validateTraitConstraints(m *model.Model) []Event {
	var events []Event
	for _, s := range m.Shapes() {
		for _, traitID := range s.Traits.SortedIDs() {
			trait := s.Traits[traitID]
			def, ok := m.Shape(traitID)
			if !ok || !def.IsTraitDefinition() {
				continue
			}
			if msg := checkConstraintValue(m, def, trait.Value, ""); msg != "" {
				events = append(events, makeEvent(ModelInvalidTraitValue, trait.Source, s.ID,
					"Trait `%s` has an invalid value: %s", traitID, msg))
			}
		}
	}
	return events
}

func checkConstraintValue(m *model.Model, shape *model.Shape, value model.Node, path string) string {
	if shape == nil || value.Kind == model.NodeNull {
		return ""
	}
	if shape.IsMember() {
		target, ok := m.Shape(shape.Target)
		if !ok {
			return ""
		}
		if msg := checkEffectiveConstraints(shape, target, value, path); msg != "" {
			return msg
		}
		return walkConstraintValue(m, target, value, path, false)
	}
	if msg := checkShapeConstraints(shape, value, path); msg != "" {
		return msg
	}
	return walkConstraintValue(m, shape, value, path, false)
}

func walkConstraintValue(m *model.Model, shape *model.Shape, value model.Node, path string, checkOwn bool) string {
	if shape == nil {
		return ""
	}
	if checkOwn {
		if msg := checkShapeConstraints(shape, value, path); msg != "" {
			return msg
		}
	}
	switch shape.Type {
	case model.TypeStructure, model.TypeUnion:
		if value.Kind != model.NodeObject {
			return ""
		}
		for _, entry := range value.Obj {
			member, ok := shape.GetMember(entry.Key)
			if !ok {
				continue
			}
			if msg := checkConstraintValue(m, member, entry.Value, constraintPath(path, "member `"+entry.Key+"`")); msg != "" {
				return msg
			}
		}
	case model.TypeList, model.TypeSet:
		if value.Kind != model.NodeArray {
			return ""
		}
		member, ok := shape.GetMember("member")
		if !ok {
			return ""
		}
		for i, item := range value.Arr {
			if msg := checkConstraintValue(m, member, item, constraintPath(path, fmt.Sprintf("element %d", i))); msg != "" {
				return msg
			}
		}
	case model.TypeMap:
		if value.Kind != model.NodeObject {
			return ""
		}
		member, ok := shape.GetMember("value")
		if !ok {
			return ""
		}
		for _, entry := range value.Obj {
			if msg := checkConstraintValue(m, member, entry.Value, constraintPath(path, fmt.Sprintf("value %q", entry.Key))); msg != "" {
				return msg
			}
		}
	}
	return ""
}

func checkEffectiveConstraints(member, target *model.Shape, value model.Node, path string) string {
	if member.HasTrait(model.TraitRange) {
		if msg := checkRangeConstraint(member, value, path); msg != "" {
			return msg
		}
	} else if target != nil && target.HasTrait(model.TraitRange) {
		if msg := checkRangeConstraint(target, value, path); msg != "" {
			return msg
		}
	}
	if member.HasTrait(model.TraitLength) {
		if msg := checkLengthConstraint(member, value, path); msg != "" {
			return msg
		}
	} else if target != nil && target.HasTrait(model.TraitLength) {
		if msg := checkLengthConstraint(target, value, path); msg != "" {
			return msg
		}
	}
	if member.HasTrait(model.TraitPattern) {
		if msg := checkPatternConstraint(member, value, path); msg != "" {
			return msg
		}
	} else if target != nil && target.HasTrait(model.TraitPattern) {
		if msg := checkPatternConstraint(target, value, path); msg != "" {
			return msg
		}
	}
	return ""
}

func checkShapeConstraints(shape *model.Shape, value model.Node, path string) string {
	if msg := checkRangeConstraint(shape, value, path); msg != "" {
		return msg
	}
	if msg := checkLengthConstraint(shape, value, path); msg != "" {
		return msg
	}
	if msg := checkPatternConstraint(shape, value, path); msg != "" {
		return msg
	}
	return ""
}

func checkRangeConstraint(shape *model.Shape, value model.Node, path string) string {
	if value.Kind != model.NodeNumber {
		return ""
	}
	min, max, ok := shape.RangeBounds()
	if !ok {
		return ""
	}
	v, ok := numberRat(value.Num)
	if !ok {
		return ""
	}
	if min != nil {
		if bound, ok := numberRat(*min); ok && v.Cmp(bound) < 0 {
			return fmt.Sprintf("%svalue %s is less than the minimum %s", path, value.Num, *min)
		}
	}
	if max != nil {
		if bound, ok := numberRat(*max); ok && v.Cmp(bound) > 0 {
			return fmt.Sprintf("%svalue %s is greater than the maximum %s", path, value.Num, *max)
		}
	}
	return ""
}

// numberRat parses a Smithy number lexeme exactly. It returns ok=false for
// malformed numbers; callers skip checks rather than reporting a second time.
func numberRat(text string) (*big.Rat, bool) {
	return new(big.Rat).SetString(text)
}

func checkLengthConstraint(shape *model.Shape, value model.Node, path string) string {
	min, max, ok := shape.Length()
	if !ok {
		return ""
	}
	var n int
	switch value.Kind {
	case model.NodeString:
		n = utf8.RuneCountInString(value.Str)
	case model.NodeArray:
		n = len(value.Arr)
	case model.NodeObject:
		n = len(value.Obj)
	default:
		return ""
	}
	if min != nil && n < *min {
		return fmt.Sprintf("%svalue has length %d, less than the minimum %d", path, n, *min)
	}
	if max != nil && n > *max {
		return fmt.Sprintf("%svalue has length %d, greater than the maximum %d", path, n, *max)
	}
	return ""
}

func checkPatternConstraint(shape *model.Shape, value model.Node, path string) string {
	if value.Kind != model.NodeString {
		return ""
	}
	pattern, ok := shape.Pattern()
	if !ok {
		return ""
	}
	matched, err := regexp.MatchString(pattern, value.Str)
	if err != nil {
		return ""
	}
	if !matched {
		return fmt.Sprintf("%svalue %q does not match pattern %q", path, value.Str, pattern)
	}
	return ""
}

func constraintPath(prefix, segment string) string {
	if prefix == "" {
		return segment + ": "
	}
	return prefix + "." + segment + ": "
}

// validateJSONNames rejects two members of one structure or union that resolve
// to the same effective JSON object key.
func validateJSONNames(m *model.Model) []Event {
	var events []Event
	for _, s := range m.Shapes() {
		switch s.Type {
		case model.TypeStructure, model.TypeUnion:
		default:
			continue
		}
		seen := make(map[string]*model.Shape)
		for _, member := range s.Members {
			name := member.ID.Member
			if jsonName, ok := member.JSONName(); ok && jsonName != "" {
				name = jsonName
			}
			if previous, ok := seen[name]; ok {
				events = append(events, makeEvent(ModelInvalidMember, member.Source, member.ID,
					"Members `%s` and `%s` of `%s` use the same @jsonName %q",
					previous.ID, member.ID, s.ID, name))
				continue
			}
			seen[name] = member
		}
	}
	return events
}

// validateRangeBounds rejects fractional @range bounds on shapes whose Go/model
// representation is integral. Upstream Smithy reports these as invalid trait
// values; the codegen range comparisons otherwise depend on the bound being
// representable.
func validateRangeBounds(m *model.Model) []Event {
	var events []Event
	for _, s := range m.Shapes() {
		if !s.HasTrait(model.TraitRange) {
			continue
		}
		target := s
		if s.IsMember() {
			resolved, ok := m.Shape(s.Target)
			if !ok {
				continue
			}
			target = resolved
		}
		min, max, ok := s.RangeBounds()
		if !ok {
			continue
		}
		switch target.Type {
		case model.TypeFloat, model.TypeDouble, model.TypeBigDecimal:
			// These shapes support fractional bounds; ordering is still checked.
		default:
			if min != nil {
				if bound, ok := numberRat(*min); ok && !bound.IsInt() {
					events = append(events, makeEvent(ModelInvalidTraitValue, s.Source, s.ID,
						"@range on `%s` has a fractional `min` value %s, but `%s` does not support decimals",
						s.ID, *min, target.ID))
				}
			}
			if max != nil {
				if bound, ok := numberRat(*max); ok && !bound.IsInt() {
					events = append(events, makeEvent(ModelInvalidTraitValue, s.Source, s.ID,
						"@range on `%s` has a fractional `max` value %s, but `%s` does not support decimals",
						s.ID, *max, target.ID))
				}
			}
		}

		if min == nil || max == nil {
			continue
		}
		minBound, minOK := numberRat(*min)
		maxBound, maxOK := numberRat(*max)
		if minOK && maxOK && minBound.Cmp(maxBound) > 0 {
			events = append(events, makeEvent(ModelInvalidTraitValue, s.Source, s.ID,
				"@range on `%s` has a `min` value greater than its `max` value", s.ID))
		}
	}
	return events
}

func enumValues(m *model.Model, enum *model.Shape) map[string]bool {
	out := make(map[string]bool, len(enum.Members))
	for _, member := range enum.Members {
		if value, ok := member.EnumValue(); ok {
			if str, ok := value.AsString(); ok {
				out[str] = true
			}
		}
	}
	return out
}

// validateRelaxedInputTargets checks the targets of the gosmithy relaxed-input
// trait. Smithy selectors are not generally enforced, so this validator adds
// the checks the Go generator relies on: the trait applies to a service or to
// a scalar member of a structure or union.
func validateRelaxedInputTargets(m *model.Model) []Event {
	var events []Event
	for _, s := range m.Shapes() {
		if !s.HasTrait(relaxedInputTrait) {
			continue
		}
		if s.Type == model.TypeService {
			continue
		}
		if !s.IsMember() {
			events = append(events, makeEvent(ModelInvalidTraitTarget, s.Source, s.ID,
				"Trait `%s` can only be applied to a service or a member", relaxedInputTrait))
			continue
		}
		container, ok := m.Shape(s.Container)
		if !ok || (container.Type != model.TypeStructure && container.Type != model.TypeUnion) {
			events = append(events, makeEvent(ModelInvalidTraitTarget, s.Source, s.ID,
				"Trait `%s` can only be applied to structure or union members", relaxedInputTrait))
			continue
		}
		target, ok := m.Shape(s.Target)
		if !ok || !isRelaxableScalar(target.Type) {
			events = append(events, makeEvent(ModelInvalidTraitTarget, s.Source, s.ID,
				"Trait `%s` can only be applied to boolean, numeric, or bigDecimal members", relaxedInputTrait))
		}
	}
	return events
}

func isRelaxableScalar(t model.ShapeType) bool {
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

func validateTraitTargets(m *model.Model) []Event {
	var events []Event
	for _, s := range m.Shapes() {
		for _, traitID := range s.Traits.SortedIDs() {
			trait := s.Traits[traitID]
			reason := ""
			switch traitID {
			case model.TraitRequired:
				if !s.IsMember() {
					reason = "can only be applied to members"
				}
			case model.TraitInput, model.TraitOutput:
				if s.Type != model.TypeStructure {
					reason = "can only be applied to structures"
				}
			case model.TraitError:
				if s.Type != model.TypeStructure {
					reason = "can only be applied to structures"
				}
			case model.TraitMixin:
				if s.IsMember() {
					reason = "cannot be applied to members"
				}
			case model.TraitEnumValue:
				if !s.IsMember() {
					reason = "can only be applied to members"
				} else if container, ok := m.Shape(s.Container); !ok || (container.Type != model.TypeEnum && container.Type != model.TypeIntEnum) {
					reason = "can only be applied to members of enum or intEnum shapes"
				}
			case model.TraitTrait:
				switch s.Type {
				case model.TypeOperation, model.TypeResource, model.TypeService, model.TypeMember:
					reason = "can only be applied to simple or aggregate shapes"
				}
			}
			if reason != "" {
				events = append(events, makeEvent(ModelInvalidTraitTarget, trait.Source, s.ID,
					"Trait `%s` %s", traitID, reason))
			}
		}
	}
	return events
}

// --- synthetic shapes ---------------------------------------------------------

// validateSyntheticShapes enforces the 2.1 rule that traits cannot be applied
// to assembler-generated shapes, and warns when a user-defined shape uses the
// reserved _Synthetic prefix.
func validateSyntheticShapes(m *model.Model) []Event {
	var events []Event
	for _, s := range m.Shapes() {
		if s.HasTrait(model.TraitSynthetic) {
			for _, traitID := range s.Traits.SortedIDs() {
				if traitID == model.TraitSynthetic {
					continue
				}
				events = append(events, makeEvent(ModelInvalidTraitTarget, s.Source, s.ID,
					"Traits cannot be applied to synthetic shapes: found trait `%s` on assembler-generated shape `%s`",
					traitID, s.ID))
			}
			continue
		}
		if s.IsMember() || !strings.HasPrefix(s.ID.Name, "_Synthetic") {
			continue
		}
		events = append(events, makeWarning(ModelInvalidTraitTarget, s.Source, s.ID,
			"Shape name starts with `_Synthetic`, which is reserved for assembler-generated shapes; consider renaming `%s`",
			s.ID))
	}
	return events
}

// --- syntactic shape IDs ------------------------------------------------------

func validateSyntacticShapeIDs(m *model.Model) []Event {
	var events []Event
	for _, key := range m.MetadataKeys() {
		value, _ := m.Metadata(key)
		events = append(events, checkSyntacticNodes(m, value, model.ShapeID{})...)
	}
	for _, s := range m.Shapes() {
		for _, traitID := range s.Traits.SortedIDs() {
			events = append(events, checkSyntacticNodes(m, s.Traits[traitID].Value, s.ID)...)
		}
	}
	return events
}

func checkSyntacticNodes(m *model.Model, n model.Node, owner model.ShapeID) []Event {
	var events []Event
	if n.SyntacticShapeID {
		if id, err := model.ParseShapeID(n.Str); err != nil || !m.HasShape(id) {
			events = append(events, makeEvent(SyntacticShapeIdTarget, n.Source, owner,
				"Syntactic shape ID `%s` does not resolve to a valid shape ID: `%s`", n.Raw, n.Str))
		}
	}
	for _, child := range n.Arr {
		events = append(events, checkSyntacticNodes(m, child, owner)...)
	}
	for _, entry := range n.Obj {
		events = append(events, checkSyntacticNodes(m, entry.Value, owner)...)
	}
	return events
}

// --- helpers ------------------------------------------------------------------

func makeEvent(id string, loc idl.SourceLocation, shapeID model.ShapeID, format string, args ...any) Event {
	return makeDiagnostic(model.SeverityDanger, id, loc, shapeID, format, args...)
}

func makeWarning(id string, loc idl.SourceLocation, shapeID model.ShapeID, format string, args ...any) Event {
	return makeDiagnostic(model.SeverityWarning, id, loc, shapeID, format, args...)
}

func makeDiagnostic(severity model.Severity, id string, loc idl.SourceLocation, shapeID model.ShapeID, format string, args ...any) Event {
	return model.Diagnostic{
		ID:       id,
		Severity: severity,
		Message:  fmt.Sprintf(format, args...),
		ShapeID:  shapeID,
		Source:   loc,
	}
}

func isIntegerLexeme(s string) bool {
	s = strings.TrimPrefix(s, "-")
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func fitsInt32(s string) bool {
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	// The maximum absolute value is 2147483648 for negative values.
	limit := "2147483647"
	if neg {
		limit = "2147483648"
	}
	s = strings.TrimLeft(s, "0")
	if s == "" {
		return true
	}
	if len(s) != len(limit) {
		return len(s) < len(limit)
	}
	return s <= limit
}

func fitsInteger(s string, typ model.ShapeType) bool {
	neg := strings.HasPrefix(s, "-")
	abs := strings.TrimPrefix(s, "-")
	abs = strings.TrimLeft(abs, "0")
	if abs == "" {
		return true
	}
	var limit string
	switch typ {
	case model.TypeByte:
		limit = "127"
		if neg {
			limit = "128"
		}
	case model.TypeShort:
		limit = "32767"
		if neg {
			limit = "32768"
		}
	case model.TypeInteger:
		limit = "2147483647"
		if neg {
			limit = "2147483648"
		}
	default: // long
		limit = "9223372036854775807"
		if neg {
			limit = "9223372036854775808"
		}
	}
	if len(abs) != len(limit) {
		return len(abs) < len(limit)
	}
	return abs <= limit
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func sortedKeys(m map[string]model.ShapeID) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
