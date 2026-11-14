package golang

import "testing"

func TestPascalCase(t *testing.T) {
	tests := map[string]string{
		"my_shape":      "MyShape",
		"httpURL":       "HttpURL",
		"content_type":  "ContentType",
		"_leading":      "Leading",
		"with-dash":     "WithDash",
		"foo__bar":      "FooBar",
		"123abc":        "X123abc",
		"":              "X",
		"alreadyPascal": "AlreadyPascal",
	}
	for input, want := range tests {
		if got := pascalCase(input); got != want {
			t.Errorf("pascalCase(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestSafeNames(t *testing.T) {
	typeNames := map[string]string{
		"City":     "City",
		"String":   "String_",
		"Error":    "Error_",
		"Service":  "Service_",
		"Validate": "Validate_",
		"my_type":  "MyType",
	}
	for input, want := range typeNames {
		if got := safeTypeName(input); got != want {
			t.Errorf("safeTypeName(%q) = %q, want %q", input, got, want)
		}
	}

	fieldNames := map[string]string{
		"cityId":    "CityId",
		"validate":  "Validate_",
		"error":     "Error_",
		"errorCode": "ErrorCode_",
		"type":      "Type",
		"values":    "Values_",
	}
	for input, want := range fieldNames {
		if got := safeFieldName(input); got != want {
			t.Errorf("safeFieldName(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestEnumConstantNames(t *testing.T) {
	tests := []struct {
		enum, value, want string
	}{
		{"Color", "red", "ColorRed"},
		{"Color", "Red", "ColorRed"},
		{"ThingKind", "WIDGET", "ThingKindWidget"},
		{"Priority", "LOW", "PriorityLow"},
		{"Enum", "HTTP_URL", "EnumHttpUrl"},
		{"Enum", "", "EnumValue"},
	}
	for _, tc := range tests {
		if got := enumConstantName(tc.enum, tc.value); got != tc.want {
			t.Errorf("enumConstantName(%q, %q) = %q, want %q", tc.enum, tc.value, got, tc.want)
		}
	}
}

func TestMarkerName(t *testing.T) {
	tests := map[string]string{
		"Shape":     "isShape",
		"HTTPUnion": "isHTTPUnion",
		"":          "isUnion",
	}
	for input, want := range tests {
		if got := markerName(input); got != want {
			t.Errorf("markerName(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestIsExportedIdentifier(t *testing.T) {
	for _, input := range []string{"Foo", "Foo_", "X1", "_Foo"} {
		if !isExportedIdentifier(input) {
			t.Errorf("isExportedIdentifier(%q) = false, want true", input)
		}
	}
	for _, input := range []string{"", "foo", "1Foo", "Foo-Bar"} {
		if isExportedIdentifier(input) {
			t.Errorf("isExportedIdentifier(%q) = true, want false", input)
		}
	}
}
