package idl

import (
	"encoding/json"
	"strings"
)

// jsonASTMaxDepth matches the reference implementation's JSON nesting limit.
const jsonASTMaxDepth = 1000

// ParseJSONAST parses a Smithy JSON AST model document: the JSON model format
// produced by smithy-build and the Smithy model serializer.
//
// JSON AST shape IDs are absolute and there are no `use` statements, so the
// returned File has an empty namespace. Assembly, trait coercion, mixin
// flattening, and validation are unchanged: the result feeds the same
// model.Assemble path as an IDL file.
func ParseJSONAST(filename string, src []byte) (*File, error) {
	p := &jsonASTParser{filename: filename, src: src, line: 1, column: 1}
	root, err := p.parseValue(0)
	if err != nil {
		return nil, err
	}
	if err := p.expectEOF(); err != nil {
		return nil, err
	}
	return buildJSONASTFile(filename, src, root)
}

type jsonASTParser struct {
	filename string
	src      []byte
	pos      int
	line     int
	column   int
}

func (p *jsonASTParser) location(start, startLine, startColumn int) SourceLocation {
	return SourceLocation{
		Filename: p.filename,
		Line:     startLine,
		Column:   startColumn,
		Start:    start,
		End:      p.pos,
	}
}

func (p *jsonASTParser) currentLocation() SourceLocation {
	return p.location(p.pos, p.line, p.column)
}

func (p *jsonASTParser) advance() byte {
	c := p.src[p.pos]
	p.pos++
	if c == '\n' {
		p.line++
		p.column = 1
	} else {
		p.column++
	}
	return c
}

func (p *jsonASTParser) skipWhitespace() {
	for p.pos < len(p.src) {
		switch p.src[p.pos] {
		case ' ', '\t', '\n', '\r':
			p.advance()
		default:
			return
		}
	}
}

func (p *jsonASTParser) expectEOF() error {
	p.skipWhitespace()
	if p.pos != len(p.src) {
		return syntaxError(p.currentLocation(), "Unexpected trailing content")
	}
	return nil
}

func (p *jsonASTParser) parseValue(depth int) (Node, error) {
	if depth > jsonASTMaxDepth {
		return Node{}, syntaxError(p.currentLocation(), "maximum JSON nesting depth exceeded")
	}
	p.skipWhitespace()
	if p.pos >= len(p.src) {
		return Node{}, syntaxError(p.currentLocation(), "Unexpected end of JSON input")
	}

	start, startLine, startColumn := p.pos, p.line, p.column
	var node Node
	var err error
	switch c := p.src[p.pos]; c {
	case '{':
		node, err = p.parseObject(depth)
	case '[':
		node, err = p.parseArray(depth)
	case '"':
		var s string
		s, err = p.parseString()
		node = Node{Kind: NodeString, Str: s}
	case 't':
		err = p.parseLiteral("true")
		node = Node{Kind: NodeBoolean, Bool: true}
	case 'f':
		err = p.parseLiteral("false")
		node = Node{Kind: NodeBoolean, Bool: false}
	case 'n':
		err = p.parseLiteral("null")
		node = Node{Kind: NodeNull}
	default:
		if c == '-' || (c >= '0' && c <= '9') {
			var num string
			num, err = p.parseNumber()
			node = Node{Kind: NodeNumber, Num: num}
		} else {
			return Node{}, syntaxError(p.currentLocation(), "Unexpected character %q", c)
		}
	}
	if err != nil {
		return Node{}, err
	}
	node.Source = p.location(start, startLine, startColumn)
	return node, nil
}

