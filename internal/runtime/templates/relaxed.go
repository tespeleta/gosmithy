package smithy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
)

// relaxedNumberText returns the JSON number literal carried by raw. With the
// relaxed-input protocol setting, a quoted JSON string holding a number is
// also accepted.
func relaxedNumberText(raw []byte) (string, error) {
	text := bytes.TrimSpace(raw)
	if len(text) == 0 {
		return "", fmt.Errorf("smithy: empty number")
	}
	if text[0] == '"' {
		var value string
		if err := json.Unmarshal(text, &value); err != nil {
			return "", err
		}
		text = []byte(value)
	}
	value := string(text)
	if !isJSONNumberLiteral(value) {
		return "", fmt.Errorf("smithy: invalid number %q", value)
	}
	return value, nil
}

// UnmarshalRelaxedInt8 decodes a JSON number or quoted number string into dst.
func UnmarshalRelaxedInt8(raw []byte, dst *int8) error {
	value, err := relaxedNumberText(raw)
	if err != nil {
		return err
	}
	parsed, err := strconv.ParseInt(value, 10, 8)
	if err != nil {
		return fmt.Errorf("smithy: parsing integer %q: %w", value, err)
	}
	*dst = int8(parsed)
	return nil
}

// UnmarshalRelaxedInt16 decodes a JSON number or quoted number string into dst.
func UnmarshalRelaxedInt16(raw []byte, dst *int16) error {
	value, err := relaxedNumberText(raw)
	if err != nil {
		return err
	}
	parsed, err := strconv.ParseInt(value, 10, 16)
	if err != nil {
		return fmt.Errorf("smithy: parsing integer %q: %w", value, err)
	}
	*dst = int16(parsed)
	return nil
}

// UnmarshalRelaxedInt32 decodes a JSON number or quoted number string into dst.
func UnmarshalRelaxedInt32(raw []byte, dst *int32) error {
	value, err := relaxedNumberText(raw)
	if err != nil {
		return err
	}
	parsed, err := strconv.ParseInt(value, 10, 32)
	if err != nil {
		return fmt.Errorf("smithy: parsing integer %q: %w", value, err)
	}
	*dst = int32(parsed)
	return nil
}

// UnmarshalRelaxedInt64 decodes a JSON number or quoted number string into dst.
func UnmarshalRelaxedInt64(raw []byte, dst *int64) error {
	value, err := relaxedNumberText(raw)
	if err != nil {
		return err
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fmt.Errorf("smithy: parsing integer %q: %w", value, err)
	}
	*dst = parsed
	return nil
}

// UnmarshalRelaxedFloat32 decodes a JSON number or quoted number string into dst.
func UnmarshalRelaxedFloat32(raw []byte, dst *float32) error {
	value, err := relaxedNumberText(raw)
	if err != nil {
		return err
	}
	parsed, err := strconv.ParseFloat(value, 32)
	if err != nil {
		return fmt.Errorf("smithy: parsing float %q: %w", value, err)
	}
	*dst = float32(parsed)
	return nil
}

// UnmarshalRelaxedFloat64 decodes a JSON number or quoted number string into dst.
func UnmarshalRelaxedFloat64(raw []byte, dst *float64) error {
	value, err := relaxedNumberText(raw)
	if err != nil {
		return err
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fmt.Errorf("smithy: parsing float %q: %w", value, err)
	}
	*dst = parsed
	return nil
}

// UnmarshalRelaxedBigInteger decodes a JSON integer or quoted integer string.
func UnmarshalRelaxedBigInteger(raw []byte, dst *big.Int) error {
	value, err := relaxedNumberText(raw)
	if err != nil {
		return err
	}
	parsed, ok := new(big.Int).SetString(value, 10)
	if !ok {
		return fmt.Errorf("smithy: invalid integer value %q", value)
	}
	*dst = *parsed
	return nil
}

// isJSONNumberLiteral reports whether value is a single JSON number literal.
func isJSONNumberLiteral(value string) bool {
	i := 0
	if i < len(value) && value[i] == '-' {
		i++
	}
	if i >= len(value) {
		return false
	}
	switch {
	case value[i] == '0':
		i++
	case value[i] >= '1' && value[i] <= '9':
		for i < len(value) && isASCIIDigit(value[i]) {
			i++
		}
	default:
		return false
	}
	if i < len(value) && value[i] == '.' {
		i++
		if i >= len(value) || !isASCIIDigit(value[i]) {
			return false
		}
		for i < len(value) && isASCIIDigit(value[i]) {
			i++
		}
	}
	if i < len(value) && (value[i] == 'e' || value[i] == 'E') {
		i++
		if i < len(value) && (value[i] == '+' || value[i] == '-') {
			i++
		}
		if i >= len(value) || !isASCIIDigit(value[i]) {
			return false
		}
		for i < len(value) && isASCIIDigit(value[i]) {
			i++
		}
	}
	return i == len(value)
}

func isASCIIDigit(c byte) bool { return c >= '0' && c <= '9' }
