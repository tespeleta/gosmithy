// Package httpbinding builds the HTTP binding index for a service that uses
// Smithy's HTTP traits.
//
// The index is model knowledge, not generator knowledge: it validates the
// @http, @httpLabel, @httpQuery, @httpQueryParams, @httpHeader,
// @httpPrefixHeaders, @httpPayload, @httpResponseCode, and @httpError traits and
// exposes the result in a form that a code generator can consume.
package httpbinding

import (
	"fmt"
	"sort"
	"strings"

	"github.com/tespeleta/gosmithy/idl"
	"github.com/tespeleta/gosmithy/model"
)

// Event IDs emitted by this package.
const (
	EventMissingHTTP        = "HttpBinding.MissingHttp"
	EventInvalidHTTP        = "HttpBinding.InvalidHttp"
	EventInvalidURI         = "HttpBinding.InvalidUri"
	EventInvalidLabel       = "HttpBinding.InvalidLabel"
	EventDuplicateBinding   = "HttpBinding.DuplicateBinding"
	EventInvalidTarget      = "HttpBinding.InvalidTarget"
	EventInvalidPayload     = "HttpBinding.InvalidPayload"
	EventInvalidResponse    = "HttpBinding.InvalidResponseCode"
	EventInvalidError       = "HttpBinding.InvalidError"
	EventRouteConflict      = "HttpBinding.RouteConflict"
	EventInvalidService     = "HttpBinding.InvalidService"
	EventUnsupportedFeature = "HttpBinding.UnsupportedFeature"
)

// Segment is one segment of an HTTP URI pattern.
type Segment struct {
	// Literal is the literal text of the segment when IsLabel is false.
	Literal string
	// Label is the label name when IsLabel is true.
	Label string
	// IsLabel reports whether the segment is a label.
	IsLabel bool
}

// String formats the segment in URI form.
func (s Segment) String() string {
	if s.IsLabel {
		return "{" + s.Label + "}"
	}
	return s.Literal
}

// MessageBindings describes the HTTP bindings of one input or output
// structure. Body lists the members that are serialized in the JSON body, in
// declaration order.
type MessageBindings struct {
	Labels        map[string]model.ShapeID
	Query         map[string]model.ShapeID
	QueryParams   model.ShapeID
	Headers       map[string]model.ShapeID
	PrefixHeader  string
	PrefixHeaders model.ShapeID
	Payload       model.ShapeID
	ResponseCode  model.ShapeID
	Body          []model.ShapeID
}

// IsBound reports whether the member is bound to a non-body location.
func (b MessageBindings) IsBound(member model.ShapeID) bool {
	for _, id := range b.Labels {
		if id == member {
			return true
		}
	}
	for _, id := range b.Query {
		if id == member {
			return true
		}
	}
	for _, id := range b.Headers {
		if id == member {
			return true
		}
	}
	if b.QueryParams == member || b.PrefixHeaders == member ||
		b.Payload == member || b.ResponseCode == member {
		return true
	}
	return false
}

// ErrorBinding describes one modeled error of an operation.
type ErrorBinding struct {
	Shape model.ShapeID
	Code  int
	Fault string
}

// Binding is the HTTP binding of one operation.
type Binding struct {
	Operation   model.ShapeID
	Method      string
	URI         string
	SuccessCode int
	Segments    []Segment
	Input       MessageBindings
	Output      MessageBindings
	Errors      []ErrorBinding
}

// HasBody reports whether the input binding has JSON body members.
func (b *Binding) HasBody() bool { return len(b.Input.Body) > 0 }

// HasPayload reports whether the input binding has an HTTP payload.
func (b *Binding) HasPayload() bool { return !b.Input.Payload.IsZero() }

