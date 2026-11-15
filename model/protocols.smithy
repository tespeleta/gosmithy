$version: "2.0"

namespace smithy.protocols

/// Defines a stable wire index for a structure or union member.
@trait(selector: ":is(structure, union) :not([trait|mixin]) > member")
integer idx
