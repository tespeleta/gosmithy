// Package openapi converts an assembled Smithy model into an OpenAPI 3.0.3
// document.
//
// The generator is deliberately small: it understands the HTTP bindings that
// gosmithy.protocols#httpJson uses and the JSON value encoding the generated Go
// code implements. Traits that cannot be represented in OpenAPI are reported
// as warnings and omitted from the document.
package openapi

// Document is the subset of an OpenAPI 3.0 document that gosmithy generates.
type Document struct {
	OpenAPI          string               `json:"openapi"`
	Info             Info                 `json:"info"`
	Tags             []Tag                `json:"tags,omitempty"`
	ExternalDocs     *ExternalDocs        `json:"externalDocs,omitempty"`
	Paths            map[string]*PathItem `json:"paths"`
	Components       *Components          `json:"components,omitempty"`
	Internal         bool                 `json:"x-smithy-internal,omitempty"`
	Deprecated       bool                 `json:"x-smithy-deprecated,omitempty"`
	Since            string               `json:"x-smithy-since,omitempty"`
	Unstable         bool                 `json:"x-smithy-unstable,omitempty"`
	UnstableFeature  string               `json:"x-smithy-unstable-feature,omitempty"`
	UnstableFeatures []string             `json:"x-smithy-unstable-features,omitempty"`
}

// Info describes the API.
type Info struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Version     string `json:"version"`
}

// Tag is an OpenAPI tag definition.
type Tag struct {
	Name         string        `json:"name"`
	Description  string        `json:"description,omitempty"`
	ExternalDocs *ExternalDocs `json:"externalDocs,omitempty"`
}

// ExternalDocs points at additional documentation.
type ExternalDocs struct {
	Description string `json:"description,omitempty"`
	URL         string `json:"url"`
}

// PathItem contains the operations for one path. Fields are pointers so an
// absent operation is omitted from JSON.
type PathItem struct {
	Get     *Operation `json:"get,omitempty"`
	Put     *Operation `json:"put,omitempty"`
	Post    *Operation `json:"post,omitempty"`
	Delete  *Operation `json:"delete,omitempty"`
	Options *Operation `json:"options,omitempty"`
	Head    *Operation `json:"head,omitempty"`
	Patch   *Operation `json:"patch,omitempty"`
	Trace   *Operation `json:"trace,omitempty"`
}

// Operation is one OpenAPI operation.
type Operation struct {
	Tags            []string             `json:"tags,omitempty"`
	Summary         string               `json:"summary,omitempty"`
	Description     string               `json:"description,omitempty"`
	OperationID     string               `json:"operationId"`
	Parameters      []*Parameter         `json:"parameters,omitempty"`
	RequestBody     *RequestBody         `json:"requestBody,omitempty"`
	Responses       map[string]*Response `json:"responses"`
	Deprecated      bool                 `json:"deprecated,omitempty"`
	ExternalDocs    *ExternalDocs        `json:"externalDocs,omitempty"`
	Internal        bool                 `json:"x-smithy-internal,omitempty"`
	Since           string               `json:"x-smithy-since,omitempty"`
	Unstable        bool                 `json:"x-smithy-unstable,omitempty"`
	UnstableFeature string               `json:"x-smithy-unstable-feature,omitempty"`
}

// Parameter is a path, query, or header parameter.
type Parameter struct {
	Name            string              `json:"name"`
	In              string              `json:"in"`
	Description     string              `json:"description,omitempty"`
	Required        bool                `json:"required,omitempty"`
	Deprecated      bool                `json:"deprecated,omitempty"`
	Schema          *Schema             `json:"schema,omitempty"`
	Style           string              `json:"style,omitempty"`
	Explode         *bool               `json:"explode,omitempty"`
	Examples        map[string]*Example `json:"examples,omitempty"`
	Internal        bool                `json:"x-smithy-internal,omitempty"`
	Since           string              `json:"x-smithy-since,omitempty"`
	Recommended     bool                `json:"x-smithy-recommended,omitempty"`
	Unstable        bool                `json:"x-smithy-unstable,omitempty"`
	UnstableFeature string              `json:"x-smithy-unstable-feature,omitempty"`
}

// RequestBody describes an operation request body.
type RequestBody struct {
	Description string                `json:"description,omitempty"`
	Required    bool                  `json:"required,omitempty"`
	Content     map[string]*MediaType `json:"content"`
}

// MediaType describes one request or response content type.
type MediaType struct {
	Schema   *Schema             `json:"schema,omitempty"`
	Examples map[string]*Example `json:"examples,omitempty"`
}

// Example is an OpenAPI example object.
type Example struct {
	Summary     string      `json:"summary,omitempty"`
	Description string      `json:"description,omitempty"`
	Value       interface{} `json:"value,omitempty"`
}

// Response describes one response status.
type Response struct {
	Description string                `json:"description"`
	Headers     map[string]*Header    `json:"headers,omitempty"`
	Content     map[string]*MediaType `json:"content,omitempty"`
}

// Header describes an OpenAPI response header.
type Header struct {
	Description string  `json:"description,omitempty"`
	Schema      *Schema `json:"schema,omitempty"`
}

// Components holds reusable schemas.
type Components struct {
	Schemas map[string]*Schema `json:"schemas"`
}

// Schema is the OpenAPI 3.0 schema subset produced by gosmithy.
type Schema struct {
	Ref                  string             `json:"$ref,omitempty"`
	ExternalDocs         *ExternalDocs      `json:"x-smithy-external-docs,omitempty"`
	Internal             bool               `json:"x-smithy-internal,omitempty"`
	Since                string             `json:"x-smithy-since,omitempty"`
	Recommended          bool               `json:"x-smithy-recommended,omitempty"`
	Unstable             bool               `json:"x-smithy-unstable,omitempty"`
	UnstableFeature      string             `json:"x-smithy-unstable-feature,omitempty"`
	Synthetic            bool               `json:"x-smithy-synthetic,omitempty"`
	Type                 string             `json:"type,omitempty"`
	Format               string             `json:"format,omitempty"`
	Title                string             `json:"title,omitempty"`
	Description          string             `json:"description,omitempty"`
	Properties           map[string]*Schema `json:"properties,omitempty"`
	Required             []string           `json:"required,omitempty"`
	Items                *Schema            `json:"items,omitempty"`
	AdditionalProperties *Schema            `json:"additionalProperties,omitempty"`
	OneOf                []*Schema          `json:"oneOf,omitempty"`
	AllOf                []*Schema          `json:"allOf,omitempty"`
	Enum                 []interface{}      `json:"enum,omitempty"`
	EnumDescriptions     map[string]string  `json:"x-enum-descriptions,omitempty"`
	EnumDeprecated       []string           `json:"x-enum-deprecated,omitempty"`
	EnumSince            map[string]string  `json:"x-enum-since,omitempty"`
	Nullable             bool               `json:"nullable,omitempty"`
	Deprecated           bool               `json:"deprecated,omitempty"`
	UniqueItems          bool               `json:"uniqueItems,omitempty"`
	Min                  *float64           `json:"minimum,omitempty"`
	Max                  *float64           `json:"maximum,omitempty"`
	MinLength            *int               `json:"minLength,omitempty"`
	MaxLength            *int               `json:"maxLength,omitempty"`
	MinItems             *int               `json:"minItems,omitempty"`
	MaxItems             *int               `json:"maxItems,omitempty"`
	MinProperties        *int               `json:"minProperties,omitempty"`
	MaxProperties        *int               `json:"maxProperties,omitempty"`
	Pattern              string             `json:"pattern,omitempty"`
}