// Index builds and validates the HTTP binding index for a service. The
// returned diagnostics are DANGER events; code generation must not proceed
// when any are present.
func Index(m *model.Model, serviceID model.ShapeID) ([]*Binding, []model.Diagnostic) {
	service, ok := m.Shape(serviceID)
	if !ok {
		return nil, []model.Diagnostic{diagnostic(EventInvalidService, idl.SourceLocation{}, serviceID,
			"Service `%s` is not found in the model", serviceID)}
	}
	if service.Type != model.TypeService {
		return nil, []model.Diagnostic{diagnostic(EventInvalidService, service.Source, serviceID,
			"`%s` is a %s shape, not a service", serviceID, service.Type)}
	}

	closure, ok := m.ServiceClosure(serviceID)
	if !ok {
		return nil, []model.Diagnostic{diagnostic(EventInvalidService, service.Source, serviceID,
			"Cannot compute the closure of service `%s`", serviceID)}
	}

	var operations []*model.Shape
	for _, id := range closure {
		shape, ok := m.Shape(id)
		if !ok || shape.Type != model.TypeOperation {
			continue
		}
		operations = append(operations, shape)
	}
	sort.Slice(operations, func(i, j int) bool { return operations[i].ID.String() < operations[j].ID.String() })

	var bindings []*Binding
	var diagnostics []model.Diagnostic
	for _, op := range operations {
		binding, events := buildBinding(m, service, op)
		diagnostics = append(diagnostics, events...)
		if binding != nil {
			bindings = append(bindings, binding)
		}
	}

	// Detect overlapping routes after the whole service is indexed so the
	// diagnostic points at the operation declaration rather than a member.
	for i := 1; i < len(bindings); i++ {
		for j := 0; j < i; j++ {
			if !routesOverlap(bindings[j], bindings[i]) {
				continue
			}
			op, _ := m.Shape(bindings[i].Operation)
			loc := idl.SourceLocation{}
			if op != nil {
				loc = op.Source
			}
			diagnostics = append(diagnostics, diagnostic(EventRouteConflict, loc, bindings[i].Operation,
				"HTTP route `%s %s` of operation `%s` overlaps with `%s %s` of operation `%s`",
				bindings[i].Method, bindings[i].URI, bindings[i].Operation,
				bindings[j].Method, bindings[j].URI, bindings[j].Operation))
			break
		}
	}

	model.SortDiagnostics(diagnostics)
	return bindings, diagnostics
}

