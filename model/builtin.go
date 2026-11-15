package model

import (
	_ "embed"
	"fmt"

	"github.com/tespeleta/gosmithy/idl"
)

// protocolsSource is the built-in subset of the smithy-protocol-traits model
// needed by the 2.1 language features. It currently defines the idx trait used
// by member index shorthand.
//
//go:embed protocols.smithy
var protocolsSource string

// BuiltinProtocolsPath is the source filename used for the embedded protocol
// trait definitions.
const BuiltinProtocolsPath = "smithy.protocols.builtin.smithy"

// ProtocolsIdxTrait is the shape ID of the member index trait.
var ProtocolsIdxTrait = MustShapeID("smithy.protocols#idx")

// parseBuiltinTraits parses the embedded smithy-protocol-traits subset.
func parseBuiltinTraits() (*idl.File, error) {
	f, err := idl.ParseWithOptions(BuiltinProtocolsPath, protocolsSource, idl.Options{
		DefaultVersion: idl.V2_0,
	})
	if err != nil {
		return nil, fmt.Errorf("model: parsing built-in protocol traits: %w", err)
	}
	if f.Namespace != "smithy.protocols" {
		return nil, fmt.Errorf("model: built-in protocol traits have namespace %q, want %q", f.Namespace, "smithy.protocols")
	}
	return f, nil
}

// definesProtocolsIdx reports whether the user model defines
// smithy.protocols#idx itself. A user definition wins over the built-in one.
func definesProtocolsIdx(files []*idl.File) bool {
	for _, f := range files {
		if f == nil {
			continue
		}
		for _, stmt := range f.Shapes {
			s, ok := stmt.(*idl.ShapeStmt)
			if !ok {
				continue
			}
			id, err := ParseShapeID(s.ID)
			if err != nil {
				continue
			}
			if id == ProtocolsIdxTrait {
				return true
			}
			if !id.IsAbsolute() && f.Namespace == "smithy.protocols" && id.Name == "idx" {
				return true
			}
		}
	}
	return false
}
