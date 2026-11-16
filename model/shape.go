package model

import (
	"sort"

	"github.com/tespeleta/gosmithy/idl"
)

// ShapeType is the type of a shape in the semantic model.
type ShapeType string

// Timestamp format values accepted by @timestampFormat. The httpJson protocol
// defaults to TimestampDateTime; TimestampEpochMilliseconds is an extension
// defined by this project.
const (
	TimestampDateTime          = "date-time"
	TimestampEpochSeconds      = "epoch-seconds"
	TimestampEpochMilliseconds = "epoch-milliseconds"
	TimestampHTTPDate          = "http-date"
)

const (
	TypeBlob       ShapeType = "blob"
	TypeBoolean    ShapeType = "boolean"
	TypeDocument   ShapeType = "document"
	TypeString     ShapeType = "string"
	TypeByte       ShapeType = "byte"
	TypeShort      ShapeType = "short"
	TypeInteger    ShapeType = "integer"
	TypeLong       ShapeType = "long"
	TypeFloat      ShapeType = "float"
	TypeDouble     ShapeType = "double"
	TypeBigInteger ShapeType = "bigInteger"
	TypeBigDecimal ShapeType = "bigDecimal"
	TypeTimestamp  ShapeType = "timestamp"
	TypeEnum       ShapeType = "enum"
	TypeIntEnum    ShapeType = "intEnum"
	TypeList       ShapeType = "list"
	TypeSet        ShapeType = "set"
	TypeMap        ShapeType = "map"
	TypeStructure  ShapeType = "structure"
	TypeUnion      ShapeType = "union"
	TypeService    ShapeType = "service"
	TypeResource   ShapeType = "resource"
	TypeOperation  ShapeType = "operation"
	TypeMember     ShapeType = "member"
)

// Trait IDs for the prelude traits used by the model, validation, and codegen.
var (
	TraitBlob            = MustShapeID("smithy.api#blob")
	TraitBoolean         = MustShapeID("smithy.api#boolean")
	TraitDocument        = MustShapeID("smithy.api#document")
	TraitString          = MustShapeID("smithy.api#string")
	TraitByte            = MustShapeID("smithy.api#byte")
	TraitShort           = MustShapeID("smithy.api#short")
	TraitInteger         = MustShapeID("smithy.api#integer")
	TraitLong            = MustShapeID("smithy.api#long")
	TraitFloat           = MustShapeID("smithy.api#float")
	TraitDouble          = MustShapeID("smithy.api#double")
	TraitBigInteger      = MustShapeID("smithy.api#bigInteger")
	TraitBigDecimal      = MustShapeID("smithy.api#bigDecimal")
	TraitTimestamp       = MustShapeID("smithy.api#timestamp")
	TraitUnit            = MustShapeID("smithy.api#Unit")
	TraitDocumentation   = MustShapeID("smithy.api#documentation")
	TraitTrait           = MustShapeID("smithy.api#trait")
	TraitMixin           = MustShapeID("smithy.api#mixin")
	TraitRequired        = MustShapeID("smithy.api#required")
	TraitError           = MustShapeID("smithy.api#error")
	TraitInput           = MustShapeID("smithy.api#input")
	TraitOutput          = MustShapeID("smithy.api#output")
	TraitPrivate         = MustShapeID("smithy.api#private")
	TraitEnumValue       = MustShapeID("smithy.api#enumValue")
	TraitDefault         = MustShapeID("smithy.api#default")
	TraitSensitive       = MustShapeID("smithy.api#sensitive")
	TraitDeprecated      = MustShapeID("smithy.api#deprecated")
	TraitInternal        = MustShapeID("smithy.api#internal")
	TraitReadonly        = MustShapeID("smithy.api#readonly")
	TraitIdempotent      = MustShapeID("smithy.api#idempotent")
	TraitSparse          = MustShapeID("smithy.api#sparse")
	TraitUnitType        = MustShapeID("smithy.api#unitType")
	TraitIDRef           = MustShapeID("smithy.api#idRef")
	TraitTags            = MustShapeID("smithy.api#tags")
	TraitSince           = MustShapeID("smithy.api#since")
	TraitTitle           = MustShapeID("smithy.api#title")
	TraitExternalDocs    = MustShapeID("smithy.api#externalDocumentation")
	TraitUnstable        = MustShapeID("smithy.api#unstable")
	TraitSynthetic       = MustShapeID("smithy.api#synthetic")
	TraitPaginated       = MustShapeID("smithy.api#paginated")
	TraitReferences      = MustShapeID("smithy.api#references")
	TraitEnum            = MustShapeID("smithy.api#enum")
	TraitHTTP            = MustShapeID("smithy.api#http")
	TraitHTTPLabel       = MustShapeID("smithy.api#httpLabel")
	TraitHTTPQuery       = MustShapeID("smithy.api#httpQuery")
	TraitHTTPQueryParams = MustShapeID("smithy.api#httpQueryParams")
	TraitHTTPHeader      = MustShapeID("smithy.api#httpHeader")
	TraitHTTPPrefix      = MustShapeID("smithy.api#httpPrefixHeaders")
	TraitHTTPPayload     = MustShapeID("smithy.api#httpPayload")
	TraitHTTPResponse    = MustShapeID("smithy.api#httpResponseCode")
	TraitHTTPError       = MustShapeID("smithy.api#httpError")
	TraitProtocolDef     = MustShapeID("smithy.api#protocolDefinition")
	TraitRetryable       = MustShapeID("smithy.api#retryable")
	TraitStreaming       = MustShapeID("smithy.api#streaming")
	TraitXMLName         = MustShapeID("smithy.api#xmlName")
	TraitJSONName        = MustShapeID("smithy.api#jsonName")
	TraitMediaType       = MustShapeID("smithy.api#mediaType")
	TraitTimestampFmt    = MustShapeID("smithy.api#timestampFormat")
	TraitLength          = MustShapeID("smithy.api#length")
	TraitRange           = MustShapeID("smithy.api#range")
	TraitPattern         = MustShapeID("smithy.api#pattern")
	TraitUniqueItems     = MustShapeID("smithy.api#uniqueItems")
	TraitBox             = MustShapeID("smithy.api#box")
	TraitClientOptional  = MustShapeID("smithy.api#clientOptional")
	TraitAddedDefault    = MustShapeID("smithy.api#addedDefault")
)

