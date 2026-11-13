$version: "2.0"

namespace example.weather

/// Provides weather forecasts.
@paginated(inputToken: "nextToken", outputToken: "nextToken", pageSize: "pageSize", items: "items")
service Weather {
    version: "2006-03-01"
    resources: [City]
    operations: [GetCurrentTime]
}

resource City {
    identifiers: { cityId: CityId }
    read: GetCity
    list: ListCities
    resources: [Forecast]
}

resource Forecast {
    identifiers: { cityId: CityId }
    read: GetForecast
}

@pattern("^[A-Za-z0-9 ]+$")
string CityId

@readonly
operation GetCurrentTime {
    output: GetCurrentTimeOutput
}

structure GetCurrentTimeOutput {
    @required
    time: Timestamp
}

@readonly
operation GetForecast {
    input: GetForecastInput
    output: GetForecastOutput
    errors: [NoSuchResource]
}

structure GetForecastInput {
    @required
    cityId: CityId
}

structure GetForecastOutput {
    @required
    chanceOfRain: Float
    @required
    low: Integer
    @required
    high: Integer
}

@readonly
operation GetCity {
    input: GetCityInput
    output: GetCityOutput
    errors: [NoSuchResource]
}

structure GetCityInput {
    @required
    cityId: CityId
}

structure GetCityOutput {
    @required
    name: String
    @required
    coordinates: CityCoordinates
}

@readonly
@paginated(inputToken: "nextToken", outputToken: "nextToken", pageSize: "pageSize", items: "items")
operation ListCities {
    input: ListCitiesInput
    output: ListCitiesOutput
}

structure ListCitiesInput {
    nextToken: String
    pageSize: Integer
}

// Traits can be applied outside of the definition.
apply ListCitiesInput @documentation("hello!")

structure ListCitiesOutput {
    nextToken: String
    @required
    items: CitySummaries
}

structure CityCoordinates {
    @required
    latitude: Float
    @required
    longitude: Float
}

@error("client")
structure NoSuchResource {
    @required
    resourceType: String
}

list CitySummaries {
    member: CitySummary
}

@references([{resource: City}])
structure CitySummary {
    @required
    cityId: CityId
    @required
    name: String
}