func (p *jsonASTParser) parseObject(depth int) (Node, error) {
	p.advance() // '{'
	node := Node{Kind: NodeObject}
	seen := make(map[string]bool)
	p.skipWhitespace()
	if p.pos < len(p.src) && p.src[p.pos] == '}' {
		p.advance()
		return node, nil
	}
	for {
		p.skipWhitespace()
		if p.pos >= len(p.src) {
			return Node{}, syntaxError(p.currentLocation(), "Unexpected end of JSON input in object")
		}
		if p.src[p.pos] != '"' {
			return Node{}, syntaxError(p.currentLocation(), "Expected an object key string")
		}
		keyStart, keyLine, keyColumn := p.pos, p.line, p.column
		key, err := p.parseString()
		if err != nil {
			return Node{}, err
		}
		keyLocation := p.location(keyStart, keyLine, keyColumn)
		if seen[key] {
			return Node{}, syntaxError(keyLocation, "Duplicate object key %q", key)
		}
		seen[key] = true

		p.skipWhitespace()
		if p.pos >= len(p.src) || p.src[p.pos] != ':' {
			return Node{}, syntaxError(p.currentLocation(), "Expected ':' after object key %q", key)
		}
		p.advance()
		value, err := p.parseValue(depth + 1)
		if err != nil {
			return Node{}, err
		}
		node.Obj = append(node.Obj, ObjectEntry{Key: key, KeySource: keyLocation, Value: value})

		p.skipWhitespace()
		if p.pos >= len(p.src) {
			return Node{}, syntaxError(p.currentLocation(), "Unexpected end of JSON input in object")
		}
		switch p.src[p.pos] {
		case ',':
			p.advance()
		case '}':
			p.advance()
			return node, nil
		default:
			return Node{}, syntaxError(p.currentLocation(), "Expected ',' or '}' in object")
		}
	}
}

func (p *jsonASTParser) parseArray(depth int) (Node, error) {
	p.advance() // '['
	node := Node{Kind: NodeArray}
	p.skipWhitespace()
	if p.pos < len(p.src) && p.src[p.pos] == ']' {
		p.advance()
		return node, nil
	}
	for {
		value, err := p.parseValue(depth + 1)
		if err != nil {
			return Node{}, err
		}
		node.Arr = append(node.Arr, value)

		p.skipWhitespace()
		if p.pos >= len(p.src) {
			return Node{}, syntaxError(p.currentLocation(), "Unexpected end of JSON input in array")
		}
		switch p.src[p.pos] {
		case ',':
			p.advance()
		case ']':
			p.advance()
			return node, nil
		default:
			return Node{}, syntaxError(p.currentLocation(), "Expected ',' or ']' in array")
		}
	}
}

func (p *jsonASTParser) parseString() (string, error) {
	start, startLine, startColumn := p.pos, p.line, p.column
	p.advance() // opening quote
	for {
		if p.pos >= len(p.src) {
			return "", syntaxError(p.currentLocation(), "Unterminated string")
		}
		switch c := p.src[p.pos]; {
		case c == '"':
			p.advance()
			var out string
			if err := json.Unmarshal(p.src[start:p.pos], &out); err != nil {
				return "", syntaxError(p.location(start, startLine, startColumn), "Invalid string literal: %v", err)
			}
			return out, nil
		case c == '\\':
			p.advance()
			if p.pos >= len(p.src) {
				return "", syntaxError(p.currentLocation(), "Unterminated string escape")
			}
			if p.src[p.pos] == 'u' {
				p.advance()
				for i := 0; i < 4; i++ {
					if p.pos >= len(p.src) {
						return "", syntaxError(p.currentLocation(), "Unterminated unicode escape")
					}
					p.advance()
				}
			} else {
				p.advance()
			}
		case c < 0x20:
			return "", syntaxError(p.currentLocation(), "Control character in string")
		default:
			p.advance()
		}
	}
}

func (p *jsonASTParser) parseLiteral(literal string) error {
	if len(p.src)-p.pos < len(literal) || string(p.src[p.pos:p.pos+len(literal)]) != literal {
		return syntaxError(p.currentLocation(), "Expected %q", literal)
	}
	for range literal {
		p.advance()
	}
	return nil
}

