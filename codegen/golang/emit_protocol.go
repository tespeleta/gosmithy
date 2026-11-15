package golang

import (
	"fmt"
	"sort"
	"strings"

	"github.com/tespeleta/gosmithy/model"
	"github.com/tespeleta/gosmithy/model/httpbinding"
	"github.com/tespeleta/gosmithy/protocol/httpjson"
	"github.com/tespeleta/gosmithy/validation"
)

// This file generates the protocol-facing part of the Go target: the service
// interface, the httpJson protocol helpers, the client, and the server.

func (g *generator) checkServiceProtocol() {
	if g.protocol == nil {
		return
	}
	for _, traitID := range g.service.Traits.SortedIDs() {
		if traitID == g.protocol.ID {
			continue
		}
		def, ok := g.m.Shape(traitID)
		if !ok || !def.HasTrait(model.TraitProtocolDef) {
			continue
		}
		g.diagnosticAt(validation.Danger, g.service.Source,
			"Service `%s` uses protocol `%s`, but %s was selected", g.service.ID, traitID, g.protocol.ID)
	}
}

// --- service interface --------------------------------------------------------

func (g *generator) emitServiceFile() ([]byte, error) {
	doc := fmt.Sprintf("Package %s contains the generated client and server for the Smithy service %s using the %s protocol.",
		g.rootPackage, g.service.ID, httpjson.TraitID)
	w := newFileWriter(g.rootPackage, doc)
	if len(g.bindings) > 0 {
		w.AddImport("context", "")
		w.AddImport(g.module+"/types", "")
	}

	w.Line("// Service is implemented by the application to handle %s operations.", g.service.ID)
	w.Line("type Service interface {")
	w.In()
	for _, bundle := range g.bindings {
		op, ok := g.m.Shape(bundle.Operation)
		if !ok || op.Type != model.TypeOperation {
			continue
		}
		input, _ := g.m.Shape(op.Input)
		output, _ := g.m.Shape(op.Output)
		inName := g.typeNameFor(input)
		outName := g.typeNameFor(output)
		w.Line("%s(ctx context.Context, input *types.%s) (*types.%s, error)", g.operationGoName(op), inName, outName)
	}
	w.Out()
	w.Line("}")
	return w.Render()
}

// --- protocol helpers ---------------------------------------------------------

func (g *generator) emitProtocolFile() ([]byte, error) {
	doc := fmt.Sprintf("Protocol helpers for %s, the HTTP+JSON protocol used by the generated client and server.", httpjson.TraitID)
	w := newFileWriter(g.rootPackage, doc)
	w.AddImport("encoding/json", "")
	w.AddImport("net/http", "")
	w.AddImport("strings", "")
	w.AddImport("time", "")
	w.AddImport(g.module+"/internal/smithy", "")

	w.Line("const (")
	w.In()
	w.Line("protocolName = %s", quoted(httpjson.TraitID))
	w.Line("contentTypeJSON = \"application/json\"")
	w.Line("defaultMaxBodyBytes = 10 << 20")
	w.Out()
	w.Line(")")
	w.Line("")

	w.Line("type protocolErrorBody struct {")
	w.In()
	w.Line("Code string `json:\"code\"`")
	w.Line("Message string `json:\"message\"`")
	w.Line("Errors []protocolErrorDetail `json:\"errors,omitempty\"`")
	w.Out()
	w.Line("}")
	w.Line("")
	w.Line("type protocolErrorDetail struct {")
	w.In()
	w.Line("Path string `json:\"path\"`")
	w.Line("Message string `json:\"message\"`")
	w.Out()
	w.Line("}")
	w.Line("")

	w.Line("// writeJSON writes value as a JSON response.")
	w.Line("func writeJSON(w http.ResponseWriter, status int, value any) {")
	w.In()
	w.Line("data, err := json.Marshal(value)")
	w.Line("if err != nil {")
	w.In()
	w.Line("http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)")
	w.Line("return")
	w.Out()
	w.Line("}")
	w.Line("w.Header().Set(\"Content-Type\", contentTypeJSON)")
	w.Line("w.WriteHeader(status)")
	w.Line("_, _ = w.Write(data)")
	w.Out()
	w.Line("}")
	w.Line("")

	w.Line("// writeProtocolError writes a protocol-level error envelope.")
	w.Line("func writeProtocolError(w http.ResponseWriter, status int, code, message string) {")
	w.In()
	w.Line("writeJSON(w, status, protocolErrorBody{Code: code, Message: message})")
	w.Out()
	w.Line("}")
	w.Line("")

	w.Line("// writeValidationProtocolError maps a validation failure to a 400 ValidationException.")
	w.Line("func writeValidationProtocolError(w http.ResponseWriter, err error) {")
	w.In()
	w.Line("details := make([]protocolErrorDetail, 0)")
	w.Line("for _, item := range smithy.ValidationErrorList(err) {")
	w.In()
	w.Line("details = append(details, protocolErrorDetail{Path: item.Path, Message: item.Message})")
	w.Out()
	w.Line("}")
	w.Line("writeJSON(w, http.StatusBadRequest, protocolErrorBody{")
	w.In()
	w.Line("Code: \"ValidationException\",")
	w.Line("Message: \"validation failed\",")
	w.Line("Errors: details,")
	w.Out()
	w.Line("})")
	w.Out()
	w.Line("}")
	w.Line("")

	w.Line("// writeModeledError writes a modeled error, adding the protocol code and, when")
	w.Line("// the error does not provide one, its message.")
	w.Line("func writeModeledError(w http.ResponseWriter, status int, code string, err error, payload any) {")
	w.In()
	w.Line("fields := make(map[string]json.RawMessage)")
	w.Line("if payload != nil {")
	w.In()
	w.Line("if data, marshalErr := json.Marshal(payload); marshalErr == nil {")
	w.In()
	w.Line("var object map[string]json.RawMessage")
	w.Line("if json.Unmarshal(data, &object) == nil {")
	w.In()
	w.Line("fields = object")
	w.Out()
	w.Line("}")
	w.Out()
	w.Line("}")
	w.Out()
	w.Line("}")
	w.Line("if encoded, marshalErr := json.Marshal(code); marshalErr == nil {")
	w.In()
	w.Line("fields[\"code\"] = encoded")
	w.Out()
	w.Line("}")
	w.Line("if _, exists := fields[\"message\"]; !exists && err != nil && err.Error() != \"\" {")
	w.In()
	w.Line("if encoded, marshalErr := json.Marshal(err.Error()); marshalErr == nil {")
	w.In()
	w.Line("fields[\"message\"] = encoded")
	w.Out()
	w.Line("}")
	w.Out()
	w.Line("}")
	w.Line("data, marshalErr := json.Marshal(fields)")
	w.Line("if marshalErr != nil {")
	w.In()
	w.Line("writeProtocolError(w, http.StatusInternalServerError, \"InternalFailure\", \"internal server error\")")
	w.Line("return")
	w.Out()
	w.Line("}")
	w.Line("w.Header().Set(\"Content-Type\", contentTypeJSON)")
	w.Line("w.WriteHeader(status)")
	w.Line("_, _ = w.Write(data)")
	w.Out()
	w.Line("}")
	w.Line("")

	w.Line("// formatTimestamp formats a timestamp for a label, query parameter, or header.")
	w.Line("func formatTimestamp(value time.Time, format string) string {")
	w.In()
	w.Line("text, err := smithy.FormatTimestamp(value, format)")
	w.Line("if err != nil {")
	w.In()
	w.Line("return \"\"")
	w.Out()
	w.Line("}")
	w.Line("return text")
	w.Out()
	w.Line("}")
	w.Line("")

	w.Line("// joinEndpoint combines the client endpoint, an optional base path, and an operation path.")
	w.Line("func joinEndpoint(endpoint, basePath, path string) string {")
	w.In()
	w.Line("if basePath != \"\" && !strings.HasPrefix(basePath, \"/\") {")
	w.In()
	w.Line("basePath = \"/\" + basePath")
	w.Out()
	w.Line("}")
	w.Line("return strings.TrimRight(endpoint, \"/\") + strings.TrimRight(basePath, \"/\") + path")
	w.Out()
	w.Line("}")
	w.Line("")

	w.Line("// cleanBasePath normalizes a base path to a leading slash and no trailing slash.")
	w.Line("func cleanBasePath(path string) string {")
	w.In()
	w.Line("if path == \"\" {")
	w.In()
	w.Line("return \"\"")
	w.Out()
	w.Line("}")
	w.Line("if !strings.HasPrefix(path, \"/\") {")
	w.In()
	w.Line("path = \"/\" + path")
	w.Out()
	w.Line("}")
	w.Line("return strings.TrimRight(path, \"/\")")
	w.Out()
	w.Line("}")
	return w.Render()
}

// --- client -------------------------------------------------------------------

