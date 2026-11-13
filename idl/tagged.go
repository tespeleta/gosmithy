package idl

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"
)

// scanTaggedLiteral decodes a 2.1 tagged string literal. The handlers mirror
// Smithy's TaggedStringLiteral: #re preserves the contents, #b and #hex
// produce base64-encoded blobs, and #timestamp produces epoch seconds.
func scanTaggedLiteral(tag, raw string, isTextBlock bool) (Node, error) {
	raw = normalizeLineEndings(raw)
	if isTextBlock {
		formatted, err := formatTextBlock(raw)
		if err != nil {
			return Node{}, err
		}
		raw = formatted
	}

	switch tag {
	case "re":
		return Node{Kind: NodeString, Str: strings.ReplaceAll(raw, "\n", "")}, nil
	case "b":
		encoded, err := scanBinaryLiteral(raw)
		if err != nil {
			return Node{}, err
		}
		return Node{Kind: NodeString, Str: encoded}, nil
	case "hex":
		encoded, err := scanHexLiteral(raw)
		if err != nil {
			return Node{}, err
		}
		return Node{Kind: NodeString, Str: encoded}, nil
	case "timestamp":
		return scanTimestampLiteral(raw)
	default:
		return Node{}, fmt.Errorf("Unknown tagged literal: #%s", tag)
	}
}

func scanBinaryLiteral(raw string) (string, error) {
	var out []byte
	runes := []rune(raw)
	for i := 0; i < len(runes); i++ {
		c := runes[i]
		if c != '\\' || i+1 >= len(runes) {
			out = append(out, []byte(string(c))...)
			continue
		}
		next := runes[i+1]
		switch next {
		case 'x':
			if i+3 >= len(runes) {
				return "", fmt.Errorf("Incomplete \\x escape in binary string")
			}
			hi := hexValue(runes[i+2])
			lo := hexValue(runes[i+3])
			if hi < 0 || lo < 0 {
				return "", fmt.Errorf("Invalid hex digit: `%c`", runes[i+2])
			}
			out = append(out, byte(hi<<4|lo))
			i += 3
		case '\\':
			out = append(out, '\\')
			i++
		case '"':
			out = append(out, '"')
			i++
		case 'a':
			out = append(out, 0x07)
			i++
		case 'b':
			out = append(out, 0x08)
			i++
		case 'f':
			out = append(out, 0x0C)
			i++
		case 'n':
			out = append(out, 0x0A)
			i++
		case 'r':
			out = append(out, 0x0D)
			i++
		case 't':
			out = append(out, '\t')
			i++
		case 'v':
			out = append(out, 0x0B)
			i++
		case '0', '1', '2', '3', '4', '5', '6', '7':
			octal := int(next - '0')
			n := 1
			for n < 3 && i+1+n < len(runes) {
				oc := runes[i+1+n]
				if oc < '0' || oc > '7' {
					break
				}
				octal = octal<<3 | int(oc-'0')
				n++
			}
			if octal > 0xFF {
				return "", fmt.Errorf("Octal escape value \\%s exceeds byte range", octalString(octal))
			}
			out = append(out, byte(octal))
			i += n
		case '\n':
			i++
		default:
			return "", fmt.Errorf("Invalid escape in binary string: `\\%c`", next)
		}
	}
	return base64.StdEncoding.EncodeToString(out), nil
}

func octalString(v int) string {
	if v == 0 {
		return "0"
	}
	var digits []byte
	for v > 0 {
		digits = append([]byte{byte('0' + v%8)}, digits...)
		v /= 8
	}
	return string(digits)
}

func scanHexLiteral(raw string) (string, error) {
	var hex []byte
	inComment := false
	for _, c := range raw {
		if inComment {
			if c == '\n' {
				inComment = false
			}
			continue
		}
		switch {
		case c == '#':
			inComment = true
		case hexValue(c) >= 0:
			hex = append(hex, byte(c))
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			// ignored
		default:
			return "", fmt.Errorf("Invalid character in hex string: `%c`", c)
		}
	}
	if len(hex)%2 != 0 {
		return "", fmt.Errorf("Odd number of hex digits in hex string")
	}
	out := make([]byte, len(hex)/2)
	for i := 0; i < len(out); i++ {
		out[i] = byte(hexValue(rune(hex[i*2]))<<4 | hexValue(rune(hex[i*2+1])))
	}
	return base64.StdEncoding.EncodeToString(out), nil
}

func scanTimestampLiteral(raw string) (Node, error) {
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return Node{}, fmt.Errorf("Invalid RFC 3339 timestamp")
	}
	sec := t.Unix()
	millis := t.Nanosecond() / 1_000_000
	if millis == 0 {
		return Node{Kind: NodeNumber, Num: fmt.Sprintf("%d", sec)}, nil
	}
	return Node{Kind: NodeNumber, Num: fmt.Sprintf("%d.%03d", sec, millis)}, nil
}