func (p *jsonASTParser) parseNumber() (string, error) {
	start := p.pos
	if p.src[p.pos] == '-' {
		p.advance()
	}
	if p.pos >= len(p.src) {
		return "", syntaxError(p.currentLocation(), "Unexpected end of number")
	}
	switch c := p.src[p.pos]; {
	case c == '0':
		p.advance()
	case c >= '1' && c <= '9':
		for p.pos < len(p.src) && isJSONDigit(p.src[p.pos]) {
			p.advance()
		}
	default:
		return "", syntaxError(p.currentLocation(), "Invalid number")
	}
	if p.pos < len(p.src) && p.src[p.pos] == '.' {
		p.advance()
		if p.pos >= len(p.src) || !isJSONDigit(p.src[p.pos]) {
			return "", syntaxError(p.currentLocation(), "Invalid number: expected a digit after the decimal point")
		}
		for p.pos < len(p.src) && isJSONDigit(p.src[p.pos]) {
			p.advance()
		}
	}
	if p.pos < len(p.src) && (p.src[p.pos] == 'e' || p.src[p.pos] == 'E') {
		p.advance()
		if p.pos < len(p.src) && (p.src[p.pos] == '+' || p.src[p.pos] == '-') {
			p.advance()
		}
		if p.pos >= len(p.src) || !isJSONDigit(p.src[p.pos]) {
			return "", syntaxError(p.currentLocation(), "Invalid number: expected an exponent")
		}
		for p.pos < len(p.src) && isJSONDigit(p.src[p.pos]) {
			p.advance()
		}
	}
	return string(p.src[start:p.pos]), nil
}

func isJSONDigit(c byte) bool { return c >= '0' && c <= '9' }

// --- JSON AST to IDL AST ---------------------------------------------------

type jsonASTBuilder struct {
	file *File
}

func buildJSONASTFile(filename string, src []byte, root Node) (*File, error) {
	if root.Kind != NodeObject {
		return nil, modelError(root.Source, "Expected a JSON object at the document root, but found %s", root.Kind)
	}

	var versionNode Node
	hasVersion := false
	for _, entry := range root.Obj {
		if entry.Key == "smithy" {
			versionNode = entry.Value
			hasVersion = true
		}
	}
	if !hasVersion {
		return nil, modelError(root.Source, "Not a Smithy JSON AST model: missing the `smithy` version property")
	}
	if versionNode.Kind != NodeString {
		return nil, modelError(versionNode.Source, "Expected `smithy` to be a string, but found %s", versionNode.Kind)
	}
	version, ok := resolveVersion(versionNode.Str, V2_1)
	if !ok {
		return nil, modelError(versionNode.Source, "Unsupported Smithy version number: %s", versionNode.Str)
	}

	file := &File{
		Path:    filename,
		Version: version,
		Source: SourceLocation{
			Filename: filename,
			Line:     1,
			Column:   1,
			Start:    0,
			End:      len(src),
		},
	}
	builder := &jsonASTBuilder{file: file}
	for _, entry := range root.Obj {
		switch entry.Key {
		case "smithy":
			// Already handled; it can appear anywhere in the object.
		case "metadata":
			if err := builder.addMetadata(entry.Value); err != nil {
				return nil, err
			}
		case "shapes":
			if err := builder.addShapes(entry.Value); err != nil {
				return nil, err
			}
		}
	}
	return file, nil
}

func (b *jsonASTBuilder) addMetadata(node Node) error {
	if node.Kind != NodeObject {
		return modelError(node.Source, "Expected `metadata` to be an object, but found %s", node.Kind)
	}
	for _, entry := range node.Obj {
		b.file.Metadata = append(b.file.Metadata, Metadata{
			Key:    entry.Key,
			Value:  entry.Value,
			Source: entry.Value.Source,
		})
	}
	return nil
}

func (b *jsonASTBuilder) addShapes(node Node) error {
	if node.Kind != NodeObject {
		return modelError(node.Source, "Expected `shapes` to be an object, but found %s", node.Kind)
	}
	for _, entry := range node.Obj {
		stmt, err := b.shape(entry)
		if err != nil {
			return err
		}
		b.file.Shapes = append(b.file.Shapes, stmt)
	}
	return nil
}