func (g *generator) emitClientFile() ([]byte, error) {
	g.localCounter = 0
	doc := fmt.Sprintf("Client for the %s protocol.", httpjson.TraitID)
	w := newFileWriter(g.rootPackage, doc)
	w.AddImport("net/http", "")
	if len(g.bindings) > 0 {
		w.AddImport("context", "")
		w.AddImport("encoding/json", "")
		w.AddImport("fmt", "")
		w.AddImport("io", "")
		w.AddImport("net/url", "")
		w.AddImport("strings", "")
		w.AddImport(g.module+"/internal/smithy", "")
		w.AddImport(g.module+"/types", "")
	}

	w.Line("// Options controls one client call.")
	w.Line("type Options struct {")
	w.In()
	w.Line("// HTTPClient sends requests. NewClient defaults to http.DefaultClient.")
	w.Line("HTTPClient *http.Client")
	w.Line("// BasePath is prefixed to every operation path.")
	w.Line("BasePath string")
	w.Line("// Headers are added to every request.")
	w.Line("Headers http.Header")
	w.Out()
	w.Line("}")
	w.Line("")

	w.Line("// ClientOption configures a Client.")
	w.Line("type ClientOption func(*Options)")
	w.Line("")

	w.Line("// WithHTTPClient sets the HTTP client used to send requests.")
	w.Line("func WithHTTPClient(client *http.Client) ClientOption {")
	w.In()
	w.Line("return func(options *Options) { options.HTTPClient = client }")
	w.Out()
	w.Line("}")
	w.Line("")

	w.Line("// WithBasePath prefixes every operation path.")
	w.Line("func WithBasePath(path string) ClientOption {")
	w.In()
	w.Line("return func(options *Options) { options.BasePath = cleanBasePath(path) }")
	w.Out()
	w.Line("}")
	w.Line("")

	w.Line("// WithHeader adds a static request header.")
	w.Line("func WithHeader(key, value string) ClientOption {")
	w.In()
	w.Line("return func(options *Options) {")
	w.In()
	w.Line("if options.Headers == nil {")
	w.In()
	w.Line("options.Headers = make(http.Header)")
	w.Out()
	w.Line("}")
	w.Line("options.Headers.Add(key, value)")
	w.Out()
	w.Line("}")
	w.Out()
	w.Line("}")
	w.Line("")

	w.Line("// Client is an HTTP+JSON client for %s.", g.service.ID)
	w.Line("type Client struct {")
	w.In()
	w.Line("endpoint string")
	w.Line("options Options")
	w.Out()
	w.Line("}")
	w.Line("")

	w.Line("// NewClient returns a client for endpoint.")
	w.Line("func NewClient(endpoint string, opts ...ClientOption) *Client {")
	w.In()
	w.Line("options := Options{HTTPClient: http.DefaultClient, Headers: make(http.Header)}")
	w.Line("for _, opt := range opts {")
	w.In()
	w.Line("if opt != nil {")
	w.In()
	w.Line("opt(&options)")
	w.Out()
	w.Line("}")
	w.Out()
	w.Line("}")
	w.Line("if options.HTTPClient == nil {")
	w.In()
	w.Line("options.HTTPClient = http.DefaultClient")
	w.Out()
	w.Line("}")
	w.Line("if options.Headers == nil {")
	w.In()
	w.Line("options.Headers = make(http.Header)")
	w.Out()
	w.Line("}")
	w.Line("return &Client{endpoint: endpoint, options: options}")
	w.Out()
	w.Line("}")
	w.Line("")

	for _, bundle := range g.bindings {
		if err := g.emitClientOperation(w, bundle); err != nil {
			return nil, err
		}
	}
	return w.Render()
}

func (g *generator) emitClientOperation(w *fileWriter, bundle *httpbinding.Binding) error {
	op, ok := g.m.Shape(bundle.Operation)
	if !ok || op.Type != model.TypeOperation {
		return nil
	}
	name := g.operationGoName(op)
	input, _ := g.m.Shape(op.Input)
	output, _ := g.m.Shape(op.Output)
	inName := g.typeNameFor(input)
	outName := g.typeNameFor(output)

	w.Line("// %s calls the %s operation.", name, op.ID)
	w.Line("func (c *Client) %s(ctx context.Context, input *types.%s, optFns ...func(*Options)) (*types.%s, error) {", name, inName, outName)
	w.In()
	w.Line("if input == nil {")
	w.In()
	w.Line("input = &types.%s{}", inName)
	w.Out()
	w.Line("}")
	w.Line("options := c.options")
	w.Line("if options.Headers != nil {")
	w.In()
	w.Line("options.Headers = options.Headers.Clone()")
	w.Out()
	w.Line("} else {")
	w.In()
	w.Line("options.Headers = make(http.Header)")
	w.Out()
	w.Line("}")
	w.Line("for _, fn := range optFns {")
	w.In()
	w.Line("if fn != nil {")
	w.In()
	w.Line("fn(&options)")
	w.Out()
	w.Line("}")
	w.Out()
	w.Line("}")
	w.Line("if options.HTTPClient == nil {")
	w.In()
	w.Line("options.HTTPClient = http.DefaultClient")
	w.Out()
	w.Line("}")
	w.Line("")

	if err := g.emitClientPath(w, bundle, name); err != nil {
		return err
	}
	hasQuery := g.emitClientQuery(w, bundle)
	w.Line("requestURL, err := url.Parse(joinEndpoint(c.endpoint, options.BasePath, path))")
	w.Line("if err != nil {")
	w.In()
	w.Line("return nil, fmt.Errorf(%s, err)", quoted(name+": building request URL: %w"))
	w.Out()
	w.Line("}")
	if hasQuery {
		w.Line("requestURL.RawQuery = query.Encode()")
	}
	w.Line("")
	w.Line("var body io.Reader")
	w.Line("contentType := \"\"")
	if err := g.emitClientBody(w, bundle, name); err != nil {
		return err
	}
	w.Line("request, err := http.NewRequestWithContext(ctx, %s, requestURL.String(), body)", quoted(bundle.Method))
	w.Line("if err != nil {")
	w.In()
	w.Line("return nil, fmt.Errorf(%s, err)", quoted(name+": building request: %w"))
	w.Out()
	w.Line("}")
	w.Line("for key, values := range options.Headers {")
	w.In()
	w.Line("for _, value := range values {")
	w.In()
	w.Line("request.Header.Add(key, value)")
	w.Out()
	w.Line("}")
	w.Out()
	w.Line("}")
	g.emitClientHeaders(w, bundle)
	w.Line("if contentType != \"\" {")
	w.In()
	w.Line("request.Header.Set(\"Content-Type\", contentType)")
	w.Out()
	w.Line("}")
	w.Line("")

	w.Line("response, err := options.HTTPClient.Do(request)")
	w.Line("if err != nil {")
	w.In()
	w.Line("return nil, fmt.Errorf(%s, err)", quoted(name+": sending request: %w"))
	w.Out()
	w.Line("}")
	w.Line("defer response.Body.Close()")
	w.Line("responseBody, err := io.ReadAll(response.Body)")
	w.Line("if err != nil {")
	w.In()
	w.Line("return nil, fmt.Errorf(%s, err)", quoted(name+": reading response: %w"))
	w.Out()
	w.Line("}")
	w.Line("if response.StatusCode < 200 || response.StatusCode >= 300 {")
	w.In()
	w.Line("return nil, decode%sError(response.StatusCode, responseBody)", name)
	w.Out()
	w.Line("}")
	w.Line("output := &types.%s{}", outName)

	if len(bundle.Output.Body) > 0 {
		w.Line("if len(responseBody) > 0 {")
		w.In()
		w.Line("if err := json.Unmarshal(responseBody, output); err != nil {")
		w.In()
		w.Line("return nil, fmt.Errorf(%s, err)", quoted(name+": decoding response body: %w"))
		w.Out()
		w.Line("}")
		w.Out()
		w.Line("}")
	}
	g.emitResetBindings(w, "output", bundle.Output, false)
	if err := g.emitClientResponseHeaders(w, bundle); err != nil {
		return err
	}
	if err := g.emitClientResponsePayload(w, bundle, name); err != nil {
		return err
	}
	w.Line("return output, nil")
	w.Out()
	w.Line("}")
	w.Line("")
	return g.emitClientErrorDecoder(w, bundle, name)
}

func (g *generator) emitClientPath(w *fileWriter, bundle *httpbinding.Binding, name string) error {
	if len(bundle.Segments) == 0 {
		w.Line("path := \"/\"")
		return nil
	}
	w.Line("path := \"\"")
	for _, segment := range bundle.Segments {
		if !segment.IsLabel {
			w.Line("path += %s", quoted("/"+segment.Literal))
			continue
		}
		memberID, ok := bundle.Input.Labels[segment.Label]
		if !ok {
			continue
		}
		member, target, ok := g.memberAndTarget(memberID)
		if !ok {
			continue
		}
		field := "input." + g.fieldName(member)
		w.Line("if %s == nil {", field)
		w.In()
		w.Line("return nil, fmt.Errorf(%s)", quoted(name+": input."+member.ID.Member+" is required for the URI label "+segment.Label))
		w.Out()
		w.Line("}")
		expr := g.formatValueExpr(w, member, target, "*"+field)
		w.Line("path += \"/\" + url.PathEscape(%s)", expr)
	}
	return nil
}