func buildBinding(m *model.Model, service, op *model.Shape) (*Binding, []model.Diagnostic) {
	var diagnostics []model.Diagnostic

	method, uri, code, ok := op.HTTP()
	if !ok {
		return nil, []model.Diagnostic{diagnostic(EventMissingHTTP, op.Source, op.ID,
			"Operation `%s` does not have an @http trait", op.ID)}
	}
	method = strings.ToUpper(strings.TrimSpace(method))
	uri = strings.TrimSpace(uri)
	if method == "" || !isMethodToken(method) {
		diagnostics = append(diagnostics, diagnostic(EventInvalidHTTP, op.Source, op.ID,
			"Operation `%s` has an invalid @http method %q", op.ID, method))
	}
	if uri == "" || !strings.HasPrefix(uri, "/") {
		diagnostics = append(diagnostics, diagnostic(EventInvalidURI, op.Source, op.ID,
			"Operation `%s` has an invalid @http uri %q: the URI must start with '/'", op.ID, uri))
	}
	segments, err := parseURI(uri)
	if err != nil {
		diagnostics = append(diagnostics, diagnostic(EventInvalidURI, op.Source, op.ID,
			"Operation `%s` has an invalid @http uri %q: %v", op.ID, uri, err))
	}
	successCode := 200
	if code != 0 {
		successCode = code
	}
	if successCode < 100 || successCode > 599 {
		diagnostics = append(diagnostics, diagnostic(EventInvalidHTTP, op.Source, op.ID,
			"Operation `%s` has an invalid @http code %d: want a status between 100 and 599", op.ID, successCode))
	}

	binding := &Binding{
		Operation:   op.ID,
		Method:      method,
		URI:         uri,
		SuccessCode: successCode,
		Segments:    segments,
	}

	inputShape, inputOK := m.Shape(op.Input)
	outputShape, outputOK := m.Shape(op.Output)
	if !inputOK {
		diagnostics = append(diagnostics, diagnostic(EventInvalidHTTP, op.Source, op.ID,
			"Operation `%s` input `%s` is not found in the model", op.ID, op.Input))
	}
	if !outputOK {
		diagnostics = append(diagnostics, diagnostic(EventInvalidHTTP, op.Source, op.ID,
			"Operation `%s` output `%s` is not found in the model", op.ID, op.Output))
	}

	if inputOK {
		binding.Input, diagnostics = bindMessage(m, op, inputShape, true, diagnostics)
	}
	if outputOK {
		binding.Output, diagnostics = bindMessage(m, op, outputShape, false, diagnostics)
	}

	// Every URI label must be backed by exactly one required @httpLabel input
	// member, and every @httpLabel member must appear in the URI.
	uriLabels := make(map[string]bool)
	for _, segment := range segments {
		if !segment.IsLabel {
			continue
		}
		if uriLabels[segment.Label] {
			diagnostics = append(diagnostics, diagnostic(EventInvalidURI, op.Source, op.ID,
				"Operation `%s` URI `%s` contains the label `%s` more than once", op.ID, uri, segment.Label))
		}
		uriLabels[segment.Label] = true
	}
	if inputOK {
		for label := range uriLabels {
			memberID, ok := binding.Input.Labels[label]
			if !ok {
				diagnostics = append(diagnostics, diagnostic(EventInvalidLabel, op.Source, op.ID,
					"Operation `%s` URI label `%s` is not bound to an input member with @httpLabel", op.ID, label))
				continue
			}
			if !memberIsRequired(m, memberID) {
				diagnostics = append(diagnostics, diagnostic(EventInvalidLabel, memberSource(m, memberID), memberID,
					"Member `%s` is bound to the URI label `%s`, so it must be marked @required", memberID, label))
			}
		}
		for label, memberID := range binding.Input.Labels {
			if !uriLabels[label] {
				diagnostics = append(diagnostics, diagnostic(EventInvalidLabel, memberSource(m, memberID), memberID,
					"Member `%s` has @httpLabel but the URI `%s` of operation `%s` has no `{%s}` label",
					memberID, uri, op.ID, label))
			}
		}
	}

	binding.Errors = buildErrors(m, service, op, &diagnostics)
	return binding, diagnostics
}

