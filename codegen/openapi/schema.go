package openapi

import (
	"fmt"
	"sort"
	"strings"

	"github.com/tespeleta/gosmithy/model"
)

var (
	traitRecommended      = model.MustShapeID("smithy.api#recommended")
	traitUnstableFeatures = model.MustShapeID("smithy.api#unstableFeatures")
)

func (b *builder) schemaForShape(shape *model.Shape) *Schema {
	if shape == nil {
		return &Schema{}
	}
	if shape.IsMember() {
		target, ok := b.model.Shape(shape.Target)
		if !ok {
			return &Schema{}
		}
		return b.schemaForMember(shape, target)
	}
	if b.needsComponent(shape) {
		return b.componentRef(shape)
	}
	return b.inlineShapeSchema(shape)
}

func (b *builder) schemaForMember(member, target *model.Shape) *Schema {
	base := b.schemaForShape(target)
	overlay := b.memberOverlay(member, target)
	if overlay == nil {
		return base
	}
	if base.Ref != "" {
		overlay.AllOf = []*Schema{base}
		return overlay
	}
	out := cloneSchema(base)
	mergeSchema(out, overlay)
	return out
}

func (b *builder) needsComponent(shape *model.Shape) bool {
	switch shape.Type {
	case model.TypeStructure, model.TypeUnion, model.TypeEnum, model.TypeIntEnum,
		model.TypeList, model.TypeSet, model.TypeMap:
		return true
	case model.TypeString:
		return shape.HasTrait(model.TraitEnum)
	default:
		return false
	}
}

func (b *builder) componentRef(shape *model.Shape) *Schema {
	name := b.schemaName(shape)
	if _, exists := b.components[name]; !exists {
		placeholder := &Schema{}
		b.components[name] = placeholder
		built := b.inlineShapeSchema(shape)
		*placeholder = *built
	}
	return &Schema{Ref: schemaRef(name)}
}

func (b *builder) schemaName(shape *model.Shape) string {
	if name, ok := b.names[shape.ID]; ok {
		return name
	}
	name := shape.ID.Name
	if b.service.Service != nil {
		if renamed, ok := b.service.Service.Rename[shape.ID]; ok && renamed != "" {
			name = renamed
		}
	}
	if previous, exists := b.nameOwner[name]; exists && previous != shape.ID {
		b.danger(shape, "OpenAPI schema name %q for `%s` collides with `%s`; use the service rename map", name, shape.ID, previous)
		name = b.uniqueName(name)
	}
	b.names[shape.ID] = name
	b.nameOwner[name] = shape.ID
	return name
}

func (b *builder) uniqueName(base string) string {
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s%d", base, i)
		if _, exists := b.nameOwner[candidate]; !exists {
			return candidate
		}
	}
}

func schemaRef(name string) string {
	name = strings.ReplaceAll(name, "~", "~0")
	name = strings.ReplaceAll(name, "/", "~1")
	return "#/components/schemas/" + name
}

