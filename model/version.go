package model

import "github.com/tespeleta/gosmithy/idl"

// Version is a Smithy IDL version. It is used by code generators to describe
// the spec version a request targets.
type Version = idl.Version

// Supported Smithy IDL versions.
var (
	V1_0 = idl.V1_0
	V2_0 = idl.V2_0
	V2_1 = idl.V2_1
)