// Trait is an applied trait. Traits are generic: the value is a resolved node.
type Trait struct {
	ID     ShapeID
	Value  Node
	Source idl.SourceLocation
}

// TraitMap maps trait shape IDs to applied traits. Iteration order is not
// specified; use SortedIDs or Traits methods for deterministic iteration.
type TraitMap map[ShapeID]*Trait

// NewTraitMap returns an empty trait map.
func NewTraitMap() TraitMap { return make(TraitMap) }

// Get returns the trait with the given ID.
func (m TraitMap) Get(id ShapeID) (*Trait, bool) {
	t, ok := m[id]
	return t, ok
}

// Has reports whether the trait is present.
func (m TraitMap) Has(id ShapeID) bool {
	_, ok := m[id]
	return ok
}

// Set stores a trait.
func (m TraitMap) Set(t *Trait) { m[t.ID] = t }

// Delete removes a trait.
func (m TraitMap) Delete(id ShapeID) { delete(m, id) }

// Len returns the number of traits.
func (m TraitMap) Len() int { return len(m) }

// SortedIDs returns the trait IDs in lexical order.
func (m TraitMap) SortedIDs() []ShapeID {
	ids := make([]ShapeID, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i].String() < ids[j].String() })
	return ids
}

// Clone returns a shallow copy of the trait map. Trait values are immutable.
func (m TraitMap) Clone() TraitMap {
	out := make(TraitMap, len(m))
	for id, t := range m {
		out[id] = t
	}
	return out
}

// ServiceProps holds the properties of a service shape.
type ServiceProps struct {
	Version    string
	Operations []ShapeID
	Resources  []ShapeID
	Errors     []ShapeID
	Rename     map[ShapeID]string
}

// ResourceProps holds the properties of a resource shape.
type ResourceProps struct {
	Identifiers          map[string]ShapeID
	Properties           map[string]ShapeID
	Create               ShapeID
	Read                 ShapeID
	Update               ShapeID
	Delete               ShapeID
	List                 ShapeID
	Put                  ShapeID
	Operations           []ShapeID
	CollectionOperations []ShapeID
	Resources            []ShapeID
}