// emitClientQuery writes the query values and reports whether the query
// variable is used.
func (g *generator) emitClientQuery(w *fileWriter, bundle *httpbinding.Binding) bool {
	names := make([]string, 0, len(bundle.Input.Query))
	for name := range bundle.Input.Query {
		names = append(names, name)
	}
	sort.Strings(names)
	used := len(names) > 0 || !bundle.Input.QueryParams.IsZero()
	if !used {
		return false
	}
	w.Line("query := url.Values{}")
	for _, name := range names {
		member, target, ok := g.memberAndTarget(bundle.Input.Query[name])
		if !ok {
			continue
		}
		field := "input." + g.fieldName(member)
		if target.Type == model.TypeList || target.Type == model.TypeSet {
			elem, elemTarget, ok := g.collectionElement(target)
			if !ok {
				continue
			}
			w.Line("for _, elem := range %s {", field)
			w.In()
			w.Line("query.Add(%s, %s)", quoted(name), g.formatValueExpr(w, elem, elemTarget, "elem"))
			w.Out()
			w.Line("}")
			continue
		}
		w.Line("if %s != nil {", field)
		w.In()
		expr := field
		if fieldHasPointerWrapper(target) {
			expr = "*" + field
		}
		w.Line("query.Add(%s, %s)", quoted(name), g.formatValueExpr(w, member, target, expr))
		w.Out()
		w.Line("}")
	}
	if !bundle.Input.QueryParams.IsZero() {
		member, target, ok := g.memberAndTarget(bundle.Input.QueryParams)
		if ok {
			field := "input." + g.fieldName(member)
			if valueMember, valueTarget, ok := g.mapValue(target); ok {
				if valueTarget.Type == model.TypeList || valueTarget.Type == model.TypeSet {
					expr := "value"
					if listMember, elemTarget, ok := g.collectionElement(valueTarget); ok {
						expr = g.formatValueExpr(w, listMember, elemTarget, "value")
					}
					w.Line("for key, values := range %s {", field)
					w.In()
					emitBoundQuerySkip(w, names)
					w.Line("for _, value := range values {")
					w.In()
					w.Line("query.Add(key, %s)", expr)
					w.Out()
					w.Line("}")
					w.Out()
					w.Line("}")
				} else {
					expr := g.formatValueExpr(w, valueMember, valueTarget, "value")
					w.Line("for key, value := range %s {", field)
					w.In()
					emitBoundQuerySkip(w, names)
					w.Line("query.Add(key, %s)", expr)
					w.Out()
					w.Line("}")
				}
			}
		}
	}
	return true
}

func (g *generator) emitClientBody(w *fileWriter, bundle *httpbinding.Binding, name string) error {
	if !bundle.Input.Payload.IsZero() {
		member, target, ok := g.memberAndTarget(bundle.Input.Payload)
		if !ok {
			return nil
		}
		field := "input." + g.fieldName(member)
		mediaType := g.payloadMediaType(member, target)
		w.Line("if %s != nil {", field)
		w.In()
		switch target.Type {
		case model.TypeBlob:
			w.AddImport("bytes", "")
			w.Line("body = bytes.NewReader(%s)", field)
		case model.TypeString:
			w.AddImport("strings", "")
			w.Line("body = strings.NewReader(%s)", g.stringExpr(target, "*"+field))
		default:
			w.AddImport("bytes", "")
			expr := field
			if fieldHasPointerWrapper(target) {
				expr = "*" + field
			}
			switch target.Type {
			case model.TypeBigDecimal:
				w.AddImport(g.module+"/internal/smithy", "")
				w.Line("payload, err := smithy.MarshalBigDecimal(%s)", expr)
			case model.TypeTimestamp:
				w.AddImport(g.module+"/internal/smithy", "")
				format := g.timestampFormatFor(member, target)
				w.Line("formatted, err := smithy.FormatTimestamp(%s, %s)", expr, quoted(format))
				w.Line("if err != nil {")
				w.In()
				w.Line("return nil, fmt.Errorf(%s, err)", quoted(name+": encoding request payload: %w"))
				w.Out()
				w.Line("}")
				w.Line("payload, err := json.Marshal(formatted)")
			default:
				w.Line("payload, err := json.Marshal(%s)", expr)
			}
			w.Line("if err != nil {")
			w.In()
			w.Line("return nil, fmt.Errorf(%s, err)", quoted(name+": encoding request payload: %w"))
			w.Out()
			w.Line("}")
			w.Line("body = bytes.NewReader(payload)")
		}
		w.Line("contentType = %s", quoted(mediaType))
		w.Out()
		w.Line("}")
		return nil
	}
	if len(bundle.Input.Body) == 0 {
		return nil
	}
	w.AddImport("bytes", "")
	w.Line("bodyCopy := *input")
	g.emitResetBindings(w, "bodyCopy", bundle.Input, true)
	w.Line("payload, err := json.Marshal(&bodyCopy)")
	w.Line("if err != nil {")
	w.In()
	w.Line("return nil, fmt.Errorf(%s, err)", quoted(name+": encoding request body: %w"))
	w.Out()
	w.Line("}")
	w.Line("body = bytes.NewReader(payload)")
	w.Line("contentType = contentTypeJSON")
	return nil
}

func (g *generator) emitClientHeaders(w *fileWriter, bundle *httpbinding.Binding) {
	names := sortedBindingNames(bundle.Input.Headers)
	for _, name := range names {
		member, target, ok := g.memberAndTarget(bundle.Input.Headers[name])
		if !ok {
			continue
		}
		field := "input." + g.fieldName(member)
		if target.Type == model.TypeList || target.Type == model.TypeSet {
			elem, elemTarget, ok := g.collectionElement(target)
			if !ok {
				continue
			}
			w.AddImport("strings", "")
			w.Line("if len(%s) > 0 {", field)
			w.In()
			w.Line("values := make([]string, len(%s))", field)
			w.Line("for i, elem := range %s {", field)
			w.In()
			w.Line("values[i] = %s", g.formatValueExpr(w, elem, elemTarget, "elem"))
			w.Out()
			w.Line("}")
			w.Line("request.Header.Set(%s, strings.Join(values, \", \"))", quoted(name))
			w.Out()
			w.Line("}")
			continue
		}
		w.Line("if %s != nil {", field)
		w.In()
		expr := field
		if fieldHasPointerWrapper(target) {
			expr = "*" + field
		}
		w.Line("request.Header.Set(%s, %s)", quoted(name), g.formatValueExpr(w, member, target, expr))
		w.Out()
		w.Line("}")
	}
	if !bundle.Input.PrefixHeaders.IsZero() {
		member, target, ok := g.memberAndTarget(bundle.Input.PrefixHeaders)
		if ok {
			field := "input." + g.fieldName(member)
			prefix := bundle.Input.PrefixHeader
			expr := "value"
			if valueMember, valueTarget, ok := g.mapValue(target); ok {
				expr = g.formatValueExpr(w, valueMember, valueTarget, "value")
			}
			w.Line("for key, value := range %s {", field)
			w.In()
			if prefix != "" {
				w.Line("request.Header.Set(%s+key, %s)", quoted(prefix), expr)
			} else {
				w.Line("request.Header.Set(key, %s)", expr)
			}
			w.Out()
			w.Line("}")
		}
	}
}

func (g *generator) emitClientResponseHeaders(w *fileWriter, bundle *httpbinding.Binding) error {
	names := sortedBindingNames(bundle.Output.Headers)
	for _, name := range names {
		member, target, ok := g.memberAndTarget(bundle.Output.Headers[name])
		if !ok {
			continue
		}
		field := "output." + g.fieldName(member)
		if target.Type == model.TypeList || target.Type == model.TypeSet {
			elem, elemTarget, ok := g.collectionElement(target)
			if !ok {
				continue
			}
			w.AddImport("strings", "")
			w.Line("if values := response.Header.Values(%s); len(values) > 0 {", quoted(name))
			w.In()
			w.Line("joined := strings.Join(values, \", \")")
			w.Line("parts := strings.Split(joined, \",\")")
			listType := g.qualifiedTypeName(target)
			w.Line("parsed := make(%s, 0, len(parts))", listType)
			w.Line("for _, part := range parts {")
			w.In()
			w.Line("part = strings.TrimSpace(part)")
			parsed := g.emitParseValue(w, elem, elemTarget, "part", "response header "+name)
			w.Line("parsed = append(parsed, %s)", parsed)
			w.Out()
			w.Line("}")
			w.Line("%s = parsed", field)
			w.Out()
			w.Line("}")
			continue
		}
		w.Line("if values := response.Header.Values(%s); len(values) > 0 {", quoted(name))
		w.In()
		parsed := g.emitParseValue(w, member, target, "values[0]", "response header "+name)
		if fieldHasPointerWrapper(target) {
			w.Line("%s = &%s", field, parsed)
		} else {
			w.Line("%s = %s", field, parsed)
		}
		w.Out()
		w.Line("}")
	}
	if !bundle.Output.PrefixHeaders.IsZero() {
		member, target, ok := g.memberAndTarget(bundle.Output.PrefixHeaders)
		if ok {
			field := "output." + g.fieldName(member)
			prefix := bundle.Output.PrefixHeader
			mapType := g.qualifiedTypeName(target)
			w.AddImport("strings", "")
			w.Line("for key, values := range response.Header {")
			w.In()
			w.Line("if len(values) == 0 {")
			w.In()
			w.Line("continue")
			w.Out()
			w.Line("}")
			w.Line("if len(key) >= len(%s) && strings.EqualFold(key[:len(%s)], %s) {", quoted(prefix), quoted(prefix), quoted(prefix))
			w.In()
			w.Line("if %s == nil {", field)
			w.In()
			w.Line("%s = make(%s)", field, mapType)
			w.Out()
			w.Line("}")
			expr := "values[0]"
			if _, valueTarget, ok := g.mapValue(target); ok {
				expr = g.httpStringToType(valueTarget, expr)
			}
			w.Line("%s[strings.ToLower(key[len(%s):])] = %s", field, quoted(prefix), expr)
			w.Out()
			w.Line("}")
			w.Out()
			w.Line("}")
		}
	}
	if !bundle.Output.ResponseCode.IsZero() {
		member, target, ok := g.memberAndTarget(bundle.Output.ResponseCode)
		if ok {
			field := "output." + g.fieldName(member)
			g.emitAssignStatusCode(w, member, target, field, "response.StatusCode")
		}
	}
	return nil
}