func (b *builder) inlineShapeSchema(shape *model.Shape) *Schema {
	schema := &Schema{}
	b.applyShapeAnnotations(schema, shape)
	if shape.Type == model.TypeString && shape.HasTrait(model.TraitEnum) {
		schema.Type = "string"
		schema.Enum, schema.EnumDescriptions, schema.EnumDeprecated, schema.EnumSince = enumTraitValues(shape)
		b.applyShapeConstraints(schema, shape)
		return schema
	}
	switch shape.Type {
	case model.TypeStructure:
		schema.Type = "object"
		schema.Properties = make(map[string]*Schema, len(shape.Members))
		for _, member := range shape.Members {
			target, ok := b.model.Shape(member.Target)
			if !ok {
				continue
			}
			name := member.ID.Member
			if jsonName, ok := member.JSONName(); ok && jsonName != "" {
				name = jsonName
			}
			schema.Properties[name] = b.schemaForMember(member, target)
			if member.Required() {
				schema.Required = append(schema.Required, name)
			}
		}
		sort.Strings(schema.Required)
	case model.TypeUnion:
		schema.OneOf = make([]*Schema, 0, len(shape.Members))
		for _, member := range shape.Members {
			target, ok := b.model.Shape(member.Target)
			if !ok {
				continue
			}
			name := member.ID.Member
			if jsonName, ok := member.JSONName(); ok && jsonName != "" {
				name = jsonName
			}
			wrapper := &Schema{
				Type:        "object",
				Title:       member.ID.Member,
				Description: b.memberDocumentation(member),
				Properties: map[string]*Schema{
					name: b.schemaForMember(member, target),
				},
				Required: []string{name},
			}
			schema.OneOf = append(schema.OneOf, wrapper)
		}
	case model.TypeList, model.TypeSet:
		schema.Type = "array"
		if member, ok := shape.GetMember("member"); ok {
			if target, ok := b.model.Shape(member.Target); ok {
				item := b.schemaForMember(member, target)
				if shape.IsSparse() {
					item = nullableSchema(item)
				}
				schema.Items = item
			}
		}
		if shape.Type == model.TypeSet {
			schema.UniqueItems = true
		}
	case model.TypeMap:
		schema.Type = "object"
		if member, ok := shape.GetMember("value"); ok {
			if target, ok := b.model.Shape(member.Target); ok {
				value := b.schemaForMember(member, target)
				if shape.IsSparse() {
					value = nullableSchema(value)
				}
				schema.AdditionalProperties = value
			}
		}
	case model.TypeEnum:
		schema.Type = "string"
		schema.Enum, schema.EnumDescriptions, schema.EnumDeprecated, schema.EnumSince = b.enumShapeValues(shape, false)
	case model.TypeIntEnum:
		schema.Type = "integer"
		schema.Format = "int32"
		schema.Enum, schema.EnumDescriptions, schema.EnumDeprecated, schema.EnumSince = b.enumShapeValues(shape, true)
	default:
		b.applySimpleSchema(schema, shape)
	}
	b.applyShapeConstraints(schema, shape)
	if shape.HasTrait(model.TraitSensitive) && schema.Type == "string" && schema.Format == "" {
		schema.Format = "password"
	}
	return schema
}

func (b *builder) applySimpleSchema(schema *Schema, shape *model.Shape) {
	switch shape.Type {
	case model.TypeBlob:
		schema.Type = "string"
		schema.Format = "byte"
	case model.TypeBoolean:
		schema.Type = "boolean"
	case model.TypeString:
		schema.Type = "string"
	case model.TypeByte:
		schema.Type = "integer"
		schema.Format = "int32"
	case model.TypeShort:
		schema.Type = "integer"
		schema.Format = "int32"
	case model.TypeInteger:
		schema.Type = "integer"
		schema.Format = "int32"
	case model.TypeLong:
		schema.Type = "integer"
		schema.Format = "int64"
	case model.TypeFloat:
		schema.Type = "number"
		schema.Format = "float"
	case model.TypeDouble:
		schema.Type = "number"
		schema.Format = "double"
	case model.TypeBigInteger:
		schema.Type = "integer"
		schema.Format = "bigint"
	case model.TypeBigDecimal:
		schema.Type = "number"
		schema.Format = "decimal"
	case model.TypeTimestamp:
		b.applyTimestampFormat(schema, shape.TimestampFormat, "")
	case model.TypeDocument:
		// An empty schema accepts any JSON value.
	default:
		schema.Type = "string"
	}
}

func (b *builder) applyTimestampFormat(schema *Schema, get func() (string, bool), memberFormat string) {
	format := memberFormat
	if format == "" {
		if value, ok := get(); ok {
			format = value
		}
	}
	switch format {
	case "epoch-seconds":
		schema.Type = "number"
		schema.Format = "double"
	case "epoch-milliseconds":
		schema.Type = "integer"
		schema.Format = "int64"
	case "http-date":
		schema.Type = "string"
		schema.Format = "http-date"
	default:
		schema.Type = "string"
		schema.Format = "date-time"
	}
}

func (b *builder) applyShapeAnnotations(schema *Schema, shape *model.Shape) {
	schema.Description = b.effectiveDocumentation(shape)
	schema.Deprecated = shape.IsDeprecated()
	schema.Internal = shape.HasTrait(model.TraitInternal)
	schema.Unstable = shape.HasTrait(model.TraitUnstable)
	schema.UnstableFeature = b.unstableFeature(shape)
	schema.Synthetic = shape.HasTrait(model.TraitSynthetic)
	schema.Recommended = shape.HasTrait(traitRecommended)
	if title, ok := b.stringTrait(shape, model.TraitTitle); ok && title != "" {
		schema.Title = title
	}
	if since, ok := b.stringTrait(shape, model.TraitSince); ok {
		schema.Since = since
	}
	if docs := b.externalDocs(shape); docs != nil {
		schema.ExternalDocs = docs
	}
}