// Shape is a shape in the semantic model. Member shapes are first-class shapes
// with Type == TypeMember, Container set, and Target set.
type Shape struct {
	ID        ShapeID
	Type      ShapeType
	Traits    TraitMap
	Members   []*Shape
	Target    ShapeID
	Container ShapeID
	Input     ShapeID
	Output    ShapeID
	Errors    []ShapeID
	Mixins    []ShapeID
	Service   *ServiceProps
	Resource  *ResourceProps
	Source    idl.SourceLocation
	Doc       string

	// ForResource is the resource a structure/union/list/map is bound to with
	// the `for` production.
	ForResource ShapeID
	// Elided reports that the member target was elided ($member).
	Elided bool

	prelude bool
	version idl.Version
}

// NewShape returns a shape with the given ID and type.
func NewShape(id ShapeID, typ ShapeType) *Shape {
	return &Shape{ID: id, Type: typ, Traits: NewTraitMap()}
}

// IsPrelude reports whether the shape was loaded from the prelude.
func (s *Shape) IsPrelude() bool { return s.prelude }

// SourceVersion returns the IDL version of the file that defined the shape.
func (s *Shape) SourceVersion() idl.Version { return s.version }

// IsMember reports whether the shape is a member shape.
func (s *Shape) IsMember() bool { return s.Type == TypeMember }

// IsSimple reports whether the shape is a simple shape.
func (s *Shape) IsSimple() bool {
	switch s.Type {
	case TypeBlob, TypeBoolean, TypeDocument, TypeString, TypeByte, TypeShort,
		TypeInteger, TypeLong, TypeFloat, TypeDouble, TypeBigInteger,
		TypeBigDecimal, TypeTimestamp:
		return true
	default:
		return false
	}
}

// IsAggregate reports whether the shape is an aggregate shape.
func (s *Shape) IsAggregate() bool {
	switch s.Type {
	case TypeList, TypeSet, TypeMap, TypeStructure, TypeUnion, TypeEnum, TypeIntEnum:
		return true
	default:
		return false
	}
}

// GetMember returns the member shape with the given name.
func (s *Shape) GetMember(name string) (*Shape, bool) {
	for _, m := range s.Members {
		if m.ID.Member == name {
			return m, true
		}
	}
	return nil, false
}

// AddMember appends a member shape. It does not check for duplicates.
func (s *Shape) AddMember(m *Shape) {
	s.Members = append(s.Members, m)
}

// HasTrait reports whether the shape has the trait.
func (s *Shape) HasTrait(id ShapeID) bool { return s.Traits.Has(id) }

// GetTrait returns the applied trait.
func (s *Shape) GetTrait(id ShapeID) (*Trait, bool) { return s.Traits.Get(id) }

// IsMixin reports whether the shape is a mixin.
func (s *Shape) IsMixin() bool { return s.HasTrait(TraitMixin) }

// IsTraitDefinition reports whether the shape defines a trait.
func (s *Shape) IsTraitDefinition() bool { return s.HasTrait(TraitTrait) }

// IsErrorStructure reports whether the shape is a structure with @error.
func (s *Shape) IsErrorStructure() bool {
	return s.Type == TypeStructure && s.HasTrait(TraitError)
}

// IsUnit reports whether the shape is smithy.api#Unit.
func (s *Shape) IsUnit() bool { return s.ID == TraitUnit }

// IsPrivate reports whether the shape has @private.
func (s *Shape) IsPrivate() bool { return s.HasTrait(TraitPrivate) }

// IsSensitive reports whether the shape has @sensitive.
func (s *Shape) IsSensitive() bool { return s.HasTrait(TraitSensitive) }

// IsDeprecated reports whether the shape has @deprecated.
func (s *Shape) IsDeprecated() bool { return s.HasTrait(TraitDeprecated) }

// IsReadOnly reports whether the shape has @readonly.
func (s *Shape) IsReadOnly() bool { return s.HasTrait(TraitReadonly) }

// IsSparse reports whether the shape has @sparse.
func (s *Shape) IsSparse() bool { return s.HasTrait(TraitSparse) }

// IsInput reports whether the shape has @input.
func (s *Shape) IsInput() bool { return s.HasTrait(TraitInput) }

// IsOutput reports whether the shape has @output.
func (s *Shape) IsOutput() bool { return s.HasTrait(TraitOutput) }

// Documentation returns the documentation trait value.
func (s *Shape) Documentation() (string, bool) {
	t, ok := s.GetTrait(TraitDocumentation)
	if !ok {
		return "", false
	}
	return t.Value.AsString()
}

// Since returns the @since trait value.
func (s *Shape) Since() (string, bool) {
	t, ok := s.GetTrait(TraitSince)
	if !ok {
		return "", false
	}
	return t.Value.AsString()
}