func (g *generator) emitClientResponsePayload(w *fileWriter, bundle *httpbinding.Binding, name string) error {
	if bundle.Output.Payload.IsZero() {
		return nil
	}
	member, target, ok := g.memberAndTarget(bundle.Output.Payload)
	if !ok {
		return nil
	}
	field := "output." + g.fieldName(member)
	switch target.Type {
	case model.TypeBlob:
		w.Line("%s = responseBody", field)
	case model.TypeString:
		if g.hasNamedType(target) {
			w.Line("payload := %s(responseBody)", g.qualifiedTypeName(target))
		} else {
			w.Line("payload := string(responseBody)")
		}
		w.Line("%s = &payload", field)
	case model.TypeDocument:
		w.AddImport(g.module+"/internal/smithy", "")
		w.Line("if len(responseBody) > 0 {")
		w.In()
		w.Line("value, err := smithy.UnmarshalDocument(responseBody)")
		w.Line("if err != nil {")
		w.In()
		w.Line("return nil, fmt.Errorf(%s, err)", quoted(name+": decoding response payload: %w"))
		w.Out()
		w.Line("}")
		w.Line("%s = value", field)
		w.Out()
		w.Line("}")
	case model.TypeBigDecimal:
		w.AddImport(g.module+"/internal/smithy", "")
		w.Line("if len(responseBody) > 0 {")
		w.In()
		w.Line("value, err := smithy.UnmarshalBigDecimal(responseBody, 256)")
		w.Line("if err != nil {")
		w.In()
		w.Line("return nil, fmt.Errorf(%s, err)", quoted(name+": decoding response payload: %w"))
		w.Out()
		w.Line("}")
		w.Line("%s = value", field)
		w.Out()
		w.Line("}")
	case model.TypeTimestamp:
		w.AddImport("encoding/json", "")
		w.AddImport(g.module+"/internal/smithy", "")
		format := g.timestampFormatFor(member, target)
		w.Line("if len(responseBody) > 0 {")
		w.In()
		w.Line("var text string")
		w.Line("if err := json.Unmarshal(responseBody, &text); err != nil {")
		w.In()
		w.Line("return nil, fmt.Errorf(%s, err)", quoted(name+": decoding response payload: %w"))
		w.Out()
		w.Line("}")
		w.Line("value, err := smithy.ParseTimestamp(text, %s)", quoted(format))
		w.Line("if err != nil {")
		w.In()
		w.Line("return nil, fmt.Errorf(%s, err)", quoted(name+": decoding response payload: %w"))
		w.Out()
		w.Line("}")
		w.Line("%s = &value", field)
		w.Out()
		w.Line("}")
	case model.TypeUnion:
		union, ok := g.typeName(target)
		if !ok {
			g.diagnosticAt(validation.Danger, target.Source,
				"Union `%s` has no generated Go type for a response payload", target.ID)
			return nil
		}
		w.Line("if len(responseBody) > 0 {")
		w.In()
		w.Line("value, err := types.Unmarshal%s(responseBody)", union)
		w.Line("if err != nil {")
		w.In()
		w.Line("return nil, fmt.Errorf(%s, err)", quoted(name+": decoding response payload: %w"))
		w.Out()
		w.Line("}")
		w.Line("%s = value", field)
		w.Out()
		w.Line("}")
	default:
		w.Line("if len(responseBody) > 0 {")
		w.In()
		w.Line("if err := json.Unmarshal(responseBody, &%s); err != nil {", field)
		w.In()
		w.Line("return nil, fmt.Errorf(%s, err)", quoted(name+": decoding response payload: %w"))
		w.Out()
		w.Line("}")
		w.Out()
		w.Line("}")
	}
	return nil
}

func (g *generator) emitClientErrorDecoder(w *fileWriter, bundle *httpbinding.Binding, name string) error {
	w.Line("// decode%sError decodes a modeled error or falls back to a ProtocolError.", name)
	w.Line("func decode%sError(status int, body []byte) error {", name)
	w.In()
	w.Line("var envelope struct {")
	w.In()
	w.Line("Code string `json:\"code\"`")
	w.Line("Message string `json:\"message\"`")
	w.Out()
	w.Line("}")
	w.Line("if err := json.Unmarshal(body, &envelope); err != nil {")
	w.In()
	w.Line("return smithy.NewProtocolError(\"UnknownError\", string(body), status, body)")
	w.Out()
	w.Line("}")
	w.Line("switch envelope.Code {")
	for _, binding := range bundle.Errors {
		shape, ok := g.m.Shape(binding.Shape)
		if !ok {
			continue
		}
		typeName, ok := g.typeName(shape)
		if !ok {
			continue
		}
		w.Line("case %s:", quoted(shape.ID.Name))
		w.In()
		w.Line("var value types.%s", typeName)
		w.Line("if err := json.Unmarshal(body, &value); err != nil {")
		w.In()
		w.Line("return smithy.NewProtocolError(envelope.Code, envelope.Message, status, body)")
		w.Out()
		w.Line("}")
		w.Line("return &value")
		w.Out()
	}
	w.Line("}")
	w.Line("code := envelope.Code")
	w.Line("if code == \"\" {")
	w.In()
	w.Line("code = \"UnknownError\"")
	w.Out()
	w.Line("}")
	w.Line("message := envelope.Message")
	w.Line("if message == \"\" {")
	w.In()
	w.Line("message = strings.TrimSpace(string(body))")
	w.Out()
	w.Line("}")
	w.Line("return smithy.NewProtocolError(code, message, status, body)")
	w.Out()
	w.Line("}")
	return nil
}

// --- server -------------------------------------------------------------------

