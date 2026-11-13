package idl

import "fmt"

// Version is a Smithy IDL version.
type Version struct {
	Major int
	Minor int
}

var (
	V1_0 = Version{1, 0}
	V2_0 = Version{2, 0}
	V2_1 = Version{2, 1}
)

func (v Version) String() string {
	return fmt.Sprintf("%d.%d", v.Major, v.Minor)
}

func (v Version) atLeast(other Version) bool {
	return v.AtLeast(other)
}

// AtLeast reports whether v is greater than or equal to other.
func (v Version) AtLeast(other Version) bool {
	if v.Major != other.Major {
		return v.Major > other.Major
	}
	return v.Minor >= other.Minor
}

// Options controls parsing.
type Options struct {
	// Version is the newest IDL version this parser build supports. A file
	// that requests a newer version is rejected. A zero value means V2_1.
	Version Version
	// DefaultVersion is assumed when a file has no $version control
	// statement. A zero value means V2_0, matching PLAN.md's "assume 2.0"
	// guidance for the MVP.
	DefaultVersion Version
}

func (o Options) maxVersion() Version {
	if o.Version == (Version{}) {
		return V2_1
	}
	return o.Version
}

func (o Options) defaultVersion() Version {
	if o.DefaultVersion != (Version{}) {
		return o.DefaultVersion
	}
	return V2_0
}

// File is the parsed representation of one Smithy IDL file.
type File struct {
	Path      string
	Version   Version
	Controls  []Control
	Metadata  []Metadata
	Namespace string
	Uses      []Use
	Shapes    []Stmt
	Source    SourceLocation
}

// Control is a control statement such as $version.
type Control struct {
	Name   string
	Value  Node
	Source SourceLocation
}

// Metadata is a metadata statement.
type Metadata struct {
	Key    string
	Value  Node
	Source SourceLocation
}

// Use is a use statement.
type Use struct {
	ID     string
	Source SourceLocation
}

// Stmt is a shape or apply statement.
type Stmt interface {
	stmt()
	Location() SourceLocation
}

// ShapeKind is the syntactic kind of a shape declaration.
type ShapeKind string

const (
	KindBlob       ShapeKind = "blob"
	KindBoolean    ShapeKind = "boolean"
	KindDocument   ShapeKind = "document"
	KindString     ShapeKind = "string"
	KindByte       ShapeKind = "byte"
	KindShort      ShapeKind = "short"
	KindInteger    ShapeKind = "integer"
	KindLong       ShapeKind = "long"
	KindFloat      ShapeKind = "float"
	KindDouble     ShapeKind = "double"
	KindBigInteger ShapeKind = "bigInteger"
	KindBigDecimal ShapeKind = "bigDecimal"
	KindTimestamp  ShapeKind = "timestamp"
	KindEnum       ShapeKind = "enum"
	KindIntEnum    ShapeKind = "intEnum"
	KindList       ShapeKind = "list"
	KindSet        ShapeKind = "set"
	KindMap        ShapeKind = "map"
	KindStructure  ShapeKind = "structure"
	KindUnion      ShapeKind = "union"
	KindService    ShapeKind = "service"
	KindResource   ShapeKind = "resource"
	KindOperation  ShapeKind = "operation"
)

// ShapeStmt is a shape definition.
type ShapeStmt struct {
	Traits []Trait
	Kind   ShapeKind
	Name   string
	ID     string // namespace#name, unresolved
	Mixins []ShapeRef
	// Members holds aggregate/enum members. Operation inputs and outputs are
	// stored in Input and Output.
	Members []Member
	// Body is the node object of a service or resource shape.
	Body        *Node
	Input       *InlineIO
	Output      *InlineIO
	Errors      []ShapeRef
	ForResource string
	Source      SourceLocation
	Doc         string
	DocSource   SourceLocation
	Namespace   string
}

func (*ShapeStmt) stmt()                      {}
func (s *ShapeStmt) Location() SourceLocation { return s.Source }

// ShapeRef is an unresolved reference to a shape from a mixin, operation
// error list, or inline target.
type ShapeRef struct {
	ID     string
	Source SourceLocation
}

// InlineIO describes an operation input or output. If Target is non-nil, the
// operation refers to an existing shape; otherwise the remaining fields
// describe an inline structure.
type InlineIO struct {
	Target      *ShapeRef
	Traits      []Trait
	Mixins      []ShapeRef
	Members     []Member
	ForResource string
	Source      SourceLocation
	Doc         string
	DocSource   SourceLocation
}

// Member is a member of an aggregate or enum shape.
type Member struct {
	Traits []Trait
	Name   string
	Target Target
	// Value is an enum/intEnum value assignment or an aggregate member default
	// assignment (the `@default` sugar).
	Value     *Node
	Elided    bool
	Index     *int
	Source    SourceLocation
	Doc       string
	DocSource SourceLocation
}

// Target is a member target: a shape ID or, in IDL 2.1, an inline collection.
type Target struct {
	ID         string
	InlineList *InlineListTarget
	InlineMap  *InlineMapTarget
	Source     SourceLocation
}

func (t Target) IsInline() bool {
	return t.InlineList != nil || t.InlineMap != nil
}

// InlineListTarget is an inline list target such as [String].
type InlineListTarget struct {
	Element Target
	Source  SourceLocation
}

// InlineMapTarget is an inline map target such as {String: Integer}.
type InlineMapTarget struct {
	Key    Target
	Value  Target
	Source SourceLocation
}

// Trait is an applied but unresolved trait.
type Trait struct {
	Name       string
	Value      Node
	Source     SourceLocation
	Annotation bool
}

// ApplyStmt applies traits to an already-defined shape.
type ApplyStmt struct {
	Target ShapeRef
	Traits []Trait
	Source SourceLocation
}

func (*ApplyStmt) stmt()                      {}
func (a *ApplyStmt) Location() SourceLocation { return a.Source }

// NodeKind is the kind of a Smithy node value.
type NodeKind int

const (
	NodeNull NodeKind = iota
	NodeBoolean
	NodeNumber
	NodeString
	NodeArray
	NodeObject
	NodeShapeID
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
	case NodeShapeID:
		return "shapeId"
	default:
		return fmt.Sprintf("NodeKind(%d)", int(k))
	}
}

// Node is a Smithy node value. The zero value is a null node.
type Node struct {
	Kind   NodeKind
	Str    string
	Num    string
	Bool   bool
	Arr    []Node
	Obj    []ObjectEntry
	Source SourceLocation
	// SyntacticShapeID is set for unquoted identifiers, which are shape ID
	// references rather than strings.
	SyntacticShapeID bool
	// Tag records the tag name for a 2.1 tagged string literal.
	Tag string
}

// ObjectEntry is one key/value pair of an object node, preserving source order.
type ObjectEntry struct {
	Key       string
	KeySource SourceLocation
	Value     Node
}

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

// String returns a human-readable representation of the node.
func (n Node) String() string {
	switch n.Kind {
	case NodeNull:
		return "null"
	case NodeBoolean:
		if n.Bool {
			return "true"
		}
		return "false"
	case NodeNumber:
		return n.Num
	case NodeString:
		return n.Str
	case NodeShapeID:
		return n.Str
	case NodeArray:
		return fmt.Sprintf("%v", n.Arr)
	case NodeObject:
		return fmt.Sprintf("%v", n.Obj)
	default:
		return fmt.Sprintf("NodeKind(%d)", int(n.Kind))
	}
}
