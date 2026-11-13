package model

import (
	"fmt"
	"sort"

	"github.com/tespeleta/gosmithy/idl"
)

// Severity is the severity of a diagnostic or validation event.
type Severity int

const (
	SeverityNote Severity = iota
	SeverityWarning
	SeverityDanger
)

func (s Severity) String() string {
	switch s {
	case SeverityNote:
		return "NOTE"
	case SeverityWarning:
		return "WARNING"
	case SeverityDanger:
		return "DANGER"
	default:
		return fmt.Sprintf("Severity(%d)", int(s))
	}
}

// ParseSeverity parses a severity name. It accepts the Smithy spellings and
// the common "error" alias for DANGER.
func ParseSeverity(s string) (Severity, bool) {
	switch s {
	case "NOTE", "note":
		return SeverityNote, true
	case "WARNING", "warning":
		return SeverityWarning, true
	case "DANGER", "danger", "ERROR", "error":
		return SeverityDanger, true
	default:
		return SeverityNote, false
	}
}

// Diagnostic is a problem found while assembling or validating a model.
//
// validation.Event is an alias of Diagnostic; keeping the type in model lets
// the assembler report problems without model importing validation (which
// would create an import cycle).
type Diagnostic struct {
	// ID is a stable event identifier such as "Model.UnresolvedShape".
	ID string
	// Severity is the event severity.
	Severity Severity
	// Message is a human-readable description.
	Message string
	// ShapeID is the shape the event is attached to, if any.
	ShapeID ShapeID
	// Source is the source location the event points at.
	Source idl.SourceLocation
}

// Error implements error.
func (d Diagnostic) Error() string {
	return d.String()
}

// String formats the diagnostic like the Smithy reference implementation:
//
//	file.smithy:3:5: DANGER: Target `Missing` is not found in the model
func (d Diagnostic) String() string {
	loc := d.Source.String()
	if loc == "" {
		return fmt.Sprintf("%s: %s", d.Severity, d.Message)
	}
	return fmt.Sprintf("%s: %s: %s", loc, d.Severity, d.Message)
}

// Message returns the diagnostic message, with the shape ID that the event is
// attached to optionally included. This is useful for tests and tooling that
// want to display the same text as the reference implementation.
func (d Diagnostic) FullMessage() string {
	if d.ShapeID.IsZero() {
		return d.Message
	}
	return d.ShapeID.String() + ": " + d.Message
}

// SortDiagnostics sorts events deterministically by source location, then by
// event ID and message.
func SortDiagnostics(events []Diagnostic) {
	sortDiagnostics(events)
}

func sortDiagnostics(events []Diagnostic) {
	// Sort is stable so events with identical keys keep registration order.
	sort.SliceStable(events, func(i, j int) bool {
		a, b := events[i], events[j]
		if a.Source.Filename != b.Source.Filename {
			return a.Source.Filename < b.Source.Filename
		}
		if a.Source.Line != b.Source.Line {
			return a.Source.Line < b.Source.Line
		}
		if a.Source.Column != b.Source.Column {
			return a.Source.Column < b.Source.Column
		}
		if a.ID != b.ID {
			return a.ID < b.ID
		}
		return a.Message < b.Message
	})
}
