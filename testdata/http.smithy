$version: "2.0"

namespace example.http

use gosmithy.protocols#httpJson

/// Exercises every HTTP binding supported by gosmithy.protocols#httpJson.
@httpJson
service HttpJson {
    version: "1.0.0"
    operations: [
        GetThing
        SearchThings
        CreateThing
        UploadData
        DownloadData
        GetTime
        SetStatus
        HeaderEcho
        EchoTimestamps
        Empty
        FailClient
        FailServer
        UnionEcho
        EchoNumbers
        PatchThing
    ]
}

@pattern("^[a-z0-9-]+$")
string ThingId

enum Color {
    @enumValue("red")
    RED
    @enumValue("green")
    GREEN
    @enumValue("blue")
    BLUE
}

structure Thing {
    @required
    id: ThingId
    @required
    name: String
    color: Color
    @required
    createdAt: Timestamp
}

list ThingList {
    member: Thing
}

list ColorList {
    member: Color
}

list StringList {
    member: String
}

map StringMap {
    key: String
    value: String
}

@error("client")
@httpError(404)
structure ThingNotFound {
    @required
    message: String
    @required
    id: ThingId
}

@error("server")
@httpError(503)
structure ThingUnavailable {
    @required
    message: String
    reason: String
}

structure GetThingInput {
    @required
    @httpLabel
    id: ThingId

    @httpHeader("X-Trace-Id")
    traceId: String
}

structure GetThingOutput {
    @required
    thing: Thing

    @httpHeader("X-Version")
    version: String

    @httpHeader("X-Tag")
    tags: StringList

    @httpPrefixHeaders("X-Meta-")
    metadata: StringMap

    @httpResponseCode
    status: Integer
}

@http(method: "GET", uri: "/things/{id}")
operation GetThing {
    input: GetThingInput
    output: GetThingOutput
    errors: [ThingNotFound]
}

structure SearchThingsInput {
    @httpQuery("q")
    query: String

    @httpQuery("color")
    colors: ColorList

    @httpQuery("since")
    @timestampFormat("epoch-seconds")
    since: Timestamp

    @httpQueryParams
    params: StringMap
}

structure SearchThingsOutput {
    @required
    things: ThingList

    @httpHeader("X-Total")
    total: Integer
}

@http(method: "GET", uri: "/things")
operation SearchThings {
    input: SearchThingsInput
    output: SearchThingsOutput
    errors: [ThingUnavailable]
}

structure CreateThingInput {
    @required
    @jsonName("display_name")
    name: String
    color: Color
}

structure CreateThingOutput {
    @required
    thing: Thing
}

@http(method: "POST", uri: "/things")
operation CreateThing {
    input: CreateThingInput
    output: CreateThingOutput
    errors: [ThingUnavailable]
}

structure UploadDataInput {
    @required
    @httpLabel
    id: ThingId

    @httpPayload
    @mediaType("application/x-custom")
    data: Blob
}

structure UploadDataOutput {
    @httpHeader("X-Size")
    size: Long
}

@http(method: "PUT", uri: "/things/{id}/data")
operation UploadData {
    input: UploadDataInput
    output: UploadDataOutput
    errors: [ThingNotFound]
}

structure DownloadDataInput {
    @required
    @httpLabel
    id: ThingId
}

structure DownloadDataOutput {
    @httpPayload
    @mediaType("application/x-custom")
    data: Blob
}

@http(method: "GET", uri: "/things/{id}/data")
operation DownloadData {
    input: DownloadDataInput
    output: DownloadDataOutput
    errors: [ThingNotFound]
}

structure GetTimeOutput {
    @required
    now: Timestamp
}

@http(method: "GET", uri: "/time")
operation GetTime {
    output: GetTimeOutput
}

structure SetStatusInput {
    @required
    @httpQuery("code")
    code: Integer
}

structure SetStatusOutput {
    @required
    @httpResponseCode
    status: Integer
    @required
    message: String
}

@http(method: "POST", uri: "/status", code: 201)
operation SetStatus {
    input: SetStatusInput
    output: SetStatusOutput
}

structure HeaderEchoInput {
    @httpHeader("X-String")
    headerString: String
    @httpHeader("X-Int")
    headerInt: Integer
    @httpHeader("X-Bool")
    headerBool: Boolean
    @httpHeader("X-List")
    headerList: StringList
    @httpPrefixHeaders("X-Extra-")
    extra: StringMap
}

structure HeaderEchoOutput {
    @httpHeader("X-String")
    headerString: String
    @httpHeader("X-Int")
    headerInt: Integer
    @httpHeader("X-Bool")
    headerBool: Boolean
    @httpHeader("X-List")
    headerList: StringList
    @httpPrefixHeaders("X-Extra-")
    extra: StringMap
}

@http(method: "POST", uri: "/headers")
operation HeaderEcho {
    input: HeaderEchoInput
    output: HeaderEchoOutput
}

@timestampFormat("epoch-seconds")
timestamp EpochTimestamp

structure TimestampsInput {
    @required
    @timestampFormat("epoch-seconds")
    epochSeconds: Timestamp
    @required
    @timestampFormat("epoch-milliseconds")
    epochMilliseconds: Timestamp
    @required
    @timestampFormat("http-date")
    httpDate: Timestamp
    @required
    dateTime: Timestamp
    @required
    epochShape: EpochTimestamp
}

structure TimestampsOutput {
    @required
    @timestampFormat("epoch-seconds")
    epochSeconds: Timestamp
    @required
    @timestampFormat("epoch-milliseconds")
    epochMilliseconds: Timestamp
    @required
    @timestampFormat("http-date")
    httpDate: Timestamp
    @required
    dateTime: Timestamp
    @required
    epochShape: EpochTimestamp
}

@http(method: "POST", uri: "/timestamps")
operation EchoTimestamps {
    input: TimestampsInput
    output: TimestampsOutput
}

@http(method: "POST", uri: "/empty")
operation Empty {}

@http(method: "GET", uri: "/fail/client")
operation FailClient {
    errors: [ThingNotFound]
}

@http(method: "GET", uri: "/fail/server")
operation FailServer {
    errors: [ThingUnavailable]
}

union ThingChoice {
    thing: Thing
    name: String
}

@sparse
list ThingChoiceList {
    member: ThingChoice
}

structure UnionEchoInput {
    @required
    choice: ThingChoice

    @sparse
    choices: ThingChoiceList
}

structure UnionEchoOutput {
    @required
    choice: ThingChoice

    @sparse
    choices: ThingChoiceList
}

@http(method: "POST", uri: "/union")
operation UnionEcho {
    input: UnionEchoInput
    output: UnionEchoOutput
}

structure EchoNumbersInput {
    @required
    bigInteger: BigInteger
    @required
    bigDecimal: BigDecimal
    @required
    document: Document
}

structure EchoNumbersOutput {
    @required
    bigInteger: BigInteger
    @required
    bigDecimal: BigDecimal
    @required
    document: Document
}

@http(method: "POST", uri: "/numbers")
operation EchoNumbers {
    input: EchoNumbersInput
    output: EchoNumbersOutput
}

structure PatchThingInput {
    @required
    @httpLabel
    id: ThingId

    @httpHeader("X-Trace-Id")
    traceId: String

    @required
    name: String
    color: Color
}

structure PatchThingOutput {
    @required
    thing: Thing
}

@http(method: "PATCH", uri: "/things/{id}")
operation PatchThing {
    input: PatchThingInput
    output: PatchThingOutput
    errors: [ThingNotFound]
}
