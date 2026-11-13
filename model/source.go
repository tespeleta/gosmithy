package model

import "github.com/tespeleta/gosmithy/idl"

// SourceLocation is a span of a Smithy IDL file. It is an alias of
// idl.SourceLocation so callers can use either package name without
// conversions.
type SourceLocation = idl.SourceLocation