func bindMessage(m *model.Model, op, shape *model.Shape, input bool, diagnostics []model.Diagnostic) (MessageBindings, []model.Diagnostic) {
	bindings := MessageBindings{
		Labels:  make(map[string]model.ShapeID),
		Query:   make(map[string]model.ShapeID),
		Headers: make(map[string]model.ShapeID),
	}
	if shape == nil || shape.IsUnit() || shape.Type != model.TypeStructure {
		// Unit (and invalid non-structure I/O) has no HTTP-bound members; the
		// structural validators report non-structure operation I/O.
		return bindings, diagnostics
	}

	for _, member := range shape.Members {
		location := ""
		setLocation := func(name string) {
			if location != "" {
				diagnostics = append(diagnostics, diagnostic(EventDuplicateBinding, member.Source, member.ID,
					"Member `%s` is bound to more than one HTTP location: @%s and @%s", member.ID, location, name))
				return
			}
			location = name
		}

		if member.HTTPLabel() {
			setLocation("httpLabel")
			if !input {
				diagnostics = append(diagnostics, diagnostic(EventInvalidLabel, member.Source, member.ID,
					"@httpLabel is only valid on operation input members, but `%s` is an output member", member.ID))
			} else {
				name := member.ID.Member
				bindings.Labels[name] = member.ID
				if err := checkLabelTarget(m, member); err != "" {
					diagnostics = append(diagnostics, diagnostic(EventInvalidTarget, member.Source, member.ID,
						"@httpLabel member `%s` %s", member.ID, err))
				}
			}
		}
		if name, ok := member.HTTPQuery(); ok {
			setLocation("httpQuery")
			if !input {
				diagnostics = append(diagnostics, diagnostic(EventInvalidTarget, member.Source, member.ID,
					"@httpQuery is only valid on operation input members, but `%s` is an output member", member.ID))
			} else if strings.TrimSpace(name) == "" {
				diagnostics = append(diagnostics, diagnostic(EventInvalidTarget, member.Source, member.ID,
					"@httpQuery on member `%s` must not be empty", member.ID))
			} else if previous, exists := bindings.Query[name]; exists {
				diagnostics = append(diagnostics, diagnostic(EventDuplicateBinding, member.Source, member.ID,
					"@httpQuery name %q on `%s` is already used by `%s`", name, member.ID, previous))
			} else {
				bindings.Query[name] = member.ID
				if err := checkQueryTarget(m, member); err != "" {
					diagnostics = append(diagnostics, diagnostic(EventInvalidTarget, member.Source, member.ID,
						"@httpQuery member `%s` %s", member.ID, err))
				}
			}
		}
		if member.HTTPQueryParams() {
			setLocation("httpQueryParams")
			if !input {
				diagnostics = append(diagnostics, diagnostic(EventInvalidTarget, member.Source, member.ID,
					"@httpQueryParams is only valid on operation input members, but `%s` is an output member", member.ID))
			} else if !bindings.QueryParams.IsZero() {
				diagnostics = append(diagnostics, diagnostic(EventDuplicateBinding, member.Source, member.ID,
					"Only one member may use @httpQueryParams, but `%s` is also bound", bindings.QueryParams))
			} else {
				bindings.QueryParams = member.ID
				if err := checkQueryParamsTarget(m, member); err != "" {
					diagnostics = append(diagnostics, diagnostic(EventInvalidTarget, member.Source, member.ID,
						"@httpQueryParams member `%s` %s", member.ID, err))
				}
			}
		}
		if name, ok := member.HTTPHeader(); ok {
			setLocation("httpHeader")
			if strings.TrimSpace(name) == "" {
				diagnostics = append(diagnostics, diagnostic(EventInvalidTarget, member.Source, member.ID,
					"@httpHeader on member `%s` must not be empty", member.ID))
			} else if previous, exists := headerCaseInsensitive(bindings.Headers, name); exists {
				diagnostics = append(diagnostics, diagnostic(EventDuplicateBinding, member.Source, member.ID,
					"@httpHeader name %q on `%s` is already used by `%s` (header names are case-insensitive)", name, member.ID, previous))
			} else {
				bindings.Headers[name] = member.ID
				if err := checkHeaderTarget(m, member); err != "" {
					diagnostics = append(diagnostics, diagnostic(EventInvalidTarget, member.Source, member.ID,
						"@httpHeader member `%s` %s", member.ID, err))
				}
			}
		}
		if name, ok := member.HTTPPrefixHeaders(); ok {
			setLocation("httpPrefixHeaders")
			if strings.TrimSpace(name) == "" {
				diagnostics = append(diagnostics, diagnostic(EventInvalidTarget, member.Source, member.ID,
					"@httpPrefixHeaders on member `%s` must not be empty", member.ID))
			} else if !bindings.PrefixHeaders.IsZero() {
				diagnostics = append(diagnostics, diagnostic(EventDuplicateBinding, member.Source, member.ID,
					"Only one member may use @httpPrefixHeaders, but `%s` is also bound", bindings.PrefixHeaders))
			} else {
				bindings.PrefixHeader = name
				bindings.PrefixHeaders = member.ID
				if err := checkPrefixHeadersTarget(m, member); err != "" {
					diagnostics = append(diagnostics, diagnostic(EventInvalidTarget, member.Source, member.ID,
						"@httpPrefixHeaders member `%s` %s", member.ID, err))
				}
			}
		}
		if member.HTTPPayload() {
			setLocation("httpPayload")
			if !bindings.Payload.IsZero() {
				diagnostics = append(diagnostics, diagnostic(EventInvalidPayload, member.Source, member.ID,
					"Only one member may use @httpPayload, but `%s` is also bound", bindings.Payload))
			} else {
				bindings.Payload = member.ID
				if err := checkPayloadTarget(m, member); err != "" {
					diagnostics = append(diagnostics, diagnostic(EventInvalidPayload, member.Source, member.ID,
						"@httpPayload member `%s` %s", member.ID, err))
				}
			}
		}
		if member.HTTPResponseCode() {
			setLocation("httpResponseCode")
			if input {
				diagnostics = append(diagnostics, diagnostic(EventInvalidResponse, member.Source, member.ID,
					"@httpResponseCode is only valid on operation output members, but `%s` is an input member", member.ID))
			} else {
				bindings.ResponseCode = member.ID
				if err := checkResponseCodeTarget(m, member); err != "" {
					diagnostics = append(diagnostics, diagnostic(EventInvalidResponse, member.Source, member.ID,
						"@httpResponseCode member `%s` %s", member.ID, err))
				}
			}
		}

		if location == "" {
			bindings.Body = append(bindings.Body, member.ID)
		}
	}

	if !bindings.Payload.IsZero() && len(bindings.Body) > 0 {
		diagnostics = append(diagnostics, diagnostic(EventInvalidPayload, shape.Source, shape.ID,
			"Structure `%s` has both an @httpPayload member and unbound body members; the payload is the entire body", shape.ID))
	}

	return bindings, diagnostics
}

