package golang

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// goKeywords are Go's reserved words. Exported Go names generated from Smithy
// identifiers are PascalCase and never collide with these in practice, but the
// set is kept complete for sanitized fallbacks.
var goKeywords = map[string]bool{
	"break": true, "case": true, "chan": true, "const": true, "continue": true,
	"default": true, "defer": true, "else": true, "fallthrough": true, "for": true,
	"func": true, "go": true, "goto": true, "if": true, "import": true,
	"interface": true, "map": true, "package": true, "range": true, "return": true,
	"select": true, "struct": true, "switch": true, "type": true, "var": true,
}

// predeclaredGoNames are Go's predeclared identifiers.
var predeclaredGoNames = map[string]bool{
	"any": true, "bool": true, "byte": true, "comparable": true,
	"complex64": true, "complex128": true, "error": true, "float32": true,
	"float64": true, "int": true, "int8": true, "int16": true, "int32": true,
	"int64": true, "rune": true, "string": true, "uint": true, "uint8": true,
	"uint16": true, "uint32": true, "uint64": true, "uintptr": true,
	"true": true, "false": true, "iota": true, "nil": true,
	"append": true, "cap": true, "clear": true, "close": true, "complex": true,
	"copy": true, "delete": true, "imag": true, "len": true, "make": true,
	"max": true, "min": true, "new": true, "panic": true, "print": true,
	"println": true, "real": true, "recover": true,
}

// reservedTypeNames are package-level names the generated types package uses or
// reserves for future generated code.
var reservedTypeNames = map[string]bool{
	"Error":           true,
	"String":          true,
	"Service":         true,
	"Client":          true,
	"Handler":         true,
	"ValidationError": true,
	"Validate":        true,
}

// reservedFieldNames are additionally avoided for struct fields so that a
// member cannot shadow a generated method.
var reservedFieldNames = map[string]bool{
	"Error":            true,
	"String":           true,
	"Service":          true,
	"Client":           true,
	"Handler":          true,
	"ValidationError":  true,
	"Validate":         true,
	"ValidateContents": true,
	"ErrorCode":        true,
	"ErrorFault":       true,
	"Values":           true,
}

// pascalCase converts a Smithy identifier to an exported Go identifier.
//
// It splits on underscores and other non-identifier characters, uppercases the
// first rune of each part, and otherwise preserves the original spelling:
// "my_shape" -> "MyShape", "httpURL" -> "HttpURL".
func pascalCase(s string) string {
	var b strings.Builder
	upperNext := true
	for _, r := range s {
		if r == '_' || !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			upperNext = true
			continue
		}
		if upperNext {
			b.WriteRune(unicode.ToUpper(r))
			upperNext = false
			continue
		}
		b.WriteRune(r)
	}
	out := b.String()
	if out == "" {
		return "X"
	}
	first, _ := utf8.DecodeRuneInString(out)
	if unicode.IsDigit(first) {
		return "X" + out
	}
	return out
}

// safeTypeName returns an exported package-level name for a shape.
func safeTypeName(name string) string {
	name = pascalCase(name)
	if goKeywords[name] || predeclaredGoNames[name] || reservedTypeNames[name] {
		return name + "_"
	}
	if !isExportedIdentifier(name) {
		return "X" + name
	}
	return name
}

// safeFieldName returns an exported field name for a member.
func safeFieldName(name string) string {
	name = pascalCase(name)
	if goKeywords[name] || predeclaredGoNames[name] || reservedFieldNames[name] {
		return name + "_"
	}
	if !isExportedIdentifier(name) {
		return "X" + name
	}
	return name
}

// enumConstantName builds the preferred constant name for an enum value.
func enumConstantName(enumName, valueName string) string {
	return enumName + constantPart(valueName)
}

// constantPart turns an enum member name into the value portion of a constant
// name. All-uppercase Smithy enum labels such as WIDGET become Widget while
// mixed-case names preserve their spelling.
func constantPart(name string) string {
	if name == "" {
		return "Value"
	}
	if isUpperLabel(name) {
		return pascalCase(strings.ToLower(name))
	}
	return pascalCase(name)
}

func isUpperLabel(s string) bool {
	sawLetter := false
	for _, r := range s {
		switch {
		case unicode.IsLetter(r):
			sawLetter = true
			if !unicode.IsUpper(r) {
				return false
			}
		case unicode.IsDigit(r), r == '_':
		default:
			return false
		}
	}
	return sawLetter
}

// markerName returns the unexported interface marker method for a union type.
func markerName(typeName string) string {
	if typeName == "" {
		return "isUnion"
	}
	name := "is" + typeName
	if goKeywords[name] || predeclaredGoNames[name] {
		return name + "_"
	}
	return name
}

func isExportedIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		switch {
		case unicode.IsLetter(r):
			if i == 0 && !unicode.IsUpper(r) {
				return false
			}
		case r == '_':
		case unicode.IsDigit(r) && i > 0:
		default:
			return false
		}
	}
	return true
}