func (b *builder) memberOverlay(member, target *model.Shape) *Schema {
	overlay := &Schema{}
	changed := false
	if doc := b.memberDescription(member, target); doc != "" {
		overlay.Description = doc
		changed = true
	}
	if title, ok := b.stringTrait(member, model.TraitTitle); ok && title != "" {
		overlay.Title = title
		changed = true
	}
	if docs := b.externalDocs(member); docs != nil {
		overlay.ExternalDocs = docs
		changed = true
	}
	if member.IsDeprecated() {
		overlay.Deprecated = true
		changed = true
	}
	if member.HasTrait(model.TraitInternal) {
		overlay.Internal = true
		changed = true
	}
	if member.HasTrait(traitRecommended) {
		overlay.Recommended = true
		changed = true
	}
	if member.HasTrait(model.TraitUnstable) {
		overlay.Unstable = true
		overlay.UnstableFeature = b.unstableFeature(member)
		changed = true
	}
	if since, ok := b.stringTrait(member, model.TraitSince); ok {
		overlay.Since = since
		changed = true
	}
	if member.HasTrait(model.TraitSensitive) {
		if target != nil && target.Type == model.TypeString {
			overlay.Format = "password"
			changed = true
		}
	}
	if target != nil && target.Type == model.TypeTimestamp {
		if format, ok := member.TimestampFormat(); ok && format != "" {
			b.applyTimestampFormat(overlay, func() (string, bool) { return "", false }, format)
			changed = true
		}
	}
	if b.applyMemberConstraints(overlay, member, target) {
		changed = true
	}
	if !changed {
		return nil
	}
	return overlay
}

func (b *builder) applyMemberConstraints(schema *Schema, member, target *model.Shape) bool {
	changed := false
	if member.HasTrait(model.TraitLength) {
		min, max, _ := member.Length()
		switch {
		case target != nil && (target.Type == model.TypeList || target.Type == model.TypeSet):
			if min != nil {
				schema.MinItems = min
				changed = true
			}
			if max != nil {
				schema.MaxItems = max
				changed = true
			}
		case target != nil && target.Type == model.TypeMap:
			if min != nil {
				schema.MinProperties = min
				changed = true
			}
			if max != nil {
				schema.MaxProperties = max
				changed = true
			}
		default:
			if min != nil {
				schema.MinLength = min
				changed = true
			}
			if max != nil {
				schema.MaxLength = max
				changed = true
			}
		}
	}
	if member.HasTrait(model.TraitRange) {
		min, max, _ := member.Range()
		if min != nil {
			schema.Min = min
			changed = true
		}
		if max != nil {
			schema.Max = max
			changed = true
		}
	}
	if pattern, ok := member.Pattern(); ok && pattern != "" {
		schema.Pattern = pattern
		changed = true
	}
	if member.HasTrait(model.TraitUniqueItems) && target != nil &&
		(target.Type == model.TypeList || target.Type == model.TypeSet) {
		schema.UniqueItems = true
		changed = true
	}
	return changed
}

func (b *builder) applyShapeConstraints(schema *Schema, shape *model.Shape) {
	if min, max, ok := shape.Length(); ok {
		switch shape.Type {
		case model.TypeString, model.TypeBlob:
			schema.MinLength = min
			schema.MaxLength = max
		case model.TypeList, model.TypeSet:
			schema.MinItems = min
			schema.MaxItems = max
		case model.TypeMap:
			schema.MinProperties = min
			schema.MaxProperties = max
		}
	}
	if min, max, ok := shape.Range(); ok {
		schema.Min = min
		schema.Max = max
	}
	if pattern, ok := shape.Pattern(); ok && pattern != "" {
		schema.Pattern = pattern
	}
	if shape.HasTrait(model.TraitUniqueItems) {
		schema.UniqueItems = true
	}
}

func nullableSchema(schema *Schema) *Schema {
	if schema == nil {
		return &Schema{Nullable: true}
	}
	if schema.Ref != "" {
		return &Schema{AllOf: []*Schema{schema}, Nullable: true}
	}
	out := cloneSchema(schema)
	out.Nullable = true
	return out
}