func (b *jsonASTBuilder) shape(entry ObjectEntry) (Stmt, error) {
	if entry.Value.Kind != NodeObject {
		return nil, modelError(entry.Value.Source, "Expected the definition of `%s` to be an object, but found %s", entry.Key, entry.Value.Kind)
	}
	shapeType, err := stringProperty(entry.Value, "type")
	if err != nil {
		return nil, err
	}
	if shapeType.Kind == NodeString && shapeType.Str == "apply" {
		traits, err := traitsProperty(entry.Value)
		if err != nil {
			return nil, err
		}
		return &ApplyStmt{
			Target: ShapeRef{ID: entry.Key, Source: entry.KeySource},
			Traits: traits,
			Source: entry.Value.Source,
		}, nil
	}
	if shapeType.Kind != NodeString {
		return nil, modelError(shapeType.Source, "Expected `type` to be a string, but found %s", shapeType.Kind)
	}

	kind, ok := jsonShapeKind(shapeType.Str)
	if !ok {
		return nil, modelError(shapeType.Source, "Invalid shape `type`: %s", shapeType.Str)
	}
	if err := checkAbsoluteShapeID(entry.Key, entry.KeySource); err != nil {
		return nil, err
	}

	stmt := &ShapeStmt{
		Kind:   kind,
		Name:   shapeName(entry.Key),
		ID:     entry.Key,
		Source: entry.Value.Source,
	}
	traits, err := traitsProperty(entry.Value)
	if err != nil {
		return nil, err
	}
	stmt.Traits = traits

	mixins, err := b.mixinsProperty(entry.Value)
	if err != nil {
		return nil, err
	}
	stmt.Mixins = mixins

	switch kind {
	case KindStructure, KindUnion, KindEnum, KindIntEnum:
		members, err := b.members(entry.Value)
		if err != nil {
			return nil, err
		}
		stmt.Members = members
	case KindList, KindSet:
		member, err := b.collectionMember(entry.Value, "member", "member")
		if err != nil {
			return nil, err
		}
		if member != nil {
			stmt.Members = []Member{*member}
		}
	case KindMap:
		key, err := b.collectionMember(entry.Value, "key", "key")
		if err != nil {
			return nil, err
		}
		value, err := b.collectionMember(entry.Value, "value", "value")
		if err != nil {
			return nil, err
		}
		if key != nil {
			stmt.Members = append(stmt.Members, *key)
		}
		if value != nil {
			stmt.Members = append(stmt.Members, *value)
		}
	case KindService:
		body, err := b.serviceBody(entry.Value)
		if err != nil {
			return nil, err
		}
		stmt.Body = body
	case KindResource:
		body, err := b.resourceBody(entry.Value)
		if err != nil {
			return nil, err
		}
		stmt.Body = body
	case KindOperation:
		if err := b.operationProperties(stmt, entry.Value); err != nil {
			return nil, err
		}
	}
	return stmt, nil
}

func (b *jsonASTBuilder) members(container Node) ([]Member, error) {
	node, ok := container.Get("members")
	if !ok {
		return nil, nil
	}
	if node.Kind != NodeObject {
		return nil, modelError(node.Source, "Expected `members` to be an object, but found %s", node.Kind)
	}
	members := make([]Member, 0, len(node.Obj))
	for _, entry := range node.Obj {
		member, err := b.member(entry.Key, entry.Value)
		if err != nil {
			return nil, err
		}
		members = append(members, member)
	}
	return members, nil
}

func (b *jsonASTBuilder) collectionMember(container Node, property, name string) (*Member, error) {
	node, ok := container.Get(property)
	if !ok {
		return nil, nil
	}
	member, err := b.member(name, node)
	if err != nil {
		return nil, err
	}
	return &member, nil
}

func (b *jsonASTBuilder) member(name string, node Node) (Member, error) {
	if node.Kind != NodeObject {
		return Member{}, modelError(node.Source, "Expected the definition of member `%s` to be an object, but found %s", name, node.Kind)
	}
	target, err := stringProperty(node, "target")
	if err != nil {
		return Member{}, err
	}
	if target.Kind != NodeString {
		return Member{}, modelError(target.Source, "Expected `target` to be a string, but found %s", target.Kind)
	}
	if err := checkAbsoluteShapeID(target.Str, target.Source); err != nil {
		return Member{}, err
	}
	traits, err := traitsProperty(node)
	if err != nil {
		return Member{}, err
	}
	member := Member{
		Name:   name,
		Target: Target{ID: target.Str, Source: target.Source},
		Source: node.Source,
	}
	member.Traits = traits
	return member, nil
}