func (g *generator) emitServerFile() ([]byte, error) {
	g.localCounter = 0
	doc := fmt.Sprintf("Server for the %s protocol.", httpjson.TraitID)
	w := newFileWriter(g.rootPackage, doc)
	w.AddImport("errors", "")
	w.AddImport("fmt", "")
	w.AddImport("net/http", "")
	w.AddImport("net/url", "")
	w.AddImport("sort", "")
	w.AddImport("strings", "")
	w.AddImport(g.module+"/internal/smithy", "")
	if len(g.bindings) > 0 {
		w.AddImport(g.module+"/types", "")
	}

	w.Line("// ServerOption configures a Server.")
	w.Line("type ServerOption func(*Server)")
	w.Line("")

	w.Line("// WithMaxBodyBytes limits the size of request bodies. A non-positive limit disables the limit.")
	w.Line("func WithMaxBodyBytes(limit int64) ServerOption {")
	w.In()
	w.Line("return func(server *Server) { server.maxBodyBytes = limit }")
	w.Out()
	w.Line("}")
	w.Line("")

	w.Line("// WithErrorLogger receives errors that the server handles internally.")
	w.Line("func WithErrorLogger(logger func(error)) ServerOption {")
	w.In()
	w.Line("return func(server *Server) { server.errorLogger = logger }")
	w.Out()
	w.Line("}")
	w.Line("")

	w.Line("// WithServerBasePath mounts every operation under path.")
	w.Line("func WithServerBasePath(path string) ServerOption {")
	w.In()
	w.Line("return func(server *Server) { server.basePath = cleanBasePath(path) }")
	w.Out()
	w.Line("}")
	w.Line("")

	w.Line("// Server implements http.Handler for a Service.")
	w.Line("type Server struct {")
	w.In()
	w.Line("service Service")
	w.Line("maxBodyBytes int64")
	w.Line("basePath string")
	w.Line("errorLogger func(error)")
	w.Line("routes []route")
	w.Out()
	w.Line("}")
	w.Line("")

	w.Line("type route struct {")
	w.In()
	w.Line("method string")
	w.Line("segments []routeSegment")
	w.Line("handle func(http.ResponseWriter, *http.Request, map[string]string)")
	w.Out()
	w.Line("}")
	w.Line("")

	w.Line("type routeSegment struct {")
	w.In()
	w.Line("literal string")
	w.Line("label string")
	w.Line("isLabel bool")
	w.Out()
	w.Line("}")
	w.Line("")

	w.Line("// NewServer returns a server that dispatches requests to service.")
	w.Line("func NewServer(service Service, opts ...ServerOption) *Server {")
	w.In()
	w.Line("server := &Server{service: service, maxBodyBytes: defaultMaxBodyBytes}")
	w.Line("for _, opt := range opts {")
	w.In()
	w.Line("if opt != nil {")
	w.In()
	w.Line("opt(server)")
	w.Out()
	w.Line("}")
	w.Out()
	w.Line("}")
	w.Line("server.routes = []route{")
	w.In()
	for _, bundle := range g.bindings {
		op, ok := g.m.Shape(bundle.Operation)
		if !ok || op.Type != model.TypeOperation {
			continue
		}
		name := g.operationGoName(op)
		w.Line("{")
		w.In()
		w.Line("method: %s,", quoted(bundle.Method))
		if len(bundle.Segments) == 0 {
			w.Line("segments: nil,")
		} else {
			w.Line("segments: []routeSegment{")
			w.In()
			for _, segment := range bundle.Segments {
				if segment.IsLabel {
					w.Line("{label: %s, isLabel: true},", quoted(segment.Label))
				} else {
					w.Line("{literal: %s},", quoted(segment.Literal))
				}
			}
			w.Out()
			w.Line("},")
		}
		w.Line("handle: server.handle%s,", name)
		w.Out()
		w.Line("},")
	}
	w.Out()
	w.Line("}")
	w.Line("return server")
	w.Out()
	w.Line("}")
	w.Line("")

	w.Line("func (s *Server) logError(err error) {")
	w.In()
	w.Line("if s.errorLogger != nil && err != nil {")
	w.In()
	w.Line("s.errorLogger(err)")
	w.Out()
	w.Line("}")
	w.Out()
	w.Line("}")
	w.Line("")

	w.Line("// ServeHTTP routes and serves one request.")
	w.Line("func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {")
	w.In()
	w.Line("defer func() {")
	w.In()
	w.Line("if recovered := recover(); recovered != nil {")
	w.In()
	w.Line("s.logError(fmt.Errorf(%s, recovered))", quoted("panic: %v"))
	w.Line("writeProtocolError(w, http.StatusInternalServerError, \"InternalFailure\", \"internal server error\")")
	w.Out()
	w.Line("}")
	w.Out()
	w.Line("}()")
	w.Line("")
	w.Line("path := r.URL.EscapedPath()")
	w.Line("if path == \"\" {")
	w.In()
	w.Line("path = \"/\"")
	w.Out()
	w.Line("}")
	w.Line("if s.basePath != \"\" {")
	w.In()
	w.Line("if path != s.basePath && !strings.HasPrefix(path, s.basePath+\"/\") {")
	w.In()
	w.Line("writeProtocolError(w, http.StatusNotFound, \"NotFound\", \"not found\")")
	w.Line("return")
	w.Out()
	w.Line("}")
	w.Line("path = strings.TrimPrefix(path, s.basePath)")
	w.Out()
	w.Line("}")
	w.Line("parts, ok := splitPath(path)")
	w.Line("if !ok {")
	w.In()
	w.Line("writeProtocolError(w, http.StatusNotFound, \"NotFound\", \"not found\")")
	w.Line("return")
	w.Out()
	w.Line("}")
	w.Line("var allowed []string")
	w.Line("for _, candidate := range s.routes {")
	w.In()
	w.Line("labels, matched := matchRoute(candidate, parts)")
	w.Line("if !matched {")
	w.In()
	w.Line("continue")
	w.Out()
	w.Line("}")
	w.Line("if candidate.method != r.Method {")
	w.In()
	w.Line("allowed = append(allowed, candidate.method)")
	w.Line("continue")
	w.Out()
	w.Line("}")
	w.Line("candidate.handle(w, r, labels)")
	w.Line("return")
	w.Out()
	w.Line("}")
	w.Line("if len(allowed) > 0 {")
	w.In()
	w.Line("sort.Strings(allowed)")
	w.Line("w.Header().Set(\"Allow\", strings.Join(allowed, \", \"))")
	w.Line("writeProtocolError(w, http.StatusMethodNotAllowed, \"MethodNotAllowed\", \"method not allowed\")")
	w.Line("return")
	w.Out()
	w.Line("}")
	w.Line("writeProtocolError(w, http.StatusNotFound, \"NotFound\", \"not found\")")
	w.Out()
	w.Line("}")
	w.Line("")

	w.Line("func splitPath(path string) ([]string, bool) {")
	w.In()
	w.Line("if path == \"\" || path == \"/\" {")
	w.In()
	w.Line("return nil, true")
	w.Out()
	w.Line("}")
	w.Line("if !strings.HasPrefix(path, \"/\") {")
	w.In()
	w.Line("return nil, false")
	w.Out()
	w.Line("}")
	w.Line("path = strings.TrimSuffix(path, \"/\")")
	w.Line("if path == \"\" {")
	w.In()
	w.Line("return nil, true")
	w.Out()
	w.Line("}")
	w.Line("parts := strings.Split(strings.TrimPrefix(path, \"/\"), \"/\")")
	w.Line("for _, part := range parts {")
	w.In()
	w.Line("if part == \"\" {")
	w.In()
	w.Line("return nil, false")
	w.Out()
	w.Line("}")
	w.Out()
	w.Line("}")
	w.Line("return parts, true")
	w.Out()
	w.Line("}")
	w.Line("")

	w.Line("func matchRoute(candidate route, parts []string) (map[string]string, bool) {")
	w.In()
	w.Line("if len(candidate.segments) != len(parts) {")
	w.In()
	w.Line("return nil, false")
	w.Out()
	w.Line("}")
	w.Line("labels := make(map[string]string)")
	w.Line("for i, segment := range candidate.segments {")
	w.In()
	w.Line("value, err := url.PathUnescape(parts[i])")
	w.Line("if err != nil {")
	w.In()
	w.Line("return nil, false")
	w.Out()
	w.Line("}")
	w.Line("if segment.isLabel {")
	w.In()
	w.Line("if value == \"\" {")
	w.In()
	w.Line("return nil, false")
	w.Out()
	w.Line("}")
	w.Line("labels[segment.label] = value")
	w.Line("continue")
	w.Out()
	w.Line("}")
	w.Line("if value != segment.literal {")
	w.In()
	w.Line("return nil, false")
	w.Out()
	w.Line("}")
	w.Out()
	w.Line("}")
	w.Line("return labels, true")
	w.Out()
	w.Line("}")
	w.Line("")

	w.Line("func (s *Server) writeDecodeError(w http.ResponseWriter, err error) {")
	w.In()
	w.Line("var protocolErr *smithy.ProtocolError")
	w.Line("if errors.As(err, &protocolErr) {")
	w.In()
	w.Line("status := protocolErr.StatusCode")
	w.Line("if status == 0 {")
	w.In()
	w.Line("status = http.StatusBadRequest")
	w.Out()
	w.Line("}")
	w.Line("code := protocolErr.Code")
	w.Line("if code == \"\" {")
	w.In()
	w.Line("code = \"BadRequest\"")
	w.Out()
	w.Line("}")
	w.Line("writeProtocolError(w, status, code, protocolErr.Message)")
	w.Line("return")
	w.Out()
	w.Line("}")
	w.Line("writeProtocolError(w, http.StatusBadRequest, \"BadRequest\", err.Error())")
	w.Out()
	w.Line("}")
	w.Line("")

	for _, bundle := range g.bindings {
		if err := g.emitServerOperation(w, bundle); err != nil {
			return nil, err
		}
	}
	return w.Render()
}

func (g *generator) emitServerOperation(w *fileWriter, bundle *httpbinding.Binding) error {
	op, ok := g.m.Shape(bundle.Operation)
	if !ok || op.Type != model.TypeOperation {
		return nil
	}
	name := g.operationGoName(op)
	input, _ := g.m.Shape(op.Input)
	output, _ := g.m.Shape(op.Output)
	inName := g.typeNameFor(input)
	outName := g.typeNameFor(output)

	if err := g.emitServerDecode(w, bundle, name, inName); err != nil {
		return err
	}
	w.Line("func (s *Server) handle%s(w http.ResponseWriter, r *http.Request, labels map[string]string) {", name)
	w.In()
	w.Line("input, err := s.decode%sInput(r, labels)", name)
	w.Line("if err != nil {")
	w.In()
	w.Line("s.writeDecodeError(w, err)")
	w.Line("return")
	w.Out()
	w.Line("}")
	w.Line("if err := input.Validate(); err != nil {")
	w.In()
	w.Line("writeValidationProtocolError(w, err)")
	w.Line("return")
	w.Out()
	w.Line("}")
	w.Line("output, err := s.service.%s(r.Context(), input)", name)
	w.Line("if err != nil {")
	w.In()
	w.Line("switch e := err.(type) {")
	for _, binding := range bundle.Errors {
		shape, ok := g.m.Shape(binding.Shape)
		if !ok {
			continue
		}
		typeName, ok := g.typeName(shape)
		if !ok {
			continue
		}
		w.Line("case *types.%s:", typeName)
		w.In()
		w.Line("writeModeledError(w, %d, e.ErrorCode(), e, e)", binding.Code)
		w.Line("return")
		w.Out()
	}
	w.Line("case *smithy.ProtocolError:")
	w.In()
	w.Line("status := e.StatusCode")
	w.Line("if status == 0 {")
	w.In()
	w.Line("status = http.StatusInternalServerError")
	w.Out()
	w.Line("}")
	w.Line("writeProtocolError(w, status, e.Code, e.Message)")
	w.Line("return")
	w.Out()
	w.Line("}")
	w.Line("s.logError(err)")
	w.Line("writeProtocolError(w, http.StatusInternalServerError, \"InternalFailure\", \"internal server error\")")
	w.Line("return")
	w.Out()
	w.Line("}")
	w.Line("if output == nil {")
	w.In()
	w.Line("output = &types.%s{}", outName)
	w.Out()
	w.Line("}")
	w.Line("")
	statusVar := "status"
	w.Line("status := %d", bundle.SuccessCode)
	if !bundle.Output.ResponseCode.IsZero() {
		member, target, ok := g.memberAndTarget(bundle.Output.ResponseCode)
		if ok {
			field := "output." + g.fieldName(member)
			w.Line("if %s != nil {", field)
			w.In()
			w.Line("status = int(*%s)", field)
			_ = target
			w.Out()
			w.Line("}")
			w.Line("if status < 100 || status > 999 {")
			w.In()
			w.Line("status = http.StatusInternalServerError")
			w.Out()
			w.Line("}")
		}
	}
	g.emitServerHeaders(w, bundle)
	if err := g.emitServerResponsePayload(w, bundle, name, statusVar); err != nil {
		return err
	}
	w.Out()
	w.Line("}")
	w.Line("")
	return nil
}