// Tags returns the @tags trait values.
func (s *Shape) Tags() []string {
	t, ok := s.GetTrait(TraitTags)
	if !ok || t.Value.Kind != NodeArray {
		return nil
	}
	out := make([]string, 0, len(t.Value.Arr))
	for _, v := range t.Value.Arr {
		if str, ok := v.AsString(); ok {
			out = append(out, str)
		}
	}
	return out
}

// ErrorType returns the @error trait value ("client" or "server").
func (s *Shape) ErrorType() (string, bool) {
	t, ok := s.GetTrait(TraitError)
	if !ok {
		return "", false
	}
	return t.Value.AsString()
}

// Required reports whether the shape has @required.
func (s *Shape) Required() bool { return s.HasTrait(TraitRequired) }

// EnumValue returns the @enumValue trait value.
func (s *Shape) EnumValue() (Node, bool) {
	t, ok := s.GetTrait(TraitEnumValue)
	if !ok {
		return Node{}, false
	}
	return t.Value, true
}

// Default returns the @default trait value.
func (s *Shape) Default() (Node, bool) {
	t, ok := s.GetTrait(TraitDefault)
	if !ok {
		return Node{}, false
	}
	return t.Value, true
}

// Pattern returns the @pattern trait value.
func (s *Shape) Pattern() (string, bool) {
	t, ok := s.GetTrait(TraitPattern)
	if !ok {
		return "", false
	}
	return t.Value.AsString()
}

// MixinLocalTraits returns the trait IDs that a mixin does not copy to shapes
// that use it.
func (s *Shape) MixinLocalTraits() []ShapeID {
	t, ok := s.GetTrait(TraitMixin)
	if !ok {
		return nil
	}
	v, ok := t.Value.Get("localTraits")
	if !ok || v.Kind != NodeArray {
		return nil
	}
	var out []ShapeID
	for _, item := range v.Arr {
		if item.SyntacticShapeID || item.Kind == NodeString {
			if id, err := ParseShapeID(item.Str); err == nil {
				out = append(out, id)
			}
		}
	}
	return out
}

// Length returns the @length constraint bounds. A nil bound is absent.
func (s *Shape) Length() (min, max *int, ok bool) {
	t, ok := s.GetTrait(TraitLength)
	if !ok {
		return nil, nil, false
	}
	return intBound(t.Value, "min"), intBound(t.Value, "max"), true
}

// Range returns the @range constraint bounds. A nil bound is absent.
func (s *Shape) Range() (min, max *float64, ok bool) {
	t, ok := s.GetTrait(TraitRange)
	if !ok {
		return nil, nil, false
	}
	return floatBound(t.Value, "min"), floatBound(t.Value, "max"), true
}

// RangeBounds returns the @range constraint bounds as their original number
// lexemes. A nil bound is absent. Unlike Range, it does not lose precision.
func (s *Shape) RangeBounds() (min, max *string, ok bool) {
	t, ok := s.GetTrait(TraitRange)
	if !ok {
		return nil, nil, false
	}
	return numberBound(t.Value, "min"), numberBound(t.Value, "max"), true
}

func numberBound(n Node, key string) *string {
	v, ok := n.Get(key)
	if !ok || v.Kind != NodeNumber {
		return nil
	}
	out := v.Num
	return &out
}

func intBound(n Node, key string) *int {
	v, ok := n.Get(key)
	if !ok {
		return nil
	}
	i, ok := v.AsInt()
	if !ok {
		return nil
	}
	out := int(i)
	return &out
}

func floatBound(n Node, key string) *float64 {
	v, ok := n.Get(key)
	if !ok {
		return nil
	}
	f, ok := v.AsFloat()
	if !ok {
		return nil
	}
	return &f
}

// HTTP returns the @http trait values: method, request URI, and response code.
func (s *Shape) HTTP() (method, uri string, code int, ok bool) {
	t, found := s.GetTrait(TraitHTTP)
	if !found {
		return "", "", 0, false
	}
	method, _ = t.Value.GetString("method")
	uri, _ = t.Value.GetString("uri")
	if v, found := t.Value.Get("code"); found {
		if n, valid := v.AsInt(); valid {
			code = int(n)
		}
	}
	return method, uri, code, true
}