func buildErrors(m *model.Model, service, op *model.Shape, diagnostics *[]model.Diagnostic) []ErrorBinding {
	var out []ErrorBinding
	for _, id := range m.ServiceOperationErrors(service.ID, op.ID) {
		shape, ok := m.Shape(id)
		if !ok {
			*diagnostics = append(*diagnostics, diagnostic(EventInvalidError, op.Source, op.ID,
				"Operation `%s` error `%s` is not found in the model", op.ID, id))
			continue
		}
		if !shape.IsErrorStructure() {
			*diagnostics = append(*diagnostics, diagnostic(EventInvalidError, shape.Source, shape.ID,
				"Error `%s` of operation `%s` must be a structure with the @error trait", shape.ID, op.ID))
			continue
		}
		fault, _ := shape.ErrorType()
		status := 400
		if fault == "server" {
			status = 500
		}
		if code, ok := shape.HTTPError(); ok {
			if code < 100 || code > 599 {
				*diagnostics = append(*diagnostics, diagnostic(EventInvalidError, shape.Source, shape.ID,
					"Error `%s` has an invalid @httpError status %d: want a status between 100 and 599", shape.ID, code))
			} else {
				status = code
			}
		}
		out = append(out, ErrorBinding{Shape: shape.ID, Code: status, Fault: fault})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Shape.String() < out[j].Shape.String() })
	return out
}

// --- URI parsing and route matching ------------------------------------------

func parseURI(raw string) ([]Segment, error) {
	if !strings.HasPrefix(raw, "/") {
		return nil, fmt.Errorf("the URI must start with '/'")
	}
	path := strings.TrimPrefix(raw, "/")
	if path == "" {
		return nil, nil
	}
	path = strings.TrimSuffix(path, "/")
	if path == "" {
		return nil, nil
	}
	parts := strings.Split(path, "/")
	segments := make([]Segment, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			return nil, fmt.Errorf("the URI contains an empty path segment")
		}
		if strings.HasPrefix(part, "{") {
			if !strings.HasSuffix(part, "}") {
				return nil, fmt.Errorf("the label segment %q is not closed with '}'", part)
			}
			name := part[1 : len(part)-1]
			if strings.HasSuffix(name, "+") {
				return nil, fmt.Errorf("greedy label {%s+} is not supported by the httpJson protocol", strings.TrimSuffix(name, "+"))
			}
			if name == "" || strings.ContainsAny(name, "{}") {
				return nil, fmt.Errorf("the label segment %q has an invalid name", part)
			}
			segments = append(segments, Segment{Label: name, IsLabel: true})
			continue
		}
		if strings.ContainsAny(part, "{}") {
			return nil, fmt.Errorf("labels must span an entire path segment, but found %q", part)
		}
		segments = append(segments, Segment{Literal: part})
	}
	return segments, nil
}

