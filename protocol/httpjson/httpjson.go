// Package httpjson defines gosmithy.protocols#httpJson, the generic HTTP+JSON
// protocol implemented by gosmithy.
//
// The protocol is defined in Smithy IDL and embedded in the binary so that the
// generator can load it when the protocol is selected. It deliberately depends
// only on traits from the Smithy prelude; no AWS traits or conventions are
// involved.
package httpjson

import (
	"fmt"
	"strings"

	"github.com/tespeleta/gosmithy/codegen"
	"github.com/tespeleta/gosmithy/idl"
	"github.com/tespeleta/gosmithy/model"
)

// TraitID is the absolute shape ID of the protocol trait.
const TraitID = "gosmithy.protocols#httpJson"

// ID is the parsed protocol trait shape ID.
var ID = model.MustShapeID(TraitID)

// DefinitionFile is the synthetic filename of the embedded protocol model.
const DefinitionFile = "gosmithy.protocols.smithy"

const definition = `$version: "2.0"
namespace gosmithy.protocols

/// A generic HTTP+JSON protocol. Values are encoded as JSON bodies, with
/// Smithy HTTP traits selecting labels, query parameters, headers, payloads,
/// and response codes.
@protocolDefinition(
    traits: [
        http
        httpError
        httpHeader
        httpLabel
        httpPayload
        httpPrefixHeaders
        httpQuery
        httpQueryParams
        httpResponseCode
        jsonName
        mediaType
        timestampFormat
    ]
)
@trait(selector: "service")
structure httpJson {}
`

// Definition parses and returns the embedded protocol model.
func Definition() (*idl.File, error) {
	file, err := idl.ParseWithOptions(DefinitionFile, definition, idl.Options{
		Version:        idl.V2_1,
		DefaultVersion: idl.V2_0,
	})
	if err != nil {
		return nil, fmt.Errorf("httpjson: parsing the embedded protocol definition: %w", err)
	}
	return file, nil
}

// Select resolves a protocol identifier to a codegen.Protocol and its model
// definition. The full shape ID and the short name "httpJson" are accepted.
func Select(name string) (*codegen.Protocol, *idl.File, error) {
	switch strings.TrimSpace(name) {
	case TraitID, "httpJson":
	default:
		return nil, nil, fmt.Errorf("unknown protocol %q (available: %s)", name, TraitID)
	}
	file, err := Definition()
	if err != nil {
		return nil, nil, err
	}
	return &codegen.Protocol{ID: ID}, file, nil
}

// Is reports whether id names this protocol.
func Is(id model.ShapeID) bool { return id == ID }