func (b *jsonASTBuilder) operationProperties(stmt *ShapeStmt, node Node) error {
	for _, property := range []string{"input", "output"} {
		value, ok := node.Get(property)
		if !ok {
			continue
		}
		target, err := referenceTarget(value)
		if err != nil {
			return err
		}
		io := &InlineIO{
			Target: &ShapeRef{ID: target, Source: value.Source},
			Source: value.Source,
		}
		if property == "input" {
			stmt.Input = io
		} else {
			stmt.Output = io
		}
	}
	errors, err := referenceList(node, "errors")
	if err != nil {
		return err
	}
	stmt.Errors = errors
	return nil
}

func (b *jsonASTBuilder) serviceBody(node Node) (*Node, error) {
	body := Node{Kind: NodeObject, Source: node.Source}
	if version, ok := node.Get("version"); ok {
		if version.Kind != NodeString {
			return nil, modelError(version.Source, "Expected `version` to be a string, but found %s", version.Kind)
		}
		body.Obj = append(body.Obj, ObjectEntry{Key: "version", Value: version})
	}
	for _, property := range []string{"operations", "resources", "errors"} {
		refs, err := referenceArray(node, property)
		if err != nil {
			return nil, err
		}
		if refs != nil {
			body.Obj = append(body.Obj, ObjectEntry{Key: property, Value: *refs})
		}
	}
	if rename, ok := node.Get("rename"); ok {
		if rename.Kind != NodeObject {
			return nil, modelError(rename.Source, "Expected `rename` to be an object, but found %s", rename.Kind)
		}
		out := Node{Kind: NodeObject, Source: rename.Source}
		for _, entry := range rename.Obj {
			if err := checkAbsoluteShapeID(entry.Key, entry.KeySource); err != nil {
				return nil, err
			}
			if entry.Value.Kind != NodeString {
				return nil, modelError(entry.Value.Source, "Expected the rename value for `%s` to be a string, but found %s", entry.Key, entry.Value.Kind)
			}
			out.Obj = append(out.Obj, entry)
		}
		body.Obj = append(body.Obj, ObjectEntry{Key: "rename", Value: out})
	}
	return &body, nil
}

func (b *jsonASTBuilder) resourceBody(node Node) (*Node, error) {
	body := Node{Kind: NodeObject, Source: node.Source}
	for _, property := range []string{"create", "read", "update", "delete", "list", "put"} {
		value, ok := node.Get(property)
		if !ok {
			continue
		}
		target, err := referenceTarget(value)
		if err != nil {
			return nil, err
		}
		body.Obj = append(body.Obj, ObjectEntry{
			Key:   property,
			Value: Node{Kind: NodeString, Str: target, Source: value.Source},
		})
	}
	for _, property := range []string{"identifiers", "properties"} {
		value, ok := node.Get(property)
		if !ok {
			continue
		}
		if value.Kind != NodeObject {
			return nil, modelError(value.Source, "Expected `%s` to be an object, but found %s", property, value.Kind)
		}
		if property == "properties" && !b.file.Version.AtLeast(V2_0) {
			return nil, modelError(value.Source,
				"Resource properties can only be used with Smithy version 2 or later. Attempted to use resource properties with version `%s`.",
				b.file.Version)
		}
		out := Node{Kind: NodeObject, Source: value.Source}
		for _, entry := range value.Obj {
			target, err := referenceTarget(entry.Value)
			if err != nil {
				return nil, err
			}
			out.Obj = append(out.Obj, ObjectEntry{
				Key:       entry.Key,
				KeySource: entry.KeySource,
				Value:     Node{Kind: NodeString, Str: target, Source: entry.Value.Source},
			})
		}
		body.Obj = append(body.Obj, ObjectEntry{Key: property, Value: out})
	}
	for _, property := range []string{"operations", "collectionOperations", "resources"} {
		refs, err := referenceArray(node, property)
		if err != nil {
			return nil, err
		}
		if refs != nil {
			body.Obj = append(body.Obj, ObjectEntry{Key: property, Value: *refs})
		}
	}
	return &body, nil
}

func (b *jsonASTBuilder) mixinsProperty(node Node) ([]ShapeRef, error) {
	if _, ok := node.Get("mixins"); !ok {
		return nil, nil
	}
	return referenceList(node, "mixins")
}