func routesOverlap(a, b *Binding) bool {
	if a.Method != b.Method || len(a.Segments) != len(b.Segments) {
		return false
	}
	for i := range a.Segments {
		left, right := a.Segments[i], b.Segments[i]
		if left.IsLabel || right.IsLabel {
			continue
		}
		if left.Literal != right.Literal {
			return false
		}
	}
	return true
}

// --- target checks ------------------------------------------------------------

func isMethodToken(method string) bool {
	if method == "" {
		return false
	}
	for i := 0; i < len(method); i++ {
		c := method[i]
		switch {
		case c >= 'A' && c <= 'Z':
		case c >= '0' && c <= '9':
		case c == '!' || c == '#' || c == '$' || c == '%' || c == '&' ||
			c == '\'' || c == '*' || c == '+' || c == '-' || c == '.' ||
			c == '^' || c == '_' || c == '`' || c == '|' || c == '~':
		default:
			return false
		}
	}
	return true
}

// Simple reports whether a shape can be serialized as an HTTP label, query, or
// header scalar.
func Simple(shape *model.Shape) bool {
	if shape == nil {
		return false
	}
	switch shape.Type {
	case model.TypeString, model.TypeBoolean, model.TypeByte, model.TypeShort,
		model.TypeInteger, model.TypeLong, model.TypeFloat, model.TypeDouble,
		model.TypeBigInteger, model.TypeBigDecimal, model.TypeTimestamp,
		model.TypeEnum, model.TypeIntEnum:
		return true
	default:
		return false
	}
}

func memberTarget(m *model.Model, member *model.Shape) (*model.Shape, string) {
	target, ok := m.Shape(member.Target)
	if !ok {
		return nil, fmt.Sprintf("targets `%s`, which is not found in the model", member.Target)
	}
	return target, ""
}

func checkLabelTarget(m *model.Model, member *model.Shape) string {
	target, reason := memberTarget(m, member)
	if reason != "" {
		return reason
	}
	if !Simple(target) {
		return fmt.Sprintf("targets `%s`, which cannot be used as an HTTP label", target.ID)
	}
	return ""
}

func checkQueryTarget(m *model.Model, member *model.Shape) string {
	target, reason := memberTarget(m, member)
	if reason != "" {
		return reason
	}
	if target.Type == model.TypeList || target.Type == model.TypeSet {
		elem, ok := target.GetMember("member")
		if !ok {
			return "targets a collection without a member"
		}
		elemTarget, reason := memberTarget(m, elem)
		if reason != "" {
			return reason
		}
		if !Simple(elemTarget) {
			return fmt.Sprintf("targets a collection of `%s`, which cannot be used in a query string", elemTarget.ID)
		}
		return ""
	}
	if !Simple(target) {
		return fmt.Sprintf("targets `%s`, which cannot be used in a query string", target.ID)
	}
	return ""
}

func checkHeaderTarget(m *model.Model, member *model.Shape) string {
	// Header binding accepts either a member whose target is a scalar or a
	// member that is itself a list of scalars (the @httpHeader trait selector
	// is applied to the containing structure in Smithy, so inspect the member
	// target and its member in turn).
	target, reason := memberTarget(m, member)
	if reason != "" {
		return reason
	}
	if target.Type == model.TypeList || target.Type == model.TypeSet {
		elem, ok := target.GetMember("member")
		if !ok {
			return "targets a collection without a member"
		}
		elemTarget, reason := memberTarget(m, elem)
		if reason != "" {
			return reason
		}
		if !Simple(elemTarget) {
			return fmt.Sprintf("targets a collection of `%s`, which cannot be used in a header", elemTarget.ID)
		}
		return ""
	}
	if !Simple(target) {
		return fmt.Sprintf("targets `%s`, which cannot be used in a header", target.ID)
	}
	return ""
}

