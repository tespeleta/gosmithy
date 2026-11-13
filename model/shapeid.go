package model

import (
	"fmt"
	"strings"
)

// ShapeID is a fully-qualified Smithy shape ID.
//
// A shape ID has a namespace, a name, and optionally a member name. IDs are
// case-sensitive, but two IDs that differ only by case conflict with each
// other and are reported by the model.
//
// The zero ShapeID is invalid.
type ShapeID struct {
	Namespace string
	Name      string
	Member    string
}

// NewShapeID returns a shape ID for the given namespace and shape name.
func NewShapeID(namespace, name string) ShapeID {
	return ShapeID{Namespace: namespace, Name: name}
}

// MustShapeID parses s and panics if it is not a valid absolute shape ID. It
// is intended for package-level constants.
func MustShapeID(s string) ShapeID {
	id, err := ParseShapeID(s)
	if err != nil {
		panic(err)
	}
	return id
}

// ParseShapeID parses an absolute or relative shape ID.
//
// A relative ID (one without a '#') is parsed as a shape name and an optional
// member name. Absolute IDs must have a non-empty namespace. The parser only
// performs syntax validation; resolving a relative ID is the assembler's job.
func ParseShapeID(s string) (ShapeID, error) {
	if s == "" {
		return ShapeID{}, fmt.Errorf("invalid shape ID: empty string")
	}

	var namespace, rest string
	hasNamespace := false
	if i := strings.IndexByte(s, '#'); i >= 0 {
		if strings.IndexByte(s[i+1:], '#') >= 0 {
			return ShapeID{}, fmt.Errorf("invalid shape ID %q: more than one '#'", s)
		}
		namespace, rest = s[:i], s[i+1:]
		hasNamespace = true
		if namespace == "" {
			return ShapeID{}, fmt.Errorf("invalid shape ID %q: empty namespace", s)
		}
		if err := validateNamespace(namespace); err != nil {
			return ShapeID{}, fmt.Errorf("invalid shape ID %q: %v", s, err)
		}
	} else {
		rest = s
	}

	name, member, hasMember := strings.Cut(rest, "$")
	if name == "" {
		return ShapeID{}, fmt.Errorf("invalid shape ID %q: empty shape name", s)
	}
	if hasMember {
		if strings.IndexByte(member, '$') >= 0 {
			return ShapeID{}, fmt.Errorf("invalid shape ID %q: more than one '$'", s)
		}
		if member == "" {
			return ShapeID{}, fmt.Errorf("invalid shape ID %q: empty member name", s)
		}
		if !validIdentifier(member) {
			return ShapeID{}, fmt.Errorf("invalid shape ID %q: invalid member name %q", s, member)
		}
	}
	if !validIdentifier(name) {
		if !hasNamespace {
			return ShapeID{}, fmt.Errorf("invalid shape ID %q: invalid shape name %q", s, name)
		}
		return ShapeID{}, fmt.Errorf("invalid shape ID %q: invalid shape name %q", s, name)
	}

	return ShapeID{Namespace: namespace, Name: name, Member: member}, nil
}

// String returns the canonical string representation of the shape ID.
func (id ShapeID) String() string {
	var b strings.Builder
	if id.Namespace != "" {
		b.WriteString(id.Namespace)
		b.WriteByte('#')
	}
	b.WriteString(id.Name)
	if id.Member != "" {
		b.WriteByte('$')
		b.WriteString(id.Member)
	}
	return b.String()
}

// IsZero reports whether the shape ID is unset.
func (id ShapeID) IsZero() bool {
	return id.Namespace == "" && id.Name == "" && id.Member == ""
}

// IsMember reports whether the shape ID identifies a member shape.
func (id ShapeID) IsMember() bool {
	return id.Member != ""
}

// IsAbsolute reports whether the shape ID has a namespace.
func (id ShapeID) IsAbsolute() bool {
	return id.Namespace != ""
}

// MemberName returns the member name, or an empty string.
func (id ShapeID) MemberName() string {
	return id.Member
}

// WithoutMember returns the ID of the shape that contains the member, or the
// ID itself when it is not a member.
func (id ShapeID) WithoutMember() ShapeID {
	id.Member = ""
	return id
}

// WithMember returns a member shape ID contained by the shape ID.
func (id ShapeID) WithMember(name string) ShapeID {
	id.Member = name
	return id
}

// WithNamespace returns the shape ID in a different namespace.
func (id ShapeID) WithNamespace(namespace string) ShapeID {
	id.Namespace = namespace
	return id
}

// EqualFold reports whether the two shape IDs are equal ignoring case. This is
// used to find conflicts that Smithy treats as identical.
func (id ShapeID) EqualFold(other ShapeID) bool {
	return strings.EqualFold(id.String(), other.String())
}

func validateNamespace(namespace string) error {
	if namespace == "" {
		return fmt.Errorf("empty namespace")
	}
	for _, part := range strings.Split(namespace, ".") {
		if part == "" {
			return fmt.Errorf("namespace %q contains an empty segment", namespace)
		}
		if !validIdentifier(part) {
			return fmt.Errorf("namespace %q contains invalid segment %q", namespace, part)
		}
	}
	return nil
}

// validIdentifier reports whether s matches [A-Za-z_][A-Za-z0-9_]*.
func validIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '_':
		case c >= 'a' && c <= 'z':
		case c >= 'A' && c <= 'Z':
		case c >= '0' && c <= '9' && i > 0:
		default:
			return false
		}
	}
	return true
}