func cloneSchema(schema *Schema) *Schema {
	if schema == nil {
		return &Schema{}
	}
	out := *schema
	if schema.Properties != nil {
		out.Properties = make(map[string]*Schema, len(schema.Properties))
		for key, value := range schema.Properties {
			out.Properties[key] = value
		}
	}
	out.Required = append([]string(nil), schema.Required...)
	out.Enum = append([]interface{}(nil), schema.Enum...)
	out.OneOf = append([]*Schema(nil), schema.OneOf...)
	out.AllOf = append([]*Schema(nil), schema.AllOf...)
	return &out
}

func mergeSchema(dst, src *Schema) {
	if src.Description != "" {
		dst.Description = src.Description
	}
	if src.Title != "" {
		dst.Title = src.Title
	}
	if src.Type != "" {
		dst.Type = src.Type
	}
	if src.Format != "" {
		dst.Format = src.Format
	}
	if src.ExternalDocs != nil {
		dst.ExternalDocs = src.ExternalDocs
	}
	dst.Deprecated = dst.Deprecated || src.Deprecated
	dst.Internal = dst.Internal || src.Internal
	dst.Recommended = dst.Recommended || src.Recommended
	dst.Unstable = dst.Unstable || src.Unstable
	dst.Synthetic = dst.Synthetic || src.Synthetic
	if src.UnstableFeature != "" {
		dst.UnstableFeature = src.UnstableFeature
	}
	dst.Nullable = dst.Nullable || src.Nullable
	dst.UniqueItems = dst.UniqueItems || src.UniqueItems
	if src.Since != "" {
		dst.Since = src.Since
	}
	if src.Min != nil {
		dst.Min = src.Min
	}
	if src.Max != nil {
		dst.Max = src.Max
	}
	if src.MinLength != nil {
		dst.MinLength = src.MinLength
	}
	if src.MaxLength != nil {
		dst.MaxLength = src.MaxLength
	}
	if src.MinItems != nil {
		dst.MinItems = src.MinItems
	}
	if src.MaxItems != nil {
		dst.MaxItems = src.MaxItems
	}
	if src.MinProperties != nil {
		dst.MinProperties = src.MinProperties
	}
	if src.MaxProperties != nil {
		dst.MaxProperties = src.MaxProperties
	}
	if src.Pattern != "" {
		dst.Pattern = src.Pattern
	}
}

func (b *builder) enumShapeValues(shape *model.Shape, integers bool) ([]interface{}, map[string]string, []string, map[string]string) {
	values := make([]interface{}, 0, len(shape.Members))
	descriptions := make(map[string]string)
	deprecated := make([]string, 0)
	since := make(map[string]string)
	for _, member := range shape.Members {
		value, ok := member.EnumValue()
		if !ok {
			continue
		}
		var key string
		if integers {
			number, ok := value.AsInt()
			if !ok {
				continue
			}
			values = append(values, number)
			key = fmt.Sprintf("%d", number)
		} else {
			text, ok := value.AsString()
			if !ok {
				continue
			}
			values = append(values, text)
			key = text
		}
		if doc := b.memberDocumentation(member); doc != "" {
			descriptions[key] = doc
		}
		if member.IsDeprecated() {
			deprecated = append(deprecated, key)
		}
		if value, ok := b.stringTrait(member, model.TraitSince); ok {
			since[key] = value
		}
	}
	if len(descriptions) == 0 {
		descriptions = nil
	}
	if len(since) == 0 {
		since = nil
	}
	return values, descriptions, deprecated, since
}

func enumTraitValues(shape *model.Shape) ([]interface{}, map[string]string, []string, map[string]string) {
	trait, ok := shape.GetTrait(model.TraitEnum)
	if !ok || trait.Value.Kind != model.NodeArray {
		return nil, nil, nil, nil
	}
	values := make([]interface{}, 0, len(trait.Value.Arr))
	descriptions := make(map[string]string)
	deprecated := make([]string, 0)
	for _, entry := range trait.Value.Arr {
		if entry.Kind != model.NodeObject {
			continue
		}
		value, ok := entry.GetString("value")
		if !ok {
			continue
		}
		values = append(values, value)
		if doc, ok := entry.GetString("documentation"); ok && doc != "" {
			descriptions[value] = doc
		}
		if flag, ok := entry.Get("deprecated"); ok && flag.Kind == model.NodeBoolean && flag.Bool {
			deprecated = append(deprecated, value)
		}
	}
	if len(descriptions) == 0 {
		descriptions = nil
	}
	return values, descriptions, deprecated, nil
}
