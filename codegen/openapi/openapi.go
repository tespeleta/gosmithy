package openapi

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/tespeleta/gosmithy/codegen"
	"github.com/tespeleta/gosmithy/idl"
	"github.com/tespeleta/gosmithy/model"
	"github.com/tespeleta/gosmithy/model/httpbinding"
	"github.com/tespeleta/gosmithy/protocol/httpjson"
	"github.com/tespeleta/gosmithy/validation"
)

const (
	// Namespace is the OpenAPI generator namespace.
	Namespace = "openapi"
	// GeneratorName is the legacy OpenAPI option name.
	GeneratorName = Namespace
	// OptionDescriptor emits the OpenAPI JSON document.
	OptionDescriptor = "descriptor"
	// OptionIndex emits the Redoc HTML index and its docs-local descriptor.
	OptionIndex = "index"
)

// DefaultVersion is the OpenAPI specification version emitted by the generator.
const DefaultVersion = "3.0.3"

type generatorMode int

const (
	modeDescriptor generatorMode = iota
	modeIndex
)

// New returns the OpenAPI descriptor generator.
func New() codegen.Generator {
	return &generator{mode: modeDescriptor, name: Namespace + ":" + OptionDescriptor}
}

// NewDescriptor returns the OpenAPI descriptor generator.
func NewDescriptor() codegen.Generator {
	return &generator{mode: modeDescriptor, name: Namespace + ":" + OptionDescriptor}
}

// NewIndex returns the generator for the docs-local descriptor and Redoc HTML.
func NewIndex() codegen.Generator {
	return &generator{mode: modeIndex, name: Namespace + ":" + OptionIndex}
}

type generator struct {
	mode generatorMode
	name string
}

func (g *generator) Name() string {
	if g.name != "" {
		return g.name
	}
	return GeneratorName
}

func (g *generator) Generate(ctx context.Context, req *codegen.Request) (*codegen.Response, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	doc, diagnostics := Build(req)
	response := &codegen.Response{Diagnostics: diagnostics}
	if doc == nil {
		return response, nil
	}
	data, err := doc.JSON()
	if err != nil {
		return nil, fmt.Errorf("openapi: encoding document: %w", err)
	}
	switch g.mode {
	case modeIndex:
		response.Files = []codegen.File{
			{Path: "docs/openapi.json", Content: data},
			{Path: "docs/index.html", Content: renderHTML(doc)},
		}
	default:
		response.Files = []codegen.File{{Path: "openapi.json", Content: data}}
	}
	return response, nil
}

