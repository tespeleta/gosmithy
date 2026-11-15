package smithy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Timestamp formats supported by gosmithy.protocols#httpJson.
const (
	TimestampDateTime          = "date-time"
	TimestampEpochSeconds      = "epoch-seconds"
	TimestampEpochMilliseconds = "epoch-milliseconds"
	TimestampHTTPDate          = "http-date"
)

// DefaultTimestampFormat is the httpJson default timestamp format.
const DefaultTimestampFormat = TimestampDateTime

// httpDateFormat is the IMF-fixdate production of RFC 9110. It is written out
// instead of importing a network HTTP package so that the model-only runtime
// keeps no HTTP dependency.
const httpDateFormat = "Mon, 02 Jan 2006 15:04:05 GMT"

// FormatTimestamp encodes t using a Smithy timestamp format.
func FormatTimestamp(t time.Time, format string) (string, error) {
	switch format {
	case "", TimestampDateTime:
		return t.Format(time.RFC3339Nano), nil
	case TimestampEpochSeconds:
		return formatEpochSeconds(t), nil
	case TimestampEpochMilliseconds:
		return strconv.FormatInt(t.UnixMilli(), 10), nil
	case TimestampHTTPDate:
		return t.UTC().Format(httpDateFormat), nil
	default:
		return "", fmt.Errorf("smithy: unknown timestamp format %q", format)
	}
}

// ParseTimestamp decodes a Smithy timestamp in the given format.
func ParseTimestamp(value, format string) (time.Time, error) {
	switch format {
	case "", TimestampDateTime:
		return time.Parse(time.RFC3339Nano, value)
	case TimestampEpochSeconds:
		return parseEpochSeconds(value)
	case TimestampEpochMilliseconds:
		millis, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return time.Time{}, fmt.Errorf("smithy: parsing epoch-milliseconds timestamp %q: %w", value, err)
		}
		return time.UnixMilli(millis).UTC(), nil
	case TimestampHTTPDate:
		t, err := time.Parse(httpDateFormat, value)
		if err != nil {
			return time.Time{}, fmt.Errorf("smithy: parsing http-date timestamp %q: %w", value, err)
		}
		return t.UTC(), nil
	default:
		return time.Time{}, fmt.Errorf("smithy: unknown timestamp format %q", format)
	}
}

// formatEpochSeconds formats t as decimal seconds without going through
// float64, so fractional nanoseconds survive the round trip.
func formatEpochSeconds(t time.Time) string {
	nanos := t.UnixNano()
	negative := nanos < 0
	absolute := uint64(nanos)
	if negative {
		absolute = uint64(-(nanos + 1)) + 1
	}
	seconds := absolute / 1e9
	fraction := absolute % 1e9
	text := strconv.FormatUint(seconds, 10)
	if fraction != 0 {
		digits := fmt.Sprintf("%09d", fraction)
		text += "." + strings.TrimRight(digits, "0")
	}
	if negative {
		text = "-" + text
	}
	return text
}

// parseEpochSeconds parses decimal seconds exactly.
func parseEpochSeconds(value string) (time.Time, error) {
	original := value
	negative := false
	switch {
	case strings.HasPrefix(value, "-"):
		negative = true
		value = value[1:]
	case strings.HasPrefix(value, "+"):
		value = value[1:]
	}
	if value == "" {
		return time.Time{}, fmt.Errorf("smithy: invalid epoch-seconds timestamp %q", original)
	}
	intPart, fracPart, hasFraction := strings.Cut(value, ".")
	if intPart == "" {
		intPart = "0"
	}
	seconds, err := strconv.ParseInt(intPart, 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("smithy: parsing epoch-seconds timestamp %q: %w", original, err)
	}
	var nanos int64
	if hasFraction {
		if fracPart == "" || len(fracPart) > 9 {
			return time.Time{}, fmt.Errorf("smithy: parsing epoch-seconds timestamp %q: want at most 9 fractional digits", original)
		}
		fracPart += strings.Repeat("0", 9-len(fracPart))
		nanos, err = strconv.ParseInt(fracPart, 10, 64)
		if err != nil {
			return time.Time{}, fmt.Errorf("smithy: parsing epoch-seconds timestamp %q: %w", original, err)
		}
	}
	if negative {
		seconds, nanos = -seconds, -nanos
	}
	return time.Unix(seconds, nanos).UTC(), nil
}

// UnmarshalDocument decodes a document value without converting JSON numbers
// to float64. Numbers are returned as json.Number so that callers can decide
// how to handle them without losing precision.
func UnmarshalDocument(raw []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("smithy: decoding document: %w", err)
	}
	return value, nil
}