func (g *generator) emitServerDecode(w *fileWriter, bundle *httpbinding.Binding, name, inName string) error {
	w.Line("func (s *Server) decode%sInput(r *http.Request, labels map[string]string) (*types.%s, error) {", name, inName)
	w.In()
	w.Line("input := &types.%s{}", inName)

	hasPayload := !bundle.Input.Payload.IsZero()
	hasBody := len(bundle.Input.Body) > 0
	if hasPayload || hasBody {
		w.Line("body, err := smithy.ReadBody(r, s.maxBodyBytes)")
		w.Line("if err != nil {")
		w.In()
		w.Line("if errors.Is(err, smithy.ErrBodyTooLarge) {")
		w.In()
		w.Line("return nil, smithy.NewProtocolError(\"PayloadTooLarge\", \"request body too large\", http.StatusRequestEntityTooLarge, nil)")
		w.Out()
		w.Line("}")
		w.Line("return nil, fmt.Errorf(\"reading request body: %%w\", err)")
		w.Out()
		w.Line("}")
	}
	if hasPayload {
		member, target, ok := g.memberAndTarget(bundle.Input.Payload)
		if ok {
			field := "input." + g.fieldName(member)
			switch target.Type {
			case model.TypeBlob:
				w.Line("%s = body", field)
			case model.TypeString:
				if g.hasNamedType(target) {
					w.Line("payload := %s(string(body))", g.qualifiedTypeName(target))
				} else {
					w.Line("payload := string(body)")
				}
				w.Line("%s = &payload", field)
			case model.TypeDocument:
				w.AddImport(g.module+"/internal/smithy", "")
				w.Line("if len(body) > 0 {")
				w.In()
				w.Line("value, err := smithy.UnmarshalDocument(body)")
				w.Line("if err != nil {")
				w.In()
				w.Line("return nil, fmt.Errorf(\"decoding request payload: %%w\", err)")
				w.Out()
				w.Line("}")
				w.Line("%s = value", field)
				w.Out()
				w.Line("}")
			case model.TypeBigDecimal:
				w.AddImport(g.module+"/internal/smithy", "")
				w.Line("if len(body) > 0 {")
				w.In()
				w.Line("value, err := smithy.UnmarshalBigDecimal(body, 256)")
				w.Line("if err != nil {")
				w.In()
				w.Line("return nil, fmt.Errorf(\"decoding request payload: %%w\", err)")
				w.Out()
				w.Line("}")
				w.Line("%s = value", field)
				w.Out()
				w.Line("}")
			case model.TypeTimestamp:
				w.AddImport("encoding/json", "")
				w.AddImport(g.module+"/internal/smithy", "")
				format := g.timestampFormatFor(member, target)
				w.Line("if len(body) > 0 {")
				w.In()
				w.Line("var text string")
				w.Line("if err := json.Unmarshal(body, &text); err != nil {")
				w.In()
				w.Line("return nil, fmt.Errorf(\"decoding request payload: %%w\", err)")
				w.Out()
				w.Line("}")
				w.Line("value, err := smithy.ParseTimestamp(text, %s)", quoted(format))
				w.Line("if err != nil {")
				w.In()
				w.Line("return nil, fmt.Errorf(\"decoding request payload: %%w\", err)")
				w.Out()
				w.Line("}")
				w.Line("%s = &value", field)
				w.Out()
				w.Line("}")
			case model.TypeUnion:
				union, ok := g.typeName(target)
				if !ok {
					g.diagnosticAt(validation.Danger, target.Source,
						"Union `%s` has no generated Go type for a request payload", target.ID)
					return nil
				}
				w.Line("if len(body) > 0 {")
				w.In()
				w.Line("value, err := types.Unmarshal%s(body)", union)
				w.Line("if err != nil {")
				w.In()
				w.Line("return nil, fmt.Errorf(\"decoding request payload: %%w\", err)")
				w.Out()
				w.Line("}")
				w.Line("%s = value", field)
				w.Out()
				w.Line("}")
			default:
				w.AddImport("encoding/json", "")
				w.Line("if len(body) > 0 {")
				w.In()
				w.Line("if err := json.Unmarshal(body, &%s); err != nil {", field)
				w.In()
				w.Line("return nil, fmt.Errorf(\"decoding request payload: %%w\", err)")
				w.Out()
				w.Line("}")
				w.Out()
				w.Line("}")
			}
		}
	} else if hasBody {
		w.AddImport("encoding/json", "")
		w.Line("if len(body) > 0 {")
		w.In()
		w.Line("if err := json.Unmarshal(body, input); err != nil {")
		w.In()
		w.Line("return nil, fmt.Errorf(\"decoding request body: %%w\", err)")
		w.Out()
		w.Line("}")
		w.Out()
		w.Line("}")
	}

	g.emitResetBindings(w, "input", bundle.Input, true)

	for _, segment := range bundle.Segments {
		if !segment.IsLabel {
			continue
		}
		memberID, ok := bundle.Input.Labels[segment.Label]
		if !ok {
			continue
		}
		member, target, ok := g.memberAndTarget(memberID)
		if !ok {
			continue
		}
		field := "input." + g.fieldName(member)
		labelVar := g.nextLocal("labelValue")
		okVar := g.nextLocal("ok")
		w.Line("%s, %s := labels[%s]", labelVar, okVar, quoted(segment.Label))
		w.Line("if !%s {", okVar)
		w.In()
		w.Line("return nil, fmt.Errorf(%s)", quoted("missing URI label "+segment.Label))
		w.Out()
		w.Line("}")
		parsed := g.emitParseValue(w, member, target, labelVar, "URI label "+segment.Label)
		g.emitAssignParsed(w, field, target, parsed)
	}

	hasQuery := len(bundle.Input.Query) > 0 || !bundle.Input.QueryParams.IsZero()
	if hasQuery {
		w.Line("query := r.URL.Query()")
	}
	names := sortedBindingNames(bundle.Input.Query)
	for _, queryName := range names {
		member, target, ok := g.memberAndTarget(bundle.Input.Query[queryName])
		if !ok {
			continue
		}
		field := "input." + g.fieldName(member)
		w.Line("if values, ok := query[%s]; ok && len(values) > 0 {", quoted(queryName))
		w.In()
		if target.Type == model.TypeList || target.Type == model.TypeSet {
			elem, elemTarget, ok := g.collectionElement(target)
			if !ok {
				w.Out()
				w.Line("}")
				continue
			}
			listType := g.qualifiedTypeName(target)
			parsedList := g.nextLocal("values")
			w.Line("%s := make(%s, 0, len(values))", parsedList, listType)
			w.Line("for _, value := range values {")
			w.In()
			parsed := g.emitParseValue(w, elem, elemTarget, "value", "query parameter "+queryName)
			w.Line("%s = append(%s, %s)", parsedList, parsedList, parsed)
			w.Out()
			w.Line("}")
			w.Line("%s = %s", field, parsedList)
		} else {
			parsed := g.emitParseValue(w, member, target, "values[0]", "query parameter "+queryName)
			g.emitAssignParsed(w, field, target, parsed)
		}
		w.Out()
		w.Line("}")
	}
	if !bundle.Input.QueryParams.IsZero() {
		member, target, ok := g.memberAndTarget(bundle.Input.QueryParams)
		if ok {
			field := "input." + g.fieldName(member)
			mapType := g.qualifiedTypeName(target)
			_, valueTarget, ok := g.mapValue(target)
			if !ok {
				return nil
			}
			if valueTarget.Type == model.TypeList || valueTarget.Type == model.TypeSet {
				listType := g.qualifiedTypeName(valueTarget)
				expr := "value"
				if _, elemTarget, ok := g.collectionElement(valueTarget); ok {
					expr = g.httpStringToType(elemTarget, "value")
				}
				parsed := g.nextLocal("parsedValues")
				w.Line("if len(query) > 0 {")
				w.In()
				w.Line("%s = make(%s, len(query))", field, mapType)
				w.Line("for key, values := range query {")
				w.In()
				emitBoundQuerySkip(w, names)
				w.Line("%s := make(%s, 0, len(values))", parsed, listType)
				w.Line("for _, value := range values {")
				w.In()
				w.Line("%s = append(%s, %s)", parsed, parsed, expr)
				w.Out()
				w.Line("}")
				w.Line("%s[key] = %s", field, parsed)
				w.Out()
				w.Line("}")
				w.Out()
				w.Line("}")
			} else {
				expr := g.httpStringToType(valueTarget, "values[0]")
				w.Line("if len(query) > 0 {")
				w.In()
				w.Line("%s = make(%s, len(query))", field, mapType)
				w.Line("for key, values := range query {")
				w.In()
				emitBoundQuerySkip(w, names)
				w.Line("if len(values) > 0 {")
				w.In()
				w.Line("%s[key] = %s", field, expr)
				w.Out()
				w.Line("}")
				w.Out()
				w.Line("}")
				w.Out()
				w.Line("}")
			}
		}
	}

	for _, headerName := range sortedBindingNames(bundle.Input.Headers) {
		member, target, ok := g.memberAndTarget(bundle.Input.Headers[headerName])
		if !ok {
			continue
		}
		field := "input." + g.fieldName(member)
		w.Line("if values := r.Header.Values(%s); len(values) > 0 {", quoted(headerName))
		w.In()
		if target.Type == model.TypeList || target.Type == model.TypeSet {
			elem, elemTarget, ok := g.collectionElement(target)
			if !ok {
				w.Out()
				w.Line("}")
				continue
			}
			listType := g.qualifiedTypeName(target)
			parsedList := g.nextLocal("values")
			w.Line("joined := strings.Join(values, \", \")")
			w.Line("parts := strings.Split(joined, \",\")")
			w.Line("%s := make(%s, 0, len(parts))", parsedList, listType)
			w.Line("for _, part := range parts {")
			w.In()
			w.Line("part = strings.TrimSpace(part)")
			w.Line("if part == \"\" {")
			w.In()
			w.Line("continue")
			w.Out()
			w.Line("}")
			parsed := g.emitParseValue(w, elem, elemTarget, "part", "header "+headerName)
			w.Line("%s = append(%s, %s)", parsedList, parsedList, parsed)
			w.Out()
			w.Line("}")
			w.Line("%s = %s", field, parsedList)
		} else {
			parsed := g.emitParseValue(w, member, target, "values[0]", "header "+headerName)
			g.emitAssignParsed(w, field, target, parsed)
		}
		w.Out()
		w.Line("}")
	}
	if !bundle.Input.PrefixHeaders.IsZero() {
		member, target, ok := g.memberAndTarget(bundle.Input.PrefixHeaders)
		if ok {
			field := "input." + g.fieldName(member)
			prefix := bundle.Input.PrefixHeader
			mapType := g.qualifiedTypeName(target)
			w.Line("for key, values := range r.Header {")
			w.In()
			w.Line("if len(values) == 0 {")
			w.In()
			w.Line("continue")
			w.Out()
			w.Line("}")
			w.Line("if len(key) >= len(%s) && strings.EqualFold(key[:len(%s)], %s) {", quoted(prefix), quoted(prefix), quoted(prefix))
			w.In()
			w.Line("if %s == nil {", field)
			w.In()
			w.Line("%s = make(%s)", field, mapType)
			w.Out()
			w.Line("}")
			expr := "values[0]"
			if _, valueTarget, ok := g.mapValue(target); ok {
				expr = g.httpStringToType(valueTarget, expr)
			}
			w.Line("%s[strings.ToLower(key[len(%s):])] = %s", field, quoted(prefix), expr)
			w.Out()
			w.Line("}")
			w.Out()
			w.Line("}")
		}
	}

	w.Line("return input, nil")
	w.Out()
	w.Line("}")
	w.Line("")
	return nil
}