// JSON renders the document as deterministic, indented JSON.
func (d *Document) JSON() ([]byte, error) {
	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// Build converts the request's service into an OpenAPI document. It returns
// diagnostics instead of an error for model and protocol problems; DANGER
// diagnostics prevent the runner from writing the document.
func Build(req *codegen.Request) (*Document, []codegen.Diagnostic) {
	var diagnostics []codegen.Diagnostic
	if req == nil || req.Model == nil {
		return nil, danger(diagnostics, "openapi generation requires an assembled model")
	}
	if req.Service.IsZero() {
		return nil, danger(diagnostics, "openapi generation requires a service")
	}
	service, ok := req.Model.Shape(req.Service)
	if !ok {
		return nil, danger(diagnostics, "service `%s` is not found in the model", req.Service)
	}
	if service.Type != model.TypeService {
		return nil, danger(diagnostics, "`%s` is a %s shape, not a service", req.Service, service.Type)
	}
	if req.Protocol == nil {
		return nil, danger(diagnostics, "openapi generation requires a protocol; the CLI supplies %s", httpjson.TraitID)
	}
	if !httpjson.Is(req.Protocol.ID) {
		return nil, danger(diagnostics, "openapi generation supports %s, not `%s`", httpjson.TraitID, req.Protocol.ID)
	}

	bindings, events := httpbinding.Index(req.Model, req.Service)
	for _, event := range events {
		diagnostics = append(diagnostics, codegen.Diagnostic{
			Severity: event.Severity,
			Message:  event.Message,
			Path:     event.Source.Filename,
			Line:     event.Source.Line,
			Column:   event.Source.Column,
		})
	}

	b := &builder{
		model:        req.Model,
		service:      service,
		names:        make(map[model.ShapeID]string),
		nameOwner:    make(map[string]model.ShapeID),
		operationIDs: make(map[string]model.ShapeID),
		components:   make(map[string]*Schema),
		diagnostics:  diagnostics,
	}
	doc := b.build(bindings)
	return doc, b.diagnostics
}

type builder struct {
	model        *model.Model
	service      *model.Shape
	names        map[model.ShapeID]string
	nameOwner    map[string]model.ShapeID
	operationIDs map[string]model.ShapeID
	components   map[string]*Schema
	diagnostics  []codegen.Diagnostic
}

func (b *builder) build(bindings []*httpbinding.Binding) *Document {
	doc := &Document{
		OpenAPI:          DefaultVersion,
		Info:             b.info(),
		Tags:             b.tags(bindings),
		Paths:            make(map[string]*PathItem),
		Internal:         b.service.HasTrait(model.TraitInternal),
		Deprecated:       b.service.IsDeprecated(),
		UnstableFeatures: b.unstableFeatures(),
	}
	doc.Since, _ = b.stringTrait(b.service, model.TraitSince)
	doc.Unstable = b.service.HasTrait(model.TraitUnstable)
	doc.UnstableFeature = b.unstableFeature(b.service)
	if docs := b.externalDocs(b.service); docs != nil {
		doc.ExternalDocs = docs
	}
	for _, binding := range bindings {
		b.addBinding(doc, binding)
	}
	if len(b.components) > 0 {
		doc.Components = &Components{Schemas: b.components}
	}
	return doc
}

func (b *builder) unstableFeatures() []string {
	trait, ok := b.service.GetTrait(traitUnstableFeatures)
	if !ok || trait.Value.Kind != model.NodeObject {
		return nil
	}
	seen := make(map[string]bool)
	for _, entry := range trait.Value.Obj {
		if entry.Key != "" {
			seen[entry.Key] = true
		}
	}
	if len(seen) == 0 {
		return nil
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (b *builder) info() Info {
	info := Info{
		Title:       b.service.ID.Name,
		Description: b.effectiveDocumentation(b.service),
		Version:     "0.0.0",
	}
	if title, ok := b.stringTrait(b.service, model.TraitTitle); ok && title != "" {
		info.Title = title
	}
	if b.service.Service != nil && b.service.Service.Version != "" {
		info.Version = b.service.Service.Version
	}
	return info
}

func (b *builder) tags(bindings []*httpbinding.Binding) []Tag {
	seen := make(map[string]bool)
	for _, tag := range b.service.Tags() {
		seen[tag] = true
	}
	for _, binding := range bindings {
		op, ok := b.model.Shape(binding.Operation)
		if !ok {
			continue
		}
		for _, tag := range op.Tags() {
			seen[tag] = true
		}
	}
	if len(seen) == 0 {
		return nil
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	tags := make([]Tag, 0, len(names))
	for _, name := range names {
		tags = append(tags, Tag{Name: name})
	}
	return tags
}

func (b *builder) addBinding(doc *Document, binding *httpbinding.Binding) {
	op, ok := b.model.Shape(binding.Operation)
	if !ok || op.Type != model.TypeOperation {
		return
	}
	examples := b.operationExamples(op)
	method := strings.ToLower(strings.TrimSpace(binding.Method))
	pathItem := doc.Paths[binding.URI]
	if pathItem == nil {
		pathItem = &PathItem{}
		doc.Paths[binding.URI] = pathItem
	}
	operation := &Operation{
		Summary:         b.operationSummary(op),
		Description:     b.effectiveDocumentation(op),
		OperationID:     b.operationID(op),
		Tags:            uniqueStrings(op.Tags()),
		Deprecated:      op.IsDeprecated(),
		Internal:        op.HasTrait(model.TraitInternal),
		Unstable:        op.HasTrait(model.TraitUnstable),
		UnstableFeature: b.unstableFeature(op),
		Parameters:      b.parameters(binding, examples),
		RequestBody:     b.requestBody(binding, examples),
		Responses:       b.responses(binding, examples),
	}
	operation.Since, _ = b.stringTrait(op, model.TraitSince)
	if docs := b.externalDocs(op); docs != nil {
		operation.ExternalDocs = docs
	}
	if !setOperation(pathItem, method, operation) {
		b.danger(op, "OpenAPI does not support HTTP method `%s` for operation `%s`", binding.Method, op.ID)
		return
	}
}

func (b *builder) operationSummary(op *model.Shape) string {
	if title, ok := b.stringTrait(op, model.TraitTitle); ok && title != "" {
		return title
	}
	return op.ID.Name
}

func (b *builder) operationID(op *model.Shape) string {
	name := op.ID.Name
	if b.service.Service != nil {
		if renamed, ok := b.service.Service.Rename[op.ID]; ok && renamed != "" {
			name = renamed
		}
	}
	if previous, exists := b.operationIDs[name]; exists && previous != op.ID {
		b.danger(op, "OpenAPI operationId %q for `%s` collides with `%s`; use the service rename map", name, op.ID, previous)
		name = b.uniqueOperationID(name)
	}
	b.operationIDs[name] = op.ID
	return name
}

func (b *builder) uniqueOperationID(base string) string {
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s%d", base, i)
		if _, exists := b.operationIDs[candidate]; !exists {
			return candidate
		}
	}
}

func (b *builder) parameters(binding *httpbinding.Binding, examples []operationExample) []*Parameter {
	var params []*Parameter
	for _, segment := range binding.Segments {
		if !segment.IsLabel {
			continue
		}
		id, ok := binding.Input.Labels[segment.Label]
		if !ok {
			continue
		}
		member, target, ok := b.memberAndTarget(id)
		if !ok {
			continue
		}
		param := &Parameter{
			Name:        segment.Label,
			In:          "path",
			Description: b.memberDescription(member, target),
			Required:    true,
			Schema:      b.schemaForMember(member, target),
		}
		b.applyParameterAnnotations(param, member)
		if values := b.parameterExamples(member, examples); len(values) > 0 {
			param.Examples = values
		}
		params = append(params, param)
	}

	queryNames := sortedBindingNames(binding.Input.Query)
	for _, name := range queryNames {
		member, target, ok := b.memberAndTarget(binding.Input.Query[name])
		if !ok {
			continue
		}
		param := &Parameter{
			Name:        name,
			In:          "query",
			Description: b.memberDescription(member, target),
			Required:    member.Required(),
			Schema:      b.schemaForMember(member, target),
			Style:       "form",
			Explode:     boolPtr(true),
		}
		b.applyParameterAnnotations(param, member)
		if values := b.parameterExamples(member, examples); len(values) > 0 {
			param.Examples = values
		}
		params = append(params, param)
	}
	if !binding.Input.QueryParams.IsZero() {
		member, target, ok := b.memberAndTarget(binding.Input.QueryParams)
		if ok {
			param := &Parameter{
				Name:        member.ID.Member,
				In:          "query",
				Description: b.memberDescription(member, target),
				Required:    member.Required(),
				Schema:      b.schemaForMember(member, target),
				Style:       "form",
				Explode:     boolPtr(true),
			}
			b.applyParameterAnnotations(param, member)
			params = append(params, param)
		}
	}

	headerNames := sortedBindingNames(binding.Input.Headers)
	for _, name := range headerNames {
		member, target, ok := b.memberAndTarget(binding.Input.Headers[name])
		if !ok {
			continue
		}
		param := &Parameter{
			Name:        name,
			In:          "header",
			Description: b.memberDescription(member, target),
			Required:    member.Required(),
			Schema:      b.schemaForMember(member, target),
		}
		b.applyParameterAnnotations(param, member)
		if values := b.parameterExamples(member, examples); len(values) > 0 {
			param.Examples = values
		}
		params = append(params, param)
	}
	if !binding.Input.PrefixHeaders.IsZero() {
		if member, _, ok := b.memberAndTarget(binding.Input.PrefixHeaders); ok {
			b.warn(binding.Operation, "@httpPrefixHeaders on `%s` is not represented in OpenAPI", member.ID)
		}
	}
	return params
}

func (b *builder) applyParameterAnnotations(param *Parameter, member *model.Shape) {
	param.Internal = member.HasTrait(model.TraitInternal)
	param.Recommended = member.HasTrait(traitRecommended)
	param.Deprecated = member.IsDeprecated()
	param.Unstable = member.HasTrait(model.TraitUnstable)
	param.UnstableFeature = b.unstableFeature(member)
	param.Since, _ = b.stringTrait(member, model.TraitSince)
}

func (b *builder) requestBody(binding *httpbinding.Binding, examples []operationExample) *RequestBody {
	if !binding.Input.Payload.IsZero() {
		member, target, ok := b.memberAndTarget(binding.Input.Payload)
		if !ok {
			return nil
		}
		mediaType := &MediaType{Schema: b.schemaForMember(member, target)}
		if values := b.bodyExamples(binding.Input.Body, binding.Input.Payload, examples, func(example operationExample) *model.Node {
			return example.Input
		}); len(values) > 0 {
			mediaType.Examples = values
		}
		return &RequestBody{
			Description: b.memberDescription(member, target),
			Required:    member.Required(),
			Content: map[string]*MediaType{
				b.payloadMediaType(member, target): mediaType,
			},
		}
	}
	if len(binding.Input.Body) == 0 {
		return nil
	}
	mediaType := &MediaType{Schema: b.bodySchema(binding.Input.Body)}
	if values := b.bodyExamples(binding.Input.Body, model.ShapeID{}, examples, func(example operationExample) *model.Node {
		return example.Input
	}); len(values) > 0 {
		mediaType.Examples = values
	}
	required := false
	for _, id := range binding.Input.Body {
		if member, ok := b.model.Shape(id); ok && member.Required() {
			required = true
			break
		}
	}
	return &RequestBody{
		Required: required,
		Content: map[string]*MediaType{
			"application/json": mediaType,
		},
	}
}

func (b *builder) bodySchema(members []model.ShapeID) *Schema {
	schema := &Schema{Type: "object", Properties: make(map[string]*Schema)}
	for _, id := range members {
		member, target, ok := b.memberAndTarget(id)
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
	return schema
}

func (b *builder) responses(binding *httpbinding.Binding, examples []operationExample) map[string]*Response {
	responses := make(map[string]*Response)
	success := &Response{
		Description: fmt.Sprintf("%s %d response", binding.Operation.Name, binding.SuccessCode),
		Headers:     b.responseHeaders(binding),
	}
	successExamples := b.bodyExamples(binding.Output.Body, binding.Output.Payload, examples, func(example operationExample) *model.Node {
		return example.Output
	})
	if !binding.Output.Payload.IsZero() {
		member, target, ok := b.memberAndTarget(binding.Output.Payload)
		if ok {
			mediaType := &MediaType{Schema: b.schemaForMember(member, target)}
			if len(successExamples) > 0 {
				mediaType.Examples = successExamples
			}
			success.Content = map[string]*MediaType{
				b.payloadMediaType(member, target): mediaType,
			}
		}
	} else if len(binding.Output.Body) > 0 {
		mediaType := &MediaType{Schema: b.bodySchema(binding.Output.Body)}
		if len(successExamples) > 0 {
			mediaType.Examples = successExamples
		}
		success.Content = map[string]*MediaType{
			"application/json": mediaType,
		}
	}
	responses[strconv.Itoa(binding.SuccessCode)] = success
	if !binding.Output.ResponseCode.IsZero() {
		if member, _, ok := b.memberAndTarget(binding.Output.ResponseCode); ok {
			b.warn(binding.Operation, "@httpResponseCode on `%s` is not represented in OpenAPI; the @http success code is used", member.ID)
		}
	}

	errorExamples := b.errorExamples(examples)
	byStatus := make(map[int][]*model.Shape)
	for _, errorBinding := range binding.Errors {
		shape, ok := b.model.Shape(errorBinding.Shape)
		if !ok {
			continue
		}
		byStatus[errorBinding.Code] = append(byStatus[errorBinding.Code], shape)
	}
	statuses := make([]int, 0, len(byStatus))
	for status := range byStatus {
		statuses = append(statuses, status)
	}
	sort.Ints(statuses)
	for _, status := range statuses {
		shapes := byStatus[status]
		key := strconv.Itoa(status)
		schemas := make([]*Schema, 0, len(shapes))
		descriptions := make([]string, 0, len(shapes))
		hasContent := false
		for _, shape := range shapes {
			schemas = append(schemas, b.schemaForShape(shape))
			if len(shape.Members) > 0 {
				hasContent = true
			}
			if doc := b.effectiveDocumentation(shape); doc != "" {
				descriptions = append(descriptions, doc)
			}
		}
		description := strings.Join(descriptions, "\n\n")
		if description == "" {
			names := make([]string, 0, len(shapes))
			for _, shape := range shapes {
				names = append(names, shape.ID.Name)
			}
			description = fmt.Sprintf("%s %d response", strings.Join(names, ", "), status)
		}
		response := &Response{Description: description}
		if hasContent {
			var schema *Schema
			if len(schemas) == 1 {
				schema = schemas[0]
			} else {
				schema = &Schema{OneOf: schemas}
			}
			mediaType := &MediaType{Schema: schema}
			for _, shape := range shapes {
				for name, example := range errorExamples[shape.ID] {
					if mediaType.Examples == nil {
						mediaType.Examples = make(map[string]*Example)
					}
					mediaType.Examples[name] = example
				}
			}
			response.Content = map[string]*MediaType{
				"application/json": mediaType,
			}
		}
		if existing, exists := responses[key]; exists {
			b.warn(binding.Operation, "success and error responses share HTTP status %d for operation `%s`; their schemas are combined", status, binding.Operation)
			mergeResponseContent(existing, response)
			continue
		}
		responses[key] = response
	}
	return responses
}

func (b *builder) responseHeaders(binding *httpbinding.Binding) map[string]*Header {
	headers := make(map[string]*Header)
	for _, name := range sortedBindingNames(binding.Output.Headers) {
		member, target, ok := b.memberAndTarget(binding.Output.Headers[name])
		if !ok {
			continue
		}
		headers[name] = &Header{
			Description: b.memberDescription(member, target),
			Schema:      b.schemaForMember(member, target),
		}
	}
	if !binding.Output.PrefixHeaders.IsZero() {
		if member, _, ok := b.memberAndTarget(binding.Output.PrefixHeaders); ok {
			b.warn(binding.Operation, "@httpPrefixHeaders on `%s` is not represented in OpenAPI", member.ID)
		}
	}
	if len(headers) == 0 {
		return nil
	}
	return headers
}

func (b *builder) payloadMediaType(member, target *model.Shape) string {
	if mediaType, ok := member.MediaType(); ok && mediaType != "" {
		return mediaType
	}
	if target != nil {
		if mediaType, ok := target.MediaType(); ok && mediaType != "" {
			return mediaType
		}
	}
	if target == nil {
		return "application/json"
	}
	switch target.Type {
	case model.TypeBlob:
		return "application/octet-stream"
	case model.TypeString:
		return "text/plain; charset=utf-8"
	default:
		return "application/json"
	}
}

func (b *builder) memberAndTarget(id model.ShapeID) (*model.Shape, *model.Shape, bool) {
	member, ok := b.model.Shape(id)
	if !ok || !member.IsMember() {
		return nil, nil, false
	}
	target, ok := b.model.Shape(member.Target)
	if !ok {
		return nil, nil, false
	}
	return member, target, true
}

func (b *builder) unstableFeature(shape *model.Shape) string {
	trait, ok := shape.GetTrait(model.TraitUnstable)
	if !ok || trait.Value.Kind != model.NodeObject {
		return ""
	}
	value, _ := trait.Value.GetString("featureId")
	return value
}

func (b *builder) externalDocs(shape *model.Shape) *ExternalDocs {
	trait, ok := shape.GetTrait(model.TraitExternalDocs)
	if !ok || trait.Value.Kind != model.NodeObject {
		return nil
	}
	urls := make(map[string]string, len(trait.Value.Obj))
	for _, entry := range trait.Value.Obj {
		value, _ := trait.Value.Get(entry.Key)
		if url, ok := value.AsString(); ok && url != "" {
			urls[entry.Key] = url
		}
	}
	if len(urls) == 0 {
		return nil
	}
	keys := make([]string, 0, len(urls))
	for key := range urls {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	// Match the upstream converter's preference order, then fall back to the
	// first key in lexical order.
	for _, preferred := range []string{"homepage", "api reference", "user guide", "developer guide", "reference", "guide"} {
		for _, key := range keys {
			if strings.ToLower(key) == preferred {
				return &ExternalDocs{Description: key, URL: urls[key]}
			}
		}
	}
	return &ExternalDocs{Description: keys[0], URL: urls[keys[0]]}
}

func (b *builder) memberDescription(member, target *model.Shape) string {
	if doc := b.memberDocumentation(member); doc != "" {
		return doc
	}
	if target != nil && !b.needsComponent(target) {
		return b.effectiveDocumentation(target)
	}
	return ""
}

func (b *builder) memberDocumentation(member *model.Shape) string {
	if member == nil {
		return ""
	}
	if doc, ok := member.Documentation(); ok && doc != "" {
		return doc
	}
	return member.Doc
}

func (b *builder) effectiveDocumentation(shape *model.Shape) string {
	if shape == nil {
		return ""
	}
	if doc, ok := shape.Documentation(); ok && doc != "" {
		return doc
	}
	if shape.Doc != "" {
		return shape.Doc
	}
	if shape.IsMember() {
		if target, ok := b.model.Shape(shape.Target); ok {
			return b.effectiveDocumentation(target)
		}
	}
	return ""
}

func (b *builder) stringTrait(shape *model.Shape, id model.ShapeID) (string, bool) {
	trait, ok := shape.GetTrait(id)
	if !ok {
		return "", false
	}
	return trait.Value.AsString()
}

func (b *builder) warn(shapeID model.ShapeID, format string, args ...interface{}) {
	loc := idl.SourceLocation{}
	if shape, ok := b.model.Shape(shapeID); ok {
		loc = shape.Source
	}
	b.diagnostics = append(b.diagnostics, codegen.Diagnostic{
		Severity: validation.Warning,
		Message:  fmt.Sprintf(format, args...),
		Path:     loc.Filename,
		Line:     loc.Line,
		Column:   loc.Column,
	})
}

func (b *builder) danger(shape *model.Shape, format string, args ...interface{}) {
	loc := idl.SourceLocation{}
	if shape != nil {
		loc = shape.Source
	}
	b.diagnostics = append(b.diagnostics, codegen.Diagnostic{
		Severity: validation.Danger,
		Message:  fmt.Sprintf(format, args...),
		Path:     loc.Filename,
		Line:     loc.Line,
		Column:   loc.Column,
	})
}

func danger(existing []codegen.Diagnostic, format string, args ...interface{}) []codegen.Diagnostic {
	return append(existing, codegen.Diagnostic{
		Severity: validation.Danger,
		Message:  fmt.Sprintf(format, args...),
	})
}

func sortedBindingNames(values map[string]model.ShapeID) []string {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func uniqueStrings(values []string) []string {
	if len(values) < 2 {
		return values
	}
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func boolPtr(value bool) *bool { return &value }

func setOperation(item *PathItem, method string, operation *Operation) bool {
	switch method {
	case "get":
		if item.Get != nil {
			return false
		}
		item.Get = operation
	case "put":
		if item.Put != nil {
			return false
		}
		item.Put = operation
	case "post":
		if item.Post != nil {
			return false
		}
		item.Post = operation
	case "delete":
		if item.Delete != nil {
			return false
		}
		item.Delete = operation
	case "options":
		if item.Options != nil {
			return false
		}
		item.Options = operation
	case "head":
		if item.Head != nil {
			return false
		}
		item.Head = operation
	case "patch":
		if item.Patch != nil {
			return false
		}
		item.Patch = operation
	case "trace":
		if item.Trace != nil {
			return false
		}
		item.Trace = operation
	default:
		return false
	}
	return true
}

func mergeResponseContent(existing, incoming *Response) {
	if incoming == nil || len(incoming.Content) == 0 {
		return
	}
	if existing.Content == nil {
		existing.Content = incoming.Content
		return
	}
	existingJSON, ok := existing.Content["application/json"]
	if !ok || existingJSON == nil || existingJSON.Schema == nil {
		existing.Content["application/json"] = incoming.Content["application/json"]
		return
	}
	incomingJSON, ok := incoming.Content["application/json"]
	if !ok || incomingJSON == nil || incomingJSON.Schema == nil {
		return
	}
	schemas := []*Schema{existingJSON.Schema}
	if len(existingJSON.Schema.OneOf) > 0 {
		schemas = existingJSON.Schema.OneOf
	}
	if len(incomingJSON.Schema.OneOf) > 0 {
		schemas = append(schemas, incomingJSON.Schema.OneOf...)
	} else {
		schemas = append(schemas, incomingJSON.Schema)
	}
	existingJSON.Schema = &Schema{OneOf: schemas}
	for name, example := range incomingJSON.Examples {
		if existingJSON.Examples == nil {
			existingJSON.Examples = make(map[string]*Example)
		}
		existingJSON.Examples[name] = example
	}
}