// HTTPError returns the @httpError status code.
func (s *Shape) HTTPError() (int, bool) {
	t, ok := s.GetTrait(TraitHTTPError)
	if !ok {
		return 0, false
	}
	n, ok := t.Value.AsInt()
	if !ok {
		return 0, false
	}
	return int(n), true
}

// HTTPLabel reports whether the member is bound to an HTTP label.
func (s *Shape) HTTPLabel() bool { return s.HasTrait(TraitHTTPLabel) }

// HTTPQuery returns the @httpQuery member name.
func (s *Shape) HTTPQuery() (string, bool) {
	t, ok := s.GetTrait(TraitHTTPQuery)
	if !ok {
		return "", false
	}
	return t.Value.AsString()
}

// HTTPQueryParams reports whether the member is bound to the query string.
func (s *Shape) HTTPQueryParams() bool { return s.HasTrait(TraitHTTPQueryParams) }

// HTTPHeader returns the @httpHeader member name.
func (s *Shape) HTTPHeader() (string, bool) {
	t, ok := s.GetTrait(TraitHTTPHeader)
	if !ok {
		return "", false
	}
	return t.Value.AsString()
}

// HTTPPrefixHeaders returns the @httpPrefixHeaders prefix.
func (s *Shape) HTTPPrefixHeaders() (string, bool) {
	t, ok := s.GetTrait(TraitHTTPPrefix)
	if !ok {
		return "", false
	}
	return t.Value.AsString()
}

// HTTPPayload reports whether the member is an HTTP payload.
func (s *Shape) HTTPPayload() bool { return s.HasTrait(TraitHTTPPayload) }

// HTTPResponseCode reports whether the member is bound to the status code.
func (s *Shape) HTTPResponseCode() bool { return s.HasTrait(TraitHTTPResponse) }

// Paginated returns the @paginated trait values.
func (s *Shape) Paginated() (inputToken, outputToken, pageSize, items string, ok bool) {
	t, found := s.GetTrait(TraitPaginated)
	if !found {
		return "", "", "", "", false
	}
	inputToken, _ = t.Value.GetString("inputToken")
	outputToken, _ = t.Value.GetString("outputToken")
	pageSize, _ = t.Value.GetString("pageSize")
	items, _ = t.Value.GetString("items")
	return inputToken, outputToken, pageSize, items, true
}

// IDRef returns the @idRef trait values.
func (s *Shape) IDRef() (selector string, failWhenMissing bool, ok bool) {
	t, found := s.GetTrait(TraitIDRef)
	if !found {
		return "", false, false
	}
	selector, _ = t.Value.GetString("selector")
	if v, found := t.Value.Get("failWhenMissing"); found && v.Kind == NodeBoolean {
		failWhenMissing = v.Bool
	}
	return selector, failWhenMissing, true
}

// Reference is a single entry of the @references trait.
type Reference struct {
	Resource string
	IDs      map[string]string
	Service  string
	Rel      string
}

// References returns the @references trait values.
func (s *Shape) References() ([]Reference, bool) {
	t, ok := s.GetTrait(TraitReferences)
	if !ok || t.Value.Kind != NodeArray {
		return nil, false
	}
	out := make([]Reference, 0, len(t.Value.Arr))
	for _, entry := range t.Value.Arr {
		if entry.Kind != NodeObject {
			continue
		}
		ref := Reference{IDs: make(map[string]string)}
		ref.Resource, _ = entry.GetString("resource")
		ref.Service, _ = entry.GetString("service")
		ref.Rel, _ = entry.GetString("rel")
		if ids, ok := entry.Get("ids"); ok && ids.Kind == NodeObject {
			for _, pair := range ids.Obj {
				if str, ok := pair.Value.AsString(); ok {
					ref.IDs[pair.Key] = str
				}
			}
		}
		out = append(out, ref)
	}
	return out, true
}

// MediaType returns the @mediaType trait value.
func (s *Shape) MediaType() (string, bool) {
	t, ok := s.GetTrait(TraitMediaType)
	if !ok {
		return "", false
	}
	return t.Value.AsString()
}

// JSONName returns the @jsonName trait value.
func (s *Shape) JSONName() (string, bool) {
	t, ok := s.GetTrait(TraitJSONName)
	if !ok {
		return "", false
	}
	return t.Value.AsString()
}

// TimestampFormat returns the @timestampFormat trait value.
func (s *Shape) TimestampFormat() (string, bool) {
	t, ok := s.GetTrait(TraitTimestampFmt)
	if !ok {
		return "", false
	}
	return t.Value.AsString()
}