func checkQueryParamsTarget(m *model.Model, member *model.Shape) string {
	target, reason := memberTarget(m, member)
	if reason != "" {
		return reason
	}
	if target.Type != model.TypeMap {
		return fmt.Sprintf("targets `%s`, but @httpQueryParams requires a map", target.ID)
	}
	key, ok := target.GetMember("key")
	if !ok {
		return "targets a map without a key member"
	}
	keyTarget, reason := memberTarget(m, key)
	if reason != "" {
		return reason
	}
	if keyTarget.Type != model.TypeString {
		return fmt.Sprintf("targets a map with a %s key, but @httpQueryParams requires string keys", keyTarget.Type)
	}
	value, ok := target.GetMember("value")
	if !ok {
		return "targets a map without a value member"
	}
	valueTarget, reason := memberTarget(m, value)
	if reason != "" {
		return reason
	}
	if valueTarget.Type == model.TypeList || valueTarget.Type == model.TypeSet {
		elem, ok := valueTarget.GetMember("member")
		if !ok {
			return "targets a map of collections without a member"
		}
		elemTarget, reason := memberTarget(m, elem)
		if reason != "" {
			return reason
		}
		if elemTarget.Type != model.TypeString {
			return fmt.Sprintf("targets a map of collections of `%s`, but @httpQueryParams list values must be strings", elemTarget.ID)
		}
		return ""
	}
	if valueTarget.Type != model.TypeString {
		return fmt.Sprintf("targets a map of `%s`, but @httpQueryParams values must be strings or string lists", valueTarget.Type)
	}
	return ""
}

func checkPrefixHeadersTarget(m *model.Model, member *model.Shape) string {
	target, reason := memberTarget(m, member)
	if reason != "" {
		return reason
	}
	if target.Type != model.TypeMap {
		return fmt.Sprintf("targets `%s`, but @httpPrefixHeaders requires a map", target.ID)
	}
	key, ok := target.GetMember("key")
	if !ok {
		return "targets a map without a key member"
	}
	keyTarget, reason := memberTarget(m, key)
	if reason != "" {
		return reason
	}
	if keyTarget.Type != model.TypeString {
		return fmt.Sprintf("targets a map with a %s key, but @httpPrefixHeaders requires string keys", keyTarget.Type)
	}
	value, ok := target.GetMember("value")
	if !ok {
		return "targets a map without a value member"
	}
	valueTarget, reason := memberTarget(m, value)
	if reason != "" {
		return reason
	}
	if valueTarget.Type != model.TypeString {
		return fmt.Sprintf("targets a map of `%s`, but @httpPrefixHeaders values must be strings", valueTarget.Type)
	}
	return ""
}

func checkPayloadTarget(m *model.Model, member *model.Shape) string {
	target, reason := memberTarget(m, member)
	if reason != "" {
		return reason
	}
	switch target.Type {
	case model.TypeOperation, model.TypeResource, model.TypeService, model.TypeMember:
		return fmt.Sprintf("targets `%s`, which cannot be used as an HTTP payload", target.ID)
	}
	return ""
}

func checkResponseCodeTarget(m *model.Model, member *model.Shape) string {
	target, reason := memberTarget(m, member)
	if reason != "" {
		return reason
	}
	switch target.Type {
	case model.TypeByte, model.TypeShort, model.TypeInteger, model.TypeLong:
		return ""
	default:
		return fmt.Sprintf("targets `%s`, but @httpResponseCode requires an integer shape", target.ID)
	}
}

func headerCaseInsensitive(headers map[string]model.ShapeID, name string) (model.ShapeID, bool) {
	for existing, id := range headers {
		if strings.EqualFold(existing, name) {
			return id, true
		}
	}
	return model.ShapeID{}, false
}

func memberIsRequired(m *model.Model, id model.ShapeID) bool {
	shape, ok := m.Shape(id)
	return ok && shape.Required()
}

func memberSource(m *model.Model, id model.ShapeID) idl.SourceLocation {
	if shape, ok := m.Shape(id); ok {
		return shape.Source
	}
	return idl.SourceLocation{}
}

func diagnostic(eventID string, loc idl.SourceLocation, shapeID model.ShapeID, format string, args ...any) model.Diagnostic {
	return model.Diagnostic{
		ID:       eventID,
		Severity: model.SeverityDanger,
		Message:  fmt.Sprintf(format, args...),
		ShapeID:  shapeID,
		Source:   loc,
	}
}
