// Package validation implements the structural validators used by gosmithy.
//
// Validation is deliberately small: it checks the model invariants that code
// generation depends on. Selector-based trait validation, custom validators,
// suppressions, and severity overrides are not implemented yet.
package validation

import (
	"github.com/tespeleta/gosmithy/model"
)

// Severity is the severity of a validation event.
type Severity = model.Severity

// Severities.
const (
	Note    = model.SeverityNote
	Warning = model.SeverityWarning
	Danger  = model.SeverityDanger
)

// Event is a validation event. It is an alias of model.Diagnostic so that
// assembly diagnostics and validator events share one representation without
// the model package importing validation (which would create a cycle).
type Event = model.Diagnostic

// ParseSeverity parses a severity name.
func ParseSeverity(s string) (Severity, bool) { return model.ParseSeverity(s) }
