$version: "2.0"

namespace example.codegen

/// A rich model used to exercise the Go code generator.
service Example {
    version: "1.0.0"
    operations: [PutThing, GetThing, ListThings]
}

operation PutThing {
    input: PutThingInput
    output: PutThingOutput
    errors: [ThingNotFound, ValidationFailure]
}

operation GetThing {
    input: GetThingInput
    output: GetThingOutput
    errors: [ThingNotFound]
}

operation ListThings {
    input: ListThingsInput
    output: ListThingsOutput
    errors: [ValidationFailure]
}

@error("client")
structure ThingNotFound {
    @required
    message: String
    resourceId: ThingId
}

@error("server")
structure ValidationFailure {
    message: String
    errors: ValidationErrorList
}

list ValidationErrorList {
    member: ValidationErrorEntry
}

structure ValidationErrorEntry {
    @required
    path: String
    @required
    message: String
}

@pattern("^[a-z]+$")
@length(min: 2, max: 16)
string ThingId

@length(min: 1, max: 64)
string Title

@range(min: 0, max: 100)
integer Percentage

@pattern("^[a-z0-9-]+$")
@length(min: 1, max: 8)
string Tag

list Tags {
    member: Tag
}

@range(min: 0, max: 10)
integer Score

@uniqueItems
list Scores {
    member: Score
}

@sparse
list OptionalTags {
    member: Tag
}

@sparse
map OptionalAttributes {
    key: String
    value: String
}

map Attributes {
    key: String
    value: String
}

list ThingIdList {
    member: ThingId
}

structure Scalars {
    aString: String
    aBoolean: Boolean
    aBlob: Blob
    aByte: Byte
    aShort: Short
    anInteger: Integer
    aLong: Long
    aFloat: Float
    aDouble: Double
    aBigInteger: BigInteger
    aBigDecimal: BigDecimal
    aTimestamp: Timestamp
    aDocument: Document
    optionalMap: OptionalAttributes
}

list ThingList {
    member: Thing
}

map ThingMap {
    key: String
    value: Thing
}

@pattern("^[a-z]+$")
@enum([
    {value: "small", name: "SMALL"},
    {value: "large", name: "LARGE"}
])
string Size

/// A thing in the example service.
structure Thing {
    /// The thing's ID.
    @required
    @length(min: 2, max: 16)
    id: ThingId

    @required
    title: Title

    @range(min: 1, max: 100)
    percentage: Percentage

    @length(min: 1, max: 4)
    tags: Tags

    scores: Scores

    @length(min: 1, max: 4)
    payload: Blob

    attributes: Attributes

    nested: Nested

    @required
    kind: ThingKind

    shape: Shape

    priority: Priority

    size: Size

    optionalTags: OptionalTags

    createdAt: Timestamp
    data: Document
}

structure Nested {
    @required
    value: String
}

structure Circle {
    @required
    radius: Double
}

structure Square {
    @required
    side: Double
}

union Shape {
    circle: Circle
    square: Square
    label: String
}

enum ThingKind {
    @enumValue("widget")
    WIDGET
    @enumValue("gadget")
    GADGET
}

intEnum Priority {
    @enumValue(1)
    LOW
    @enumValue(2)
    HIGH
}

/// Kept for compatibility with the first version of the model.
@deprecated(message: "use Thing instead", since: "0.9")
structure OldThing {
    name: String
}

structure PutThingInput {
    @required
    id: ThingId
    @required
    thing: Thing
}

structure PutThingOutput {
    thing: Thing
}

structure GetThingInput {
    @required
    id: ThingId
}

structure GetThingOutput {
    thing: Thing
    related: ThingList
    index: ThingMap
    old: OldThing
    scalars: Scalars
}

structure ListThingsInput {
    @length(min: 1, max: 10)
    ids: ThingIdList
}

structure ListThingsOutput {
    things: ThingList
}