func (g *generator) emitServerHeaders(w *fileWriter, bundle *httpbinding.Binding) {
	for _, headerName := range sortedBindingNames(bundle.Output.Headers) {
		member, target, ok := g.memberAndTarget(bundle.Output.Headers[headerName])
		if !ok {
			continue
		}
		field := "output." + g.fieldName(member)
		if target.Type == model.TypeList || target.Type == model.TypeSet {
			elem, elemTarget, ok := g.collectionElement(target)
			if !ok {
				continue
			}
			w.AddImport("strings", "")
			w.Line("if len(%s) > 0 {", field)
			w.In()
			w.Line("values := make([]string, len(%s))", field)
			w.Line("for i, elem := range %s {", field)
			w.In()
			w.Line("values[i] = %s", g.formatValueExpr(w, elem, elemTarget, "elem"))
			w.Out()
			w.Line("}")
			w.Line("w.Header().Set(%s, strings.Join(values, \", \"))", quoted(headerName))
			w.Out()
			w.Line("}")
			continue
		}
		w.Line("if %s != nil {", field)
		w.In()
		expr := field
		if fieldHasPointerWrapper(target) {
			expr = "*" + field
		}
		w.Line("w.Header().Set(%s, %s)", quoted(headerName), g.formatValueExpr(w, member, target, expr))
		w.Out()
		w.Line("}")
	}
	if !bundle.Output.PrefixHeaders.IsZero() {
		member, target, ok := g.memberAndTarget(bundle.Output.PrefixHeaders)
		if ok {
			field := "output." + g.fieldName(member)
			prefix := bundle.Output.PrefixHeader
			expr := "value"
			if valueMember, valueTarget, ok := g.mapValue(target); ok {
				expr = g.formatValueExpr(w, valueMember, valueTarget, "value")
			}
			w.Line("for key, value := range %s {", field)
			w.In()
			if prefix != "" {
				w.Line("w.Header().Set(%s+key, %s)", quoted(prefix), expr)
			} else {
				w.Line("w.Header().Set(key, %s)", expr)
			}
			w.Out()
			w.Line("}")
		}
	}
}

func (g *generator) emitServerResponsePayload(w *fileWriter, bundle *httpbinding.Binding, name, statusVar string) error {
	if !bundle.Output.Payload.IsZero() {
		member, target, ok := g.memberAndTarget(bundle.Output.Payload)
		if ok {
			field := "output." + g.fieldName(member)
			mediaType := g.payloadMediaType(member, target)
			w.Line("if %s != nil {", field)
			w.In()
			switch target.Type {
			case model.TypeBlob:
				w.Line("w.Header().Set(\"Content-Type\", %s)", quoted(mediaType))
				w.Line("w.WriteHeader(%s)", statusVar)
				w.Line("_, _ = w.Write(%s)", field)
			case model.TypeString:
				w.Line("w.Header().Set(\"Content-Type\", %s)", quoted(mediaType))
				w.Line("w.WriteHeader(%s)", statusVar)
				w.Line("_, _ = w.Write([]byte(%s))", g.stringExpr(target, "*"+field))
			default:
				w.AddImport("encoding/json", "")
				expr := field
				if fieldHasPointerWrapper(target) {
					expr = "*" + field
				}
				switch target.Type {
				case model.TypeBigDecimal:
					w.AddImport(g.module+"/internal/smithy", "")
					w.Line("data, err := smithy.MarshalBigDecimal(%s)", expr)
				case model.TypeTimestamp:
					w.AddImport(g.module+"/internal/smithy", "")
					format := g.timestampFormatFor(member, target)
					w.Line("formatted, err := smithy.FormatTimestamp(%s, %s)", expr, quoted(format))
					w.Line("if err != nil {")
					w.In()
					w.Line("s.logError(err)")
					w.Line("writeProtocolError(w, http.StatusInternalServerError, \"InternalFailure\", \"internal server error\")")
					w.Line("return")
					w.Out()
					w.Line("}")
					w.Line("data, err := json.Marshal(formatted)")
				default:
					w.Line("data, err := json.Marshal(%s)", expr)
				}
				w.Line("if err != nil {")
				w.In()
				w.Line("s.logError(err)")
				w.Line("writeProtocolError(w, http.StatusInternalServerError, \"InternalFailure\", \"internal server error\")")
				w.Line("return")
				w.Out()
				w.Line("}")
				w.Line("w.Header().Set(\"Content-Type\", %s)", quoted(mediaType))
				w.Line("w.WriteHeader(%s)", statusVar)
				w.Line("_, _ = w.Write(data)")
			}
			w.Line("return")
			w.Out()
			w.Line("}")
		}
	}
	if len(bundle.Output.Body) > 0 {
		w.AddImport("encoding/json", "")
		w.Line("bodyCopy := *output")
		g.emitResetBindings(w, "bodyCopy", bundle.Output, false)
		w.Line("data, err := json.Marshal(&bodyCopy)")
		w.Line("if err != nil {")
		w.In()
		w.Line("s.logError(err)")
		w.Line("writeProtocolError(w, http.StatusInternalServerError, \"InternalFailure\", \"internal server error\")")
		w.Line("return")
		w.Out()
		w.Line("}")
		w.Line("w.Header().Set(\"Content-Type\", contentTypeJSON)")
		w.Line("w.WriteHeader(%s)", statusVar)
		w.Line("_, _ = w.Write(data)")
		w.Line("return")
		return nil
	}
	w.Line("w.WriteHeader(%s)", statusVar)
	return nil
}

// --- shared generation helpers ------------------------------------------------

func (g *generator) emitResetBindings(w *fileWriter, receiver string, bindings httpbinding.MessageBindings, input bool) {
	ids := make([]model.ShapeID, 0, len(bindings.Labels)+len(bindings.Query)+len(bindings.Headers)+4)
	for _, id := range bindings.Labels {
		ids = append(ids, id)
	}
	for _, id := range bindings.Query {
		ids = append(ids, id)
	}
	for _, id := range bindings.Headers {
		ids = append(ids, id)
	}
	if !bindings.QueryParams.IsZero() {
		ids = append(ids, bindings.QueryParams)
	}
	if !bindings.PrefixHeaders.IsZero() {
		ids = append(ids, bindings.PrefixHeaders)
	}
	if !input && !bindings.ResponseCode.IsZero() {
		ids = append(ids, bindings.ResponseCode)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i].String() < ids[j].String() })
	seen := make(map[model.ShapeID]bool)
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		member, ok := g.m.Shape(id)
		if !ok {
			continue
		}
		w.Line("%s.%s = nil", receiver, g.fieldName(member))
	}
}