func traitsProperty(node Node) ([]Trait, error) {
	value, ok := node.Get("traits")
	if !ok {
		return nil, nil
	}
	if value.Kind != NodeObject {
		return nil, modelError(value.Source, "Expected `traits` to be an object, but found %s", value.Kind)
	}
	traits := make([]Trait, 0, len(value.Obj))
	for _, entry := range value.Obj {
		if err := checkAbsoluteShapeID(entry.Key, entry.KeySource); err != nil {
			return nil, err
		}
		traits = append(traits, Trait{
			Name:   entry.Key,
			Value:  entry.Value,
			Source: entry.KeySource,
		})
	}
	return traits, nil
}

func referenceList(node Node, property string) ([]ShapeRef, error) {
	value, ok := node.Get(property)
	if !ok {
		return nil, nil
	}
	if value.Kind != NodeArray {
		return nil, modelError(value.Source, "Expected `%s` to be an array, but found %s", property, value.Kind)
	}
	refs := make([]ShapeRef, 0, len(value.Arr))
	for _, item := range value.Arr {
		target, err := referenceTarget(item)
		if err != nil {
			return nil, err
		}
		refs = append(refs, ShapeRef{ID: target, Source: item.Source})
	}
	return refs, nil
}

func referenceArray(node Node, property string) (*Node, error) {
	refs, ok := node.Get(property)
	if !ok {
		return nil, nil
	}
	if refs.Kind != NodeArray {
		return nil, modelError(refs.Source, "Expected `%s` to be an array, but found %s", property, refs.Kind)
	}
	out := Node{Kind: NodeArray, Source: refs.Source}
	for _, item := range refs.Arr {
		target, err := referenceTarget(item)
		if err != nil {
			return nil, err
		}
		out.Arr = append(out.Arr, Node{Kind: NodeString, Str: target, Source: item.Source})
	}
	return &out, nil
}

// referenceTarget reads a {"target": "ns#Shape"} reference object.
func referenceTarget(node Node) (string, error) {
	if node.Kind != NodeObject {
		return "", modelError(node.Source, "Expected a shape reference object, but found %s", node.Kind)
	}
	target, err := stringProperty(node, "target")
	if err != nil {
		return "", err
	}
	if target.Kind != NodeString {
		return "", modelError(target.Source, "Expected `target` to be a string, but found %s", target.Kind)
	}
	if err := checkAbsoluteShapeID(target.Str, target.Source); err != nil {
		return "", err
	}
	return target.Str, nil
}

func stringProperty(node Node, name string) (Node, error) {
	value, ok := node.Get(name)
	if !ok {
		return Node{}, modelError(node.Source, "Missing expected member `%s`.", name)
	}
	return value, nil
}

func checkAbsoluteShapeID(id string, location SourceLocation) error {
	hash := strings.IndexByte(id, '#')
	if hash <= 0 || hash == len(id)-1 || strings.IndexByte(id[hash+1:], '#') >= 0 {
		return modelError(location, "Expected an absolute shape ID, but found `%s`", id)
	}
	return nil
}

func shapeName(id string) string {
	name := id
	if i := strings.LastIndexByte(id, '#'); i >= 0 {
		name = id[i+1:]
	}
	if member := strings.IndexByte(name, '$'); member >= 0 {
		name = name[:member]
	}
	return name
}

func jsonShapeKind(typ string) (ShapeKind, bool) {
	switch typ {
	case "blob":
		return KindBlob, true
	case "boolean":
		return KindBoolean, true
	case "document":
		return KindDocument, true
	case "string":
		return KindString, true
	case "byte":
		return KindByte, true
	case "short":
		return KindShort, true
	case "integer":
		return KindInteger, true
	case "long":
		return KindLong, true
	case "float":
		return KindFloat, true
	case "double":
		return KindDouble, true
	case "bigInteger":
		return KindBigInteger, true
	case "bigDecimal":
		return KindBigDecimal, true
	case "timestamp":
		return KindTimestamp, true
	case "enum":
		return KindEnum, true
	case "intEnum":
		return KindIntEnum, true
	case "list":
		return KindList, true
	case "set":
		return KindSet, true
	case "map":
		return KindMap, true
	case "structure":
		return KindStructure, true
	case "union":
		return KindUnion, true
	case "service":
		return KindService, true
	case "resource":
		return KindResource, true
	case "operation":
		return KindOperation, true
	default:
		return "", false
	}
}
