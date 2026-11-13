package idl

import "fmt"

// SourceLocation identifies a span of a Smithy IDL file.
//
// Start and End are 0-based byte offsets into the original source. Line and
// Column are 1-based positions of Start. End is exclusive.
type SourceLocation struct {
	Filename string
	Line     int
	Column   int
	Start    int
	End      int
}

func (l SourceLocation) String() string {
	if l.Filename == "" {
		return fmt.Sprintf("%d:%d", l.Line, l.Column)
	}
	return fmt.Sprintf("%s:%d:%d", l.Filename, l.Line, l.Column)
}

// isZero reports whether the location has not been set.
func (l SourceLocation) isZero() bool {
	return l.Line == 0 && l.Column == 0 && l.Start == 0 && l.End == 0 && l.Filename == ""
}

// ParseError is an error produced while tokenizing or parsing Smithy IDL.
//
// Syntax errors are rendered with the same "Syntax error at line L, column C"
// prefix used by the reference implementation. A few parser-level failures
// (for example, a use statement that targets a member) are rendered as plain
// messages, mirroring Smithy's ModelSyntaxException behavior.
type ParseError struct {
	Location SourceLocation
	Message  string
	Prefixed bool
	// ShapeID is set when the reference implementation associates the error
	// with a shape. Validation events render it as "shape: message".
	ShapeID string
}

func (e *ParseError) Error() string {
	prefix := ""
	if e.Location.Filename != "" {
		prefix = fmt.Sprintf("%s:%d:%d: ", e.Location.Filename, e.Location.Line, e.Location.Column)
	}
	if e.ShapeID != "" {
		prefix += e.ShapeID + ": "
	}
	if e.Prefixed || e.Location.Line <= 0 {
		return fmt.Sprintf("%sSyntax error at line %d, column %d: %s",
			prefix, e.Location.Line, e.Location.Column, e.Message)
	}
	if e.Location.Filename == "" && e.Location.Line > 0 {
		return fmt.Sprintf("%sline %d, column %d: %s", prefix, e.Location.Line, e.Location.Column, e.Message)
	}
	return prefix + e.Message
}

func syntaxError(loc SourceLocation, format string, args ...any) *ParseError {
	return &ParseError{Location: loc, Message: fmt.Sprintf(format, args...), Prefixed: true}
}

func modelError(loc SourceLocation, format string, args ...any) *ParseError {
	return &ParseError{Location: loc, Message: fmt.Sprintf(format, args...)}
}
