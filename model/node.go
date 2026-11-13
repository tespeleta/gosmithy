package model

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/tespeleta/gosmithy/idl"
)

// NodeKind is the kind of a Smithy node value in the semantic model.
//
// Unlike idl.NodeKind, a model node never has a "shape ID" kind: syntactic
// shape IDs are resolved to strings during assembly and marked with
// Node.SyntacticShapeID.
type NodeKind int

const (
	NodeNull NodeKind = iota
	NodeBoolean
	NodeNumber
	NodeString
	NodeArray
	NodeObject
)

func (k NodeKind) String() string {
	switch k {
	case NodeNull:
		return "null"
	case NodeBoolean:
		return "boolean"
	case NodeNumber:
		return "number"
	case NodeString:
		return "string"
	case NodeArray:
		return "array"
	case NodeObject:
		return "object"
	default:
		return fmt.Sprintf("NodeKind(%d)", int(k))
	}
}

// Node is a resolved Smithy node value.
type Node struct {
	Kind NodeKind
	Str  string
	Num  string
	Bool bool
	Arr  []Node
	Obj  []ObjectEntry
	// Source is the location the node was parsed from.
	Source idl.SourceLocation
	// SyntacticShapeID is true when the node came from an unquoted identifier
	// in the IDL. Str then contains the resolved absolute shape ID and Raw
	// contains the original identifier text.
	SyntacticShapeID bool
	// Raw is the original text of a syntactic shape ID.
	Raw string
}

// ObjectEntry is one key/value pair of an object node, preserving source order.
type ObjectEntry struct {
	Key       string
	KeySource idl.SourceLocation
	Value     Node
}

// NullNode returns a null node.
func NullNode() Node { return Node{Kind: NodeNull} }

// StringNode returns a string node.
func StringNode(s string) Node { return Node{Kind: NodeString, Str: s} }

// BoolNode returns a boolean node.
func BoolNode(b bool) Node { return Node{Kind: NodeBoolean, Bool: b} }

// NumberNode returns a number node. The value is kept in its original textual
// form to avoid losing precision.
func NumberNode(s string) Node { return Node{Kind: NodeNumber, Num: s} }

// ArrayNode returns an array node.
func ArrayNode(values []Node) Node { return Node{Kind: NodeArray, Arr: values} }

// ObjectNode returns an object node.
func ObjectNode(entries []ObjectEntry) Node { return Node{Kind: NodeObject, Obj: entries} }

// Get returns the value for key in an object node.
func (n Node) Get(key string) (Node, bool) {
	for _, e := range n.Obj {
		if e.Key == key {
			return e.Value, true
		}
	}
	return Node{}, false
}

// GetString returns the string value for key in an object node.
func (n Node) GetString(key string) (string, bool) {
	v, ok := n.Get(key)
	if !ok || v.Kind != NodeString {
		return "", false
	}
	return v.Str, true
}

// IsNull reports whether the node is a null node.
func (n Node) IsNull() bool { return n.Kind == NodeNull }

// AsString returns the node's string value. Syntactic shape IDs are strings in
// the semantic model.
func (n Node) AsString() (string, bool) {
	if n.Kind != NodeString {
		return "", false
	}
	return n.Str, true
}

// AsInt returns the node's integer value.
func (n Node) AsInt() (int64, bool) {
	if n.Kind != NodeNumber || !isIntegerLexeme(n.Num) {
		return 0, false
	}
	v, err := strconv.ParseInt(n.Num, 10, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// AsFloat returns the node's floating point value.
func (n Node) AsFloat() (float64, bool) {
	if n.Kind != NodeNumber {
		return 0, false
	}
	v, err := strconv.ParseFloat(n.Num, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// IsInteger reports whether the node is a number without a fractional or
// exponent part.
func (n Node) IsInteger() bool {
	return n.Kind == NodeNumber && isIntegerLexeme(n.Num)
}

func isIntegerLexeme(s string) bool {
	s = strings.TrimPrefix(s, "-")
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// Equal reports whether two nodes are structurally equal.
func (n Node) Equal(other Node) bool {
	if n.Kind != other.Kind {
		return false
	}
	switch n.Kind {
	case NodeNull:
		return true
	case NodeBoolean:
		return n.Bool == other.Bool
	case NodeNumber:
		return n.Num == other.Num
	case NodeString:
		return n.Str == other.Str
	case NodeArray:
		if len(n.Arr) != len(other.Arr) {
			return false
		}
		for i := range n.Arr {
			if !n.Arr[i].Equal(other.Arr[i]) {
				return false
			}
		}
		return true
	case NodeObject:
		if len(n.Obj) != len(other.Obj) {
			return false
		}
		for _, e := range n.Obj {
			ov, ok := other.Get(e.Key)
			if !ok || !e.Value.Equal(ov) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

// String returns a compact human-readable representation of the node. JSON
// serialization is available through MarshalJSON.
func (n Node) String() string {
	b, err := json.Marshal(n)
	if err != nil {
		return fmt.Sprintf("<invalid %s node>", n.Kind)
	}
	return string(b)
}

// MarshalJSON implements json.Marshaler.
func (n Node) MarshalJSON() ([]byte, error) {
	switch n.Kind {
	case NodeNull:
		return []byte("null"), nil
	case NodeBoolean:
		if n.Bool {
			return []byte("true"), nil
		}
		return []byte("false"), nil
	case NodeNumber:
		return []byte(n.Num), nil
	case NodeString:
		return json.Marshal(n.Str)
	case NodeArray:
		if n.Arr == nil {
			return []byte("[]"), nil
		}
		return json.Marshal(n.Arr)
	case NodeObject:
		obj := make(map[string]Node, len(n.Obj))
		for _, e := range n.Obj {
			obj[e.Key] = e.Value
		}
		return json.Marshal(obj)
	default:
		return nil, fmt.Errorf("model: cannot marshal node of kind %s", n.Kind)
	}
}

// nodeFromIDL converts a parsed IDL node into a model node, resolving any
// syntactic shape IDs. resolve receives the raw identifier and returns the
// absolute shape ID it resolves to.
func nodeFromIDL(n idl.Node, resolve func(string) ShapeID) Node {
	switch n.Kind {
	case idl.NodeNull:
		return Node{Kind: NodeNull, Source: n.Source}
	case idl.NodeBoolean:
		return Node{Kind: NodeBoolean, Bool: n.Bool, Source: n.Source}
	case idl.NodeNumber:
		return Node{Kind: NodeNumber, Num: n.Num, Source: n.Source}
	case idl.NodeString:
		return Node{Kind: NodeString, Str: n.Str, Source: n.Source}
	case idl.NodeShapeID:
		id := resolve(n.Str)
		return Node{
			Kind:             NodeString,
			Str:              id.String(),
			Source:           n.Source,
			SyntacticShapeID: true,
			Raw:              n.Str,
		}
	case idl.NodeArray:
		arr := make([]Node, 0, len(n.Arr))
		for _, child := range n.Arr {
			arr = append(arr, nodeFromIDL(child, resolve))
		}
		return Node{Kind: NodeArray, Arr: arr, Source: n.Source}
	case idl.NodeObject:
		obj := make([]ObjectEntry, 0, len(n.Obj))
		for _, e := range n.Obj {
			obj = append(obj, ObjectEntry{
				Key:       e.Key,
				KeySource: e.KeySource,
				Value:     nodeFromIDL(e.Value, resolve),
			})
		}
		return Node{Kind: NodeObject, Obj: obj, Source: n.Source}
	default:
		return Node{Kind: NodeNull, Source: n.Source}
	}
}
