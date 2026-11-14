package smithy

import (
	"errors"
	"fmt"
	"io"
	"net/http"
)

// ProtocolError is returned by generated clients for response errors that do
// not decode into a modeled error shape. It is also the error form used by the
// generated server for protocol-level failures (bad request, unknown route,
// and so on).
type ProtocolError struct {
	// Code is the Smithy error code, for example "NotFound" or
	// "ValidationException".
	Code string
	// Message is the human-readable error message.
	Message string
	// StatusCode is the HTTP status code.
	StatusCode int
	// Body is the raw response body, when available.
	Body []byte
}

// Error implements error.
func (e *ProtocolError) Error() string {
	if e == nil {
		return "<nil>"
	}
	switch {
	case e.Code == "" && e.Message == "":
		return fmt.Sprintf("HTTP %d", e.StatusCode)
	case e.Code == "":
		return e.Message
	case e.Message == "":
		return e.Code
	default:
		return e.Code + ": " + e.Message
	}
}

// NewProtocolError returns a ProtocolError with the given fields.
func NewProtocolError(code, message string, status int, body []byte) *ProtocolError {
	return &ProtocolError{Code: code, Message: message, StatusCode: status, Body: body}
}

// ValidationErrorList returns the validation failures carried by err. A non
// validation error returns nil. Both value and pointer ValidationErrors are
// recognized, including when wrapped with fmt.Errorf("%w").
func ValidationErrorList(err error) []ValidationError {
	if err == nil {
		return nil
	}
	var value ValidationErrors
	if errors.As(err, &value) {
		return value
	}
	var pointer *ValidationErrors
	if errors.As(err, &pointer) && pointer != nil {
		return *pointer
	}
	return nil
}

// ErrBodyTooLarge is returned by ReadBody when a request exceeds the
// configured limit.
var ErrBodyTooLarge = errors.New("smithy: request body too large")

// ReadBody reads a request body, enforcing limit when limit > 0.
func ReadBody(r *http.Request, limit int64) ([]byte, error) {
	if r == nil || r.Body == nil {
		return nil, nil
	}
	if limit <= 0 {
		return io.ReadAll(r.Body)
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, ErrBodyTooLarge
	}
	return data, nil
}