// emitParseValue writes statements that decode source into a uniquely named
// local variable and returns that variable. On failure it returns a wrapped
// error, which matches both client methods and server decode functions.
func (g *generator) emitParseValue(w *fileWriter, member, target *model.Shape, source, context string) string {
	name := g.nextLocal("parsed")
	fail := func() {
		w.Line("return nil, fmt.Errorf(%s, err)", quoted(context+": %w"))
	}
	switch target.Type {
	case model.TypeString:
		if g.hasNamedType(target) {
			w.Line("%s := %s(%s)", name, g.qualifiedTypeName(target), source)
		} else {
			w.Line("%s := %s", name, source)
		}
	case model.TypeEnum:
		w.Line("%s := %s(%s)", name, g.qualifiedTypeName(target), source)
	case model.TypeBoolean:
		w.AddImport("strconv", "")
		w.Line("%s, err := strconv.ParseBool(%s)", name, source)
		w.Line("if err != nil {")
		w.In()
		fail()
		w.Out()
		w.Line("}")
	case model.TypeByte, model.TypeShort, model.TypeInteger, model.TypeLong, model.TypeIntEnum:
		w.AddImport("strconv", "")
		bits := 64
		cast := ""
		switch target.Type {
		case model.TypeByte:
			bits, cast = 8, "int8"
		case model.TypeShort:
			bits, cast = 16, "int16"
		case model.TypeInteger:
			bits, cast = 32, "int32"
		case model.TypeLong:
			bits, cast = 64, "int64"
		case model.TypeIntEnum:
			bits, cast = 32, g.qualifiedTypeName(target)
		}
		parsed := g.nextLocal("number")
		w.Line("%s, err := strconv.ParseInt(%s, 10, %d)", parsed, source, bits)
		w.Line("if err != nil {")
		w.In()
		fail()
		w.Out()
		w.Line("}")
		w.Line("%s := %s(%s)", name, cast, parsed)
	case model.TypeFloat, model.TypeDouble:
		w.AddImport("strconv", "")
		bits := 64
		cast := "float64"
		if target.Type == model.TypeFloat {
			bits, cast = 32, "float32"
		}
		parsed := g.nextLocal("number")
		w.Line("%s, err := strconv.ParseFloat(%s, %d)", parsed, source, bits)
		w.Line("if err != nil {")
		w.In()
		fail()
		w.Out()
		w.Line("}")
		w.Line("%s := %s(%s)", name, cast, parsed)
	case model.TypeBigInteger:
		w.AddImport("math/big", "")
		okName := g.nextLocal("ok")
		w.Line("%s, %s := new(big.Int).SetString(%s, 10)", name, okName, source)
		w.Line("if !%s {", okName)
		w.In()
		w.Line("return nil, fmt.Errorf(%s, %s)", quoted(context+": invalid integer value %q"), source)
		w.Out()
		w.Line("}")
	case model.TypeBigDecimal:
		w.AddImport("math/big", "")
		w.Line("%s, _, err := big.ParseFloat(%s, 10, 256, big.ToNearestEven)", name, source)
		w.Line("if err != nil {")
		w.In()
		fail()
		w.Out()
		w.Line("}")
	case model.TypeTimestamp:
		w.AddImport(g.module+"/internal/smithy", "")
		format := g.timestampFormatFor(member, target)
		w.Line("%s, err := smithy.ParseTimestamp(%s, %s)", name, source, quoted(format))
		w.Line("if err != nil {")
		w.In()
		fail()
		w.Out()
		w.Line("}")
	default:
		w.Line("%s := %s", name, source)
	}
	return name
}

// emitAssignParsed assigns a parsed scalar to a member field.
func (g *generator) emitAssignParsed(w *fileWriter, field string, target *model.Shape, parsed string) {
	if fieldHasPointerWrapper(target) {
		w.Line("%s = &%s", field, parsed)
		return
	}
	w.Line("%s = %s", field, parsed)
}

func (g *generator) emitAssignStatusCode(w *fileWriter, member, target *model.Shape, field, statusExpr string) {
	name := g.nextLocal("statusCode")
	switch target.Type {
	case model.TypeByte:
		w.Line("%s := int8(%s)", name, statusExpr)
	case model.TypeShort:
		w.Line("%s := int16(%s)", name, statusExpr)
	case model.TypeInteger:
		w.Line("%s := int32(%s)", name, statusExpr)
	default:
		w.Line("%s := int64(%s)", name, statusExpr)
	}
	w.Line("%s = &%s", field, name)
}

// formatValueExpr returns a Go expression that formats a scalar for a label,
// query parameter, or header.
func (g *generator) formatValueExpr(w *fileWriter, member, target *model.Shape, expr string) string {
	switch target.Type {
	case model.TypeString:
		if g.hasNamedType(target) {
			return "string(" + expr + ")"
		}
		return expr
	case model.TypeEnum:
		return "string(" + expr + ")"
	case model.TypeBoolean:
		w.AddImport("strconv", "")
		return "strconv.FormatBool(" + expr + ")"
	case model.TypeByte, model.TypeShort, model.TypeInteger, model.TypeLong, model.TypeIntEnum:
		w.AddImport("strconv", "")
		return "strconv.FormatInt(int64(" + expr + "), 10)"
	case model.TypeFloat:
		w.AddImport("strconv", "")
		return "strconv.FormatFloat(float64(" + expr + "), 'g', -1, 32)"
	case model.TypeDouble:
		w.AddImport("strconv", "")
		return "strconv.FormatFloat(" + expr + ", 'g', -1, 64)"
	case model.TypeBigInteger:
		return expr + ".String()"
	case model.TypeBigDecimal:
		return expr + ".Text('g', -1)"
	case model.TypeTimestamp:
		format := g.timestampFormatFor(member, target)
		return "formatTimestamp(" + expr + ", " + quoted(format) + ")"
	default:
		return "fmt.Sprint(" + expr + ")"
	}
}

// httpStringToType converts a Go string expression (an HTTP query or header
// value) into the target's Go representation. It is the inverse of
// formatValueExpr for the string-like targets that HTTP map bindings allow.
func (g *generator) httpStringToType(target *model.Shape, expr string) string {
	if target == nil {
		return expr
	}
	switch target.Type {
	case model.TypeString:
		if g.hasNamedType(target) {
			return g.qualifiedTypeName(target) + "(" + expr + ")"
		}
		return expr
	case model.TypeEnum:
		return g.qualifiedTypeName(target) + "(" + expr + ")"
	default:
		return expr
	}
}

// payloadMediaType resolves the Content-Type of an HTTP payload.
func (g *generator) payloadMediaType(member, target *model.Shape) string {
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

// --- small model helpers ------------------------------------------------------

func (g *generator) memberAndTarget(id model.ShapeID) (*model.Shape, *model.Shape, bool) {
	member, ok := g.m.Shape(id)
	if !ok || !member.IsMember() {
		return nil, nil, false
	}
	target, ok := g.m.Shape(member.Target)
	if !ok {
		return nil, nil, false
	}
	return member, target, true
}

func (g *generator) collectionElement(collection *model.Shape) (*model.Shape, *model.Shape, bool) {
	member, ok := collection.GetMember("member")
	if !ok {
		return nil, nil, false
	}
	target, ok := g.m.Shape(member.Target)
	if !ok {
		return nil, nil, false
	}
	return member, target, true
}

func (g *generator) mapValue(m *model.Shape) (*model.Shape, *model.Shape, bool) {
	member, ok := m.GetMember("value")
	if !ok {
		return nil, nil, false
	}
	target, ok := g.m.Shape(member.Target)
	if !ok {
		return nil, nil, false
	}
	return member, target, true
}

func (g *generator) typeNameFor(shape *model.Shape) string {
	if shape == nil {
		return "Unit"
	}
	if name, ok := g.typeNames[shape.ID]; ok {
		return name
	}
	if shape.IsUnit() {
		return "Unit"
	}
	return safeTypeName(g.rawShapeName(shape.ID))
}

func (g *generator) qualifiedTypeName(shape *model.Shape) string {
	return "types." + g.typeNameFor(shape)
}

func (g *generator) hasNamedType(shape *model.Shape) bool {
	if shape == nil {
		return false
	}
	_, ok := g.typeNames[shape.ID]
	return ok
}

func (g *generator) stringExpr(target *model.Shape, expr string) string {
	if target == nil {
		return expr
	}
	switch target.Type {
	case model.TypeString, model.TypeEnum:
		if g.hasNamedType(target) {
			return "string(" + expr + ")"
		}
	}
	return expr
}

// emitBoundQuerySkip skips query parameter keys that are bound explicitly with
// @httpQuery, so that @httpQueryParams only carries unbound parameters.
func emitBoundQuerySkip(w *fileWriter, names []string) {
	if len(names) == 0 {
		return
	}
	quotedNames := make([]string, len(names))
	for i, name := range names {
		quotedNames[i] = quoted(name)
	}
	w.Line("switch key {")
	w.In()
	w.Line("case %s:", strings.Join(quotedNames, ", "))
	w.In()
	w.Line("continue")
	w.Out()
	w.Out()
	w.Line("}")
}

func sortedBindingNames(m map[string]model.ShapeID) []string {
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (g *generator) nextLocal(prefix string) string {
	g.localCounter++
	return fmt.Sprintf("%s%d", prefix, g.localCounter)
}

// goPackageName derives a Go package name from a module path.
func goPackageName(module string) string {
	module = strings.TrimRight(module, "/")
	parts := strings.Split(module, "/")
	if len(parts) == 0 {
		return "service"
	}
	name := parts[len(parts)-1]
	if len(parts) == 1 {
		if idx := strings.LastIndex(name, "."); idx >= 0 {
			name = name[idx+1:]
		}
	}
	if isVersionSegment(name) && len(parts) > 1 {
		name = parts[len(parts)-2]
	}
	name = strings.ToLower(name)
	if idx := strings.LastIndex(name, ".v"); idx >= 0 && isVersionSegment(name[idx+1:]) {
		name = name[:idx]
	}
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := b.String()
	out = strings.Trim(out, "_")
	if out == "" {
		return "service"
	}
	if out[0] >= '0' && out[0] <= '9' {
		out = "pkg_" + out
	}
	if goKeywords[out] || predeclaredGoNames[out] {
		out += "_"
	}
	return out
}

func isVersionSegment(s string) bool {
	if len(s) < 2 || s[0] != 'v' {
		return false
	}
	for i := 1; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
