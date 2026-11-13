package idl

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

const maxNestingLevel = 64
const maxInlineCollectionDepth = 3

// Parse parses a Smithy IDL file using the default parser options.
func Parse(filename, src string) (*File, error) {
	return ParseWithOptions(filename, src, Options{})
}

// ParseWithOptions parses a Smithy IDL file.
func ParseWithOptions(filename, src string, opts Options) (file *File, err error) {
	p := &parser{
		filename:              filename,
		src:                   src,
		t:                     newTokenizer(filename, src),
		opts:                  opts,
		version:               opts.defaultVersion(),
		useNames:              make(map[string]string),
		definedControls:       make(map[string]bool),
		operationInputSuffix:  "Input",
		operationOutputSuffix: "Output",
	}
	p.file = &File{Path: filename, Source: SourceLocation{
		Filename: filename,
		Line:     1,
		Column:   1,
		Start:    0,
		End:      len(src),
	}}

	defer func() {
		if r := recover(); r != nil {
			if pe, ok := r.(*ParseError); ok {
				file = nil
				err = pe
				return
			}
			panic(r)
		}
	}()

	p.parse()
	return p.file, nil
}

type parser struct {
	filename string
	src      string
	t        *tokenizer
	tok      Token
	file     *File
	opts     Options

	version         Version
	emittedVer      bool
	namespace       string
	useNames        map[string]string
	definedControls map[string]bool
	pendingDocs     []string
	pendingDocSrc   SourceLocation

	nesting int
	lastEnd int

	operationInputSuffix  string
	operationOutputSuffix string
}

type traitResult struct {
	name       string
	value      Node
	source     SourceLocation
	annotation bool
}

func (p *parser) parse() {
	p.advance() // prime the token stream
	p.skipWsAndDocs()
	p.parseControlSection()

	if !p.emittedVer {
		p.version = p.opts.defaultVersion()
	}
	p.file.Version = p.version

	p.skipWsAndDocs()
	p.parseMetadataSection()
	p.parseShapeSection()
}

// --- token helpers ---------------------------------------------------------

func (p *parser) advance() {
	p.tok = p.t.next()
	if p.tok.Kind == TokenDocComment {
		if len(p.pendingDocs) == 0 {
			p.pendingDocSrc = p.tok.Source
		}
		p.pendingDocs = append(p.pendingDocs, docCommentText(p.src, p.tok))
	}
}

func (p *parser) loc() SourceLocation { return p.tok.Source }

func (p *parser) fail(format string, args ...any) {
	panic(syntaxError(p.loc(), format, args...))
}

func (p *parser) failAt(loc SourceLocation, prefixed bool, format string, args ...any) {
	if prefixed {
		panic(syntaxError(loc, format, args...))
	}
	panic(modelError(loc, format, args...))
}

func (p *parser) failShapeSyntax(shapeID string, format string, args ...any) {
	err := syntaxError(p.loc(), format, args...)
	err.ShapeID = shapeID
	panic(err)
}

func (p *parser) expect(kinds ...TokenKind) Token {
	for _, k := range kinds {
		if p.tok.Kind == k {
			return p.tok
		}
	}

	if p.tok.Kind == TokenError {
		panic(syntaxError(p.loc(), "%s", p.tok.Err))
	}

	var msg string
	if len(kinds) == 1 {
		msg = fmt.Sprintf("Expected %s but found %s",
			kinds[0].debug(kinds[0].exampleLexeme()), p.tok.debug())
	} else {
		var b strings.Builder
		b.WriteString("Expected one of ")
		for i, k := range kinds {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(k.debug(k.exampleLexeme()))
		}
		b.WriteString("; but found ")
		b.WriteString(p.tok.debug())
		msg = b.String()
	}
	panic(syntaxError(p.loc(), "%s", msg))
}

func (p *parser) skipSpaces() {
	for p.tok.Kind == TokenSpace {
		p.advance()
	}
}

func (p *parser) skipOptionalComma() {
	if p.tok.Kind == TokenComma {
		p.advance()
	}
}

func (p *parser) skipWs() {
	for p.tok.Kind.isWhitespace() {
		p.advance()
	}
}

func (p *parser) skipWsAndDocs() {
	for p.tok.Kind.isWhitespace() || p.tok.Kind == TokenDocComment {
		p.advance()
	}
}

func (p *parser) expectAndSkipSpaces() {
	p.expect(TokenSpace)
	p.skipSpaces()
}

func (p *parser) expectAndSkipWhitespace() {
	if !p.tok.Kind.isWhitespace() {
		p.fail("Expected one or more whitespace characters, but found %s", p.tok.debug())
	}
	p.skipWsAndDocs()
}

// expectAndSkipBr implements Smithy's br production.
func (p *parser) expectAndSkipBr() {
	p.skipSpaces()
	p.clearPendingDocs()

	switch p.tok.Kind {
	case TokenNewline, TokenComment, TokenDocComment:
		p.advance()
		p.skipWs()
	case TokenEOF:
		return
	default:
		p.fail("Expected a line break, but found %s", p.tok.debug())
	}
}

func (p *parser) clearPendingDocs() {
	p.pendingDocs = nil
	p.pendingDocSrc = SourceLocation{}
}

func (p *parser) removePendingDocs(fallback SourceLocation) (string, SourceLocation, bool) {
	if len(p.pendingDocs) == 0 {
		return "", SourceLocation{}, false
	}
	doc := strings.Join(p.pendingDocs, "\n")
	src := p.pendingDocSrc
	if src.isZero() {
		src = fallback
	}
	p.clearPendingDocs()
	return doc, src, true
}

func (p *parser) increaseNesting() {
	p.nesting++
	if p.nesting > maxNestingLevel {
		p.fail("Parser exceeded maximum allowed depth of %d", maxNestingLevel)
	}
}

func (p *parser) decreaseNesting() { p.nesting-- }

// --- control section -------------------------------------------------------

func (p *parser) parseControlSection() {
	for p.tok.Kind == TokenDollar {
		p.advance()
		p.expect(TokenIdentifier, TokenString)
		key := p.tokenText()
		p.advance()
		p.skipSpaces()
		p.expect(TokenColon)
		p.advance()
		p.skipSpaces()

		if p.definedControls[key] {
			p.fail("Duplicate control statement `%s`", key)
		}
		p.definedControls[key] = true

		value := p.parseNode()
		p.file.Controls = append(p.file.Controls, Control{Name: key, Value: value, Source: value.Source})

		switch key {
		case "version":
			p.onVersion(value)
		case "operationInputSuffix":
			if value.Kind != NodeString {
				p.failAt(value.Source, false,
					"The $operationInputSuffix control statement must have a string value, but found %s", nodeRepr(value))
			}
			p.operationInputSuffix = value.Str
		case "operationOutputSuffix":
			if value.Kind != NodeString {
				p.failAt(value.Source, false,
					"The $operationOutputSuffix control statement must have a string value, but found %s", nodeRepr(value))
			}
			p.operationOutputSuffix = value.Str
		}

		p.expectAndSkipBr()
	}
}

func (p *parser) onVersion(value Node) {
	if value.Kind != NodeString {
		p.failAt(value.Source, false,
			"The $version control statement must have a string value, but found %s", nodeRepr(value))
	}

	resolved, ok := resolveVersion(value.Str, p.opts.maxVersion())
	if !ok {
		// The reference implementation reports this after consuming the node,
		// which points at the token following the value.
		p.fail("Unsupported Smithy version number: %s", value.Str)
	}

	p.emittedVer = true
	p.version = resolved
}

func resolveVersion(s string, max Version) (Version, bool) {
	switch s {
	case "1.0":
		return V1_0, true
	case "2.0":
		if max.atLeast(V2_0) {
			return V2_0, true
		}
		return Version{}, false
	case "2.1":
		if max.atLeast(V2_1) {
			return V2_1, true
		}
		return Version{}, false
	case "1":
		return V1_0, true
	case "2":
		if max.Major == 2 {
			return max, true
		}
		return Version{}, false
	default:
		return Version{}, false
	}
}

func (p *parser) tokenText() string {
	if p.tok.Kind == TokenString {
		return p.tok.StringValue
	}
	return p.tok.Lexeme
}

func nodeRepr(n Node) string {
	switch n.Kind {
	case NodeString:
		return strconv.Quote(n.Str)
	case NodeNumber:
		return n.Num
	case NodeBoolean:
		if n.Bool {
			return "true"
		}
		return "false"
	case NodeNull:
		return "null"
	default:
		return n.String()
	}
}

// --- metadata section ------------------------------------------------------

func (p *parser) parseMetadataSection() {
	for p.isCurrentLexeme("metadata") {
		p.advance()
		p.expectAndSkipSpaces()
		p.expect(TokenIdentifier, TokenString)
		key := p.tokenText()
		loc := p.loc()
		p.advance()
		p.skipSpaces()
		p.expect(TokenEqual)
		p.advance()
		p.skipSpaces()
		value := p.parseNode()
		p.file.Metadata = append(p.file.Metadata, Metadata{Key: key, Value: value, Source: loc})
		p.expectAndSkipBr()
		p.skipWsAndDocs()
	}
}

func (p *parser) isCurrentLexeme(s string) bool {
	return p.tok.Kind == TokenIdentifier && p.tok.Lexeme == s
}

// --- shape section ---------------------------------------------------------

func (p *parser) parseShapeSection() {
	if p.isCurrentLexeme("namespace") {
		p.advance()
		p.expectAndSkipSpaces()
		p.namespace = p.parseNamespace()
		p.file.Namespace = p.namespace
		p.expectAndSkipBr()

		possibleDocLoc := p.loc()
		p.skipWsAndDocs()
		p.parseUseSection()
		p.parseFirstShapeStatement(possibleDocLoc)
		p.parseSubsequentShapeStatements()
		return
	}

	if p.tok.Kind != TokenEOF {
		p.fail("Expected a namespace definition but found %s", p.tok.debug())
	}
}

func (p *parser) parseUseSection() {
	for p.isCurrentLexeme("use") {
		p.advance()
		p.expectAndSkipSpaces()

		loc := p.loc()
		id := p.parseAbsoluteShapeID()
		if strings.Contains(id, "$") {
			p.failAt(loc, false, "Use statements cannot use members")
		}

		name := id
		if i := strings.LastIndexByte(id, '#'); i >= 0 {
			name = id[i+1:]
		}
		if previous, ok := p.useNames[name]; ok {
			p.failAt(loc, false, "Cannot use name `%s` because it conflicts with `%s`", id, previous)
		}
		p.useNames[name] = id
		p.file.Uses = append(p.file.Uses, Use{ID: id, Source: loc})

		p.expectAndSkipBr()
		p.skipWsAndDocs()
	}
}

func (p *parser) parseFirstShapeStatement(possibleDocLoc SourceLocation) {
	if p.tok.Kind == TokenEOF {
		return
	}

	p.skipWsAndDocs()
	doc, docSrc, hasDoc := p.removePendingDocs(possibleDocLoc)
	traits, traitDoc, traitDocSrc, traitHasDoc := p.parseDocsAndTraitsBeforeShape(false)
	if traitHasDoc {
		doc, docSrc, hasDoc = traitDoc, traitDocSrc, true
	}

	if p.parseShapeDefinition(traits, hasDoc) {
		p.parseShapeOrApply(traits, doc, docSrc)
	}
}

func (p *parser) parseSubsequentShapeStatements() {
	for p.tok.Kind != TokenEOF {
		traits, doc, docSrc, hasDoc := p.parseDocsAndTraitsBeforeShape(false)
		if p.parseShapeDefinition(traits, hasDoc) {
			p.parseShapeOrApply(traits, doc, docSrc)
		}
	}
}

func (p *parser) parseShapeDefinition(traits []traitResult, hasDoc bool) bool {
	if p.tok.Kind != TokenEOF {
		return true
	}
	if hasDoc {
		if len(traits) == 0 {
			// Dangling documentation comments are a warning, not a parse
			// failure. M2 will surface the warning event.
			return false
		}
		return true
	}
	return len(traits) > 0
}

func (p *parser) parseShapeOrApply(traits []traitResult, doc string, docSrc SourceLocation) {
	loc := p.loc()
	p.expect(TokenIdentifier)
	shapeType := p.tok.Lexeme

	if shapeType == "apply" {
		p.parseApplyStatement(traits)
		return
	}

	kind, ok := shapeKinds[shapeType]
	if !ok {
		p.fail("Unknown shape type: %s", shapeType)
	}

	p.advance()
	p.expectAndSkipSpaces()
	name := p.parseShapeName()

	stmt := &ShapeStmt{
		Kind:      kind,
		Name:      name,
		ID:        p.shapeID(name),
		Source:    loc,
		Doc:       doc,
		DocSource: docSrc,
		Namespace: p.namespace,
	}

	switch kind {
	case KindString, KindBlob, KindBoolean, KindDocument, KindByte, KindShort,
		KindInteger, KindLong, KindFloat, KindDouble, KindBigInteger,
		KindBigDecimal, KindTimestamp:
		p.parseSimpleShape(stmt)
	case KindList, KindSet, KindMap, KindUnion, KindStructure:
		p.parseAggregateShape(stmt)
	case KindEnum, KindIntEnum:
		p.parseEnumShape(stmt)
	case KindService:
		p.parseServiceStatement(stmt)
	case KindResource:
		p.parseResourceStatement(stmt)
	case KindOperation:
		p.parseOperationStatement(stmt)
	default:
		p.fail("Shape type unknown: %s", shapeType)
	}

	for _, tr := range traits {
		stmt.Traits = append(stmt.Traits, traitResultToTrait(tr))
	}

	p.expectAndSkipBr()
	p.file.Shapes = append(p.file.Shapes, stmt)
}

var shapeKinds = map[string]ShapeKind{
	"blob":       KindBlob,
	"boolean":    KindBoolean,
	"document":   KindDocument,
	"string":     KindString,
	"byte":       KindByte,
	"short":      KindShort,
	"integer":    KindInteger,
	"long":       KindLong,
	"float":      KindFloat,
	"double":     KindDouble,
	"bigInteger": KindBigInteger,
	"bigDecimal": KindBigDecimal,
	"timestamp":  KindTimestamp,
	"enum":       KindEnum,
	"intEnum":    KindIntEnum,
	"list":       KindList,
	"set":        KindSet,
	"map":        KindMap,
	"structure":  KindStructure,
	"union":      KindUnion,
	"service":    KindService,
	"resource":   KindResource,
	"operation":  KindOperation,
}

func (p *parser) shapeID(name string) string {
	if p.namespace == "" {
		return name
	}
	return p.namespace + "#" + name
}

func (p *parser) parseShapeName() string {
	loc := p.loc()
	p.expect(TokenIdentifier)
	name := p.tok.Lexeme
	if previous, ok := p.useNames[name]; ok {
		p.failAt(loc, false, "Shape name `%s` conflicts with imported shape `%s`", name, previous)
	}
	p.advance()
	return name
}

func (p *parser) parseSimpleShape(stmt *ShapeStmt) {
	p.parseMixins(stmt)
}

func (p *parser) parseAggregateShape(stmt *ShapeStmt) {
	p.parseForResource(stmt)
	p.parseMixins(stmt)
	p.parseMembers(stmt)
}

func (p *parser) parseForResource(stmt *ShapeStmt) {
	p.skipSpaces()
	if !p.isCurrentLexeme("for") {
		return
	}

	if !p.version.atLeast(V2_0) {
		p.fail("Structures can only be bound to resources with Smithy version 2 or later. "+
			"Attempted to bind a structure to a resource with version `%s`.", p.version)
	}

	p.advance()
	p.expectAndSkipSpaces()
	stmt.ForResource = p.parseShapeID(false)
}

func (p *parser) parseMixins(stmt *ShapeStmt) {
	p.skipSpaces()
	if !p.isCurrentLexeme("with") {
		return
	}

	p.expect(TokenIdentifier)
	if !p.version.atLeast(V2_0) {
		p.fail("Mixins can only be used with Smithy version 2 or later. "+
			"Attempted to use mixins with version `%s`.", p.version)
	}

	p.advance()
	p.skipWsAndDocs()
	p.expect(TokenLBracket)
	p.advance()
	p.skipWsAndDocs()

	for {
		loc := p.loc()
		id := p.parseShapeID(false)
		stmt.Mixins = append(stmt.Mixins, ShapeRef{ID: id, Source: loc})
		p.skipWsAndDocs()
		if p.tok.Kind == TokenRBracket {
			break
		}
	}

	p.expect(TokenRBracket)
	p.advance()
}

func (p *parser) parseMembers(stmt *ShapeStmt) {
	p.skipWsAndDocs()
	p.clearPendingDocs()
	p.expect(TokenLBrace)
	p.advance()
	p.skipWs()

	for p.tok.Kind != TokenEOF && p.tok.Kind != TokenRBrace {
		if member, ok := p.parseMember(stmt); ok {
			stmt.Members = append(stmt.Members, member)
		}
		p.skipWs()
	}

	p.expect(TokenRBrace)
	p.advance()
}

func (p *parser) parseMember(parent *ShapeStmt) (Member, bool) {
	traits, doc, docSrc, hasDoc := p.parseDocsAndTraitsBeforeShape(true)
	loc := p.loc()

	if p.tok.Kind == TokenRBrace && hasDoc && len(traits) == 0 {
		// Dangling documentation comment before the closing brace. M2 will
		// surface the warning event.
		return Member{}, false
	}

	if p.tok.Kind == TokenRBrace {
		p.expect(TokenIdentifier)
	}

	index := p.parseOptionalMemberIndex()
	if index != nil {
		loc = p.loc()
	}

	elided := p.tok.Kind == TokenDollar
	if elided {
		p.expect(TokenDollar)
		p.advance()
	}

	p.expect(TokenIdentifier)
	memberName := p.tok.Lexeme
	if hasDuplicateMember(parent.Members, memberName) {
		p.fail("Duplicate member of `%s`: '%s'", parent.ID, memberName)
	}

	if elided && !p.version.atLeast(V2_0) {
		p.fail("Members can only elide targets in IDL version 2 or later")
	}

	member := Member{
		Name:      memberName,
		Source:    loc,
		Doc:       doc,
		DocSource: docSrc,
		Elided:    elided,
		Index:     index,
	}
	member.Traits = traitsToTraitSlice(traits)

	p.advance() // member name

	if !elided {
		p.skipSpaces()
		p.expect(TokenColon)
		p.advance()
		p.skipSpaces()
		member.Target = p.parseTarget(0)
	}

	p.skipSpaces()
	if p.tok.Kind == TokenEqual {
		if !p.version.atLeast(V2_0) {
			p.fail("@default assignment is only supported in IDL version 2 or later")
		}
		p.expect(TokenEqual)
		p.advance()
		p.skipSpaces()
		value := p.parseNode()
		member.Value = &value
		p.skipSpaces()
		p.skipOptionalComma()
		p.expectAndSkipBr()
	}

	return member, true
}

func hasDuplicateMember(members []Member, name string) bool {
	for _, m := range members {
		if m.Name == name {
			return true
		}
	}
	return false
}

func (p *parser) parseOptionalMemberIndex() *int {
	if p.tok.Kind != TokenNumber {
		return nil
	}

	if !p.version.atLeast(V2_1) {
		p.fail("Member index syntax requires Smithy IDL version 2.1 or later")
	}

	loc := p.loc()
	value := p.parseMemberIndexValue(p.tok.Lexeme, loc)
	p.advance()
	p.expect(TokenDot)
	p.advance()
	p.expectAndSkipSpaces()
	return &value
}

func (p *parser) parseMemberIndexValue(lexeme string, loc SourceLocation) int {
	if isNonPositiveInteger(lexeme) {
		p.failAt(loc, true, "Member indexes must be integers greater than zero")
	}
	if lexeme == "" || lexeme[0] < '1' || lexeme[0] > '9' {
		p.failAt(loc, true, "Member indexes must match [1-9][0-9]*")
	}

	value := 0
	for i := 0; i < len(lexeme); i++ {
		c := lexeme[i]
		if c < '0' || c > '9' {
			p.failAt(loc, true, "Member indexes must match [1-9][0-9]*")
		}
		digit := int(c - '0')
		if value > (math.MaxInt32-digit)/10 {
			p.failAt(loc, true, "Member indexes must be less than or equal to %d", math.MaxInt32)
		}
		value = value*10 + digit
	}
	return value
}

func isNonPositiveInteger(lexeme string) bool {
	if lexeme == "0" {
		return true
	}
	if len(lexeme) < 2 || lexeme[0] != '-' {
		return false
	}
	first := lexeme[1]
	if first == '0' {
		return len(lexeme) == 2
	}
	if first < '1' || first > '9' {
		return false
	}
	for i := 2; i < len(lexeme); i++ {
		if lexeme[i] < '0' || lexeme[i] > '9' {
			return false
		}
	}
	return true
}

func (p *parser) parseTarget(depth int) Target {
	switch p.tok.Kind {
	case TokenLBracket:
		if !p.version.atLeast(V2_1) {
			p.fail("Inline collection syntax requires Smithy IDL version 2.1 or later")
		}
		if depth >= maxInlineCollectionDepth {
			p.fail("Inline collections cannot be nested more than %d levels deep", maxInlineCollectionDepth)
		}
		loc := p.loc()
		p.expect(TokenLBracket)
		p.advance()
		p.skipWs()
		element := p.parseTarget(depth + 1)
		p.skipWs()
		p.expect(TokenRBracket)
		p.advance()
		return Target{InlineList: &InlineListTarget{Element: element, Source: loc}, Source: loc}
	case TokenLBrace:
		if !p.version.atLeast(V2_1) {
			p.fail("Inline collection syntax requires Smithy IDL version 2.1 or later")
		}
		if depth >= maxInlineCollectionDepth {
			p.fail("Inline collections cannot be nested more than %d levels deep", maxInlineCollectionDepth)
		}
		loc := p.loc()
		p.expect(TokenLBrace)
		p.advance()
		p.skipWs()
		key := p.parseTarget(depth + 1)
		p.skipWs()
		p.expect(TokenColon)
		p.advance()
		p.skipWs()
		value := p.parseTarget(depth + 1)
		p.skipWs()
		p.expect(TokenRBrace)
		p.advance()
		return Target{InlineMap: &InlineMapTarget{Key: key, Value: value, Source: loc}, Source: loc}
	default:
		loc := p.loc()
		id := p.parseShapeID(false)
		return Target{ID: id, Source: loc}
	}
}

func (p *parser) parseEnumShape(stmt *ShapeStmt) {
	p.parseMixins(stmt)
	p.skipWsAndDocs()
	p.expect(TokenLBrace)
	p.advance()
	p.skipWs()

	for p.tok.Kind != TokenEOF && p.tok.Kind != TokenRBrace {
		traits, doc, docSrc, _ := p.parseDocsAndTraitsBeforeShape(true)
		memberLoc := p.loc()
		p.expect(TokenIdentifier)
		name := p.tok.Lexeme
		if hasDuplicateMember(stmt.Members, name) {
			p.failShapeSyntax(stmt.ID, "Found conflicting member name, `%s`", name)
		}

		member := Member{
			Name:      name,
			Source:    memberLoc,
			Doc:       doc,
			DocSource: docSrc,
		}
		member.Traits = traitsToTraitSlice(traits)

		p.advance()
		p.skipSpaces()
		if p.tok.Kind == TokenEqual {
			p.advance()
			p.skipSpaces()
			value := p.parseNode()
			member.Value = &value
			p.skipSpaces()
			p.skipOptionalComma()
			p.expectAndSkipBr()
		} else {
			p.skipWs()
		}

		stmt.Members = append(stmt.Members, member)
	}

	p.expect(TokenRBrace)
	p.advance()
}

func (p *parser) parseServiceStatement(stmt *ShapeStmt) {
	p.parseMixins(stmt)
	p.skipWsAndDocs()
	p.expect(TokenLBrace)
	body := p.parseNode()
	requireObjectNode(p, body)
	stmt.Body = &body
}

func (p *parser) parseResourceStatement(stmt *ShapeStmt) {
	p.parseMixins(stmt)
	p.skipWsAndDocs()
	p.expect(TokenLBrace)
	body := p.parseNode()
	requireObjectNode(p, body)
	stmt.Body = &body

	if _, ok := body.Get("properties"); ok && !p.version.atLeast(V2_0) {
		p.fail("Resource properties can only be used with Smithy version 2 or later. "+
			"Attempted to use resource properties with version `%s`.", p.version)
	}
}

func requireObjectNode(p *parser, n Node) {
	if n.Kind != NodeObject {
		p.failAt(n.Source, false, "Expected object node but found %s", n.Kind)
	}
}

func (p *parser) parseOperationStatement(stmt *ShapeStmt) {
	p.parseMixins(stmt)
	p.skipWsAndDocs()
	p.expect(TokenLBrace)
	p.advance()
	p.skipWsAndDocs()

	defined := make(map[string]bool)
	for p.tok.Kind != TokenEOF && p.tok.Kind != TokenRBrace {
		p.expect(TokenIdentifier)
		key := p.tok.Lexeme
		keyLoc := p.loc()
		if defined[key] {
			p.failAt(keyLoc, true, "Duplicate operation %s property for `%s`", key, stmt.ID)
		}
		defined[key] = true

		p.advance()
		p.skipWsAndDocs()

		switch key {
		case "input":
			token := p.expect(TokenColon, TokenWalrus)
			p.advance()
			stmt.Input = p.parseInlineableOperationMember(stmt, token.Kind, p.operationInputSuffix)
		case "output":
			token := p.expect(TokenColon, TokenWalrus)
			p.advance()
			stmt.Output = p.parseInlineableOperationMember(stmt, token.Kind, p.operationOutputSuffix)
		case "errors":
			p.expect(TokenColon)
			p.advance()
			stmt.Errors = p.parseIdList()
		default:
			p.fail("Unknown property %s for %s", key, stmt.ID)
		}

		p.expectAndSkipWhitespace()
		p.skipWsAndDocs()
	}

	p.expect(TokenRBrace)
	p.advance()
}

func (p *parser) parseInlineableOperationMember(stmt *ShapeStmt, token TokenKind, suffix string) *InlineIO {
	if token == TokenWalrus {
		if !p.version.atLeast(V2_0) {
			p.fail("Inlined operation inputs and outputs can only be used with Smithy version 2 or later. "+
				"Attempted to use inlined IO with version `%s`.", p.version)
		}
		p.clearPendingDocs()
		p.skipWs()
		return p.parseInlineStructure(stmt.Name + suffix)
	}

	p.skipWsAndDocs()
	loc := p.loc()
	id := p.parseShapeID(false)
	return &InlineIO{Target: &ShapeRef{ID: id, Source: loc}, Source: loc}
}

func (p *parser) parseInlineStructure(name string) *InlineIO {
	traits, doc, docSrc, _ := p.parseDocsAndTraitsBeforeShape(true)
	loc := p.loc()

	stmt := &ShapeStmt{
		Kind:      KindStructure,
		Name:      name,
		ID:        p.shapeID(name),
		Source:    loc,
		Namespace: p.namespace,
	}
	p.parseAggregateShape(stmt)

	return &InlineIO{
		Traits:      traitsToTraitSlice(traits),
		Mixins:      stmt.Mixins,
		Members:     stmt.Members,
		ForResource: stmt.ForResource,
		Source:      loc,
		Doc:         doc,
		DocSource:   docSrc,
	}
}

func (p *parser) parseIdList() []ShapeRef {
	p.increaseNesting()
	p.skipWsAndDocs()
	p.expect(TokenLBracket)
	p.advance()
	p.skipWsAndDocs()

	var refs []ShapeRef
	for p.tok.Kind != TokenEOF && p.tok.Kind != TokenRBracket {
		p.expect(TokenIdentifier)
		loc := p.loc()
		id := p.parseShapeID(false)
		refs = append(refs, ShapeRef{ID: id, Source: loc})
		p.skipWsAndDocs()
	}

	p.expect(TokenRBracket)
	p.advance()
	p.decreaseNesting()
	return refs
}

func (p *parser) parseApplyStatement(traits []traitResult) {
	if len(traits) > 0 {
		p.fail("Traits applied to apply statement")
	}

	loc := p.loc()
	p.advance() // apply
	p.expectAndSkipWhitespace()

	targetLoc := p.loc()
	target := p.parseShapeID(false)
	p.expectAndSkipWhitespace()

	var applied []traitResult
	if p.tok.Kind == TokenAt {
		applied = append(applied, p.parseTrait())
	} else {
		p.expect(TokenAt, TokenLBrace)
		p.advance()
		p.skipWsAndDocs()
		applied = p.parseTraits()
		p.skipWsAndDocs()
		p.expect(TokenRBrace)
		p.advance()
	}

	stmt := &ApplyStmt{
		Target: ShapeRef{ID: target, Source: targetLoc},
		Source: loc,
	}
	for _, tr := range applied {
		stmt.Traits = append(stmt.Traits, traitResultToTrait(tr))
	}

	p.expectAndSkipBr()
	p.file.Shapes = append(p.file.Shapes, stmt)
}

// --- traits ----------------------------------------------------------------

func (p *parser) parseDocsAndTraitsBeforeShape(isMember bool) ([]traitResult, string, SourceLocation, bool) {
	p.skipWs()

	var doc string
	var docSrc SourceLocation
	var hasDoc bool

	if p.tok.Kind == TokenDocComment {
		hasDoc = true
		p.skipWsAndDocs()
		doc, docSrc, _ = p.removePendingDocs(p.loc())
	} else {
		p.skipWsAndDocs()
	}

	p.skipWsAndDocs()
	traits := p.parseTraits()
	p.skipWsAndDocs()

	if isMember {
		p.clearPendingDocs()
	}

	return traits, doc, docSrc, hasDoc
}

func (p *parser) parseTraits() []traitResult {
	var traits []traitResult
	for p.tok.Kind == TokenAt {
		traits = append(traits, p.parseTrait())
		p.skipWsAndDocs()
	}
	return traits
}

func (p *parser) parseTrait() traitResult {
	loc := p.loc()
	p.expect(TokenAt)
	p.advance()
	name := p.parseShapeID(false)

	if p.tok.Kind != TokenLParen {
		return traitResult{
			name:       name,
			value:      Node{Kind: NodeNull, Source: loc},
			source:     loc,
			annotation: true,
		}
	}

	p.advance()
	p.skipWsAndDocs()

	if p.tok.Kind == TokenRParen {
		p.advance()
		return traitResult{
			name:       name,
			value:      Node{Kind: NodeNull, Source: loc},
			source:     loc,
			annotation: true,
		}
	}

	value := p.parseTraitValueBody(loc)
	p.skipWsAndDocs()
	p.expect(TokenRParen)
	p.advance()

	return traitResult{name: name, value: value, source: loc}
}

func (p *parser) parseTraitValueBody(loc SourceLocation) Node {
	p.expect(TokenLBrace, TokenLBracket, TokenTextBlock, TokenString, TokenNumber, TokenIdentifier, TokenTag)

	switch p.tok.Kind {
	case TokenLBrace, TokenLBracket:
		n := p.parseNode()
		p.skipWsAndDocs()
		return n
	case TokenTextBlock:
		n := Node{Kind: NodeString, Str: p.tok.StringValue, Source: p.tok.Source}
		p.advance()
		p.skipWsAndDocs()
		return n
	case TokenNumber:
		n := p.parseNode()
		p.skipWsAndDocs()
		return n
	case TokenTag:
		n := p.parseNode()
		p.skipWsAndDocs()
		return n
	case TokenString:
		value := p.tok.StringValue
		first := Node{Kind: NodeString, Str: value, Source: loc}
		p.advance()
		p.skipWsAndDocs()
		if p.tok.Kind == TokenColon {
			p.advance()
			p.skipWsAndDocs()
			return p.parseStructuredTrait(first)
		}
		return first
	default: // Identifier
		identifierLoc := p.loc()
		identifier := p.parseShapeID(false)
		p.skipWsAndDocs()
		if p.tok.Kind == TokenRParen || isDefinitelyShapeID(identifier) {
			return p.createIdentifierNode(identifier, identifierLoc)
		}
		p.expect(TokenColon)
		p.advance()
		p.skipWsAndDocs()
		return p.parseStructuredTrait(Node{Kind: NodeString, Str: identifier, Source: identifierLoc})
	}
}

func (p *parser) parseStructuredTrait(firstKey Node) Node {
	p.increaseNesting()
	defer p.decreaseNesting()

	firstValue := p.parseNode()
	entries := []ObjectEntry{{
		Key:       firstKey.Str,
		KeySource: firstKey.Source,
		Value:     firstValue,
	}}
	p.skipWsAndDocs()

	for p.tok.Kind != TokenRParen {
		p.expect(TokenIdentifier, TokenString)
		key := p.tokenText()
		keySource := p.loc()
		p.advance()
		p.skipWsAndDocs()
		p.expect(TokenColon)
		p.advance()
		p.skipWsAndDocs()
		value := p.parseNode()
		for _, e := range entries {
			if e.Key == key {
				p.failAt(keySource, false, "Duplicate member of trait: '%s'", key)
			}
		}
		entries = append(entries, ObjectEntry{Key: key, KeySource: keySource, Value: value})
		p.skipWsAndDocs()
	}

	return Node{Kind: NodeObject, Obj: entries, Source: firstKey.Source}
}

func isDefinitelyShapeID(id string) bool {
	return strings.ContainsAny(id, ".$#")
}

func (p *parser) createIdentifierNode(identifier string, loc SourceLocation) Node {
	switch identifier {
	case "true":
		return Node{Kind: NodeBoolean, Bool: true, Source: loc}
	case "false":
		return Node{Kind: NodeBoolean, Bool: false, Source: loc}
	case "null":
		return Node{Kind: NodeNull, Source: loc}
	default:
		return Node{Kind: NodeShapeID, Str: identifier, Source: loc, SyntacticShapeID: true}
	}
}

func traitResultToTrait(tr traitResult) Trait {
	return Trait{
		Name:       tr.name,
		Value:      tr.value,
		Source:     tr.source,
		Annotation: tr.annotation,
	}
}

func traitsToTraitSlice(results []traitResult) []Trait {
	if len(results) == 0 {
		return nil
	}
	traits := make([]Trait, 0, len(results))
	for _, tr := range results {
		traits = append(traits, traitResultToTrait(tr))
	}
	return traits
}

// --- node values -----------------------------------------------------------

func (p *parser) parseNode() Node {
	loc := p.loc()

	switch p.tok.Kind {
	case TokenString, TokenTextBlock:
		n := Node{Kind: NodeString, Str: p.tok.StringValue, Source: loc}
		p.advance()
		return n
	case TokenNumber:
		lexeme := p.tok.Lexeme
		p.advance()
		if p.tok.Kind == TokenDot {
			p.failAt(loc, true, "Invalid number '%s.': '.' must be followed by a digit", lexeme)
		}
		return Node{Kind: NodeNumber, Num: lexeme, Source: loc}
	case TokenIdentifier:
		identifier := p.parseShapeID(false)
		return p.createIdentifierNode(identifier, loc)
	case TokenLBrace:
		return p.parseObjectNode()
	case TokenLBracket:
		return p.parseArrayNode()
	case TokenTag:
		return p.parseTaggedLiteral()
	default:
		p.expect(TokenString, TokenTextBlock, TokenNumber, TokenIdentifier,
			TokenLBrace, TokenLBracket, TokenTag)
		return Node{}
	}
}

func (p *parser) parseArrayNode() Node {
	loc := p.loc()
	p.increaseNesting()
	defer p.decreaseNesting()

	p.expect(TokenLBracket)
	p.advance()
	p.skipWsAndDocs()

	var values []Node
	for p.tok.Kind != TokenRBracket {
		values = append(values, p.parseNode())
		p.skipWsAndDocs()
	}

	p.expect(TokenRBracket)
	p.advance()
	return Node{Kind: NodeArray, Arr: values, Source: loc}
}

func (p *parser) parseObjectNode() Node {
	loc := p.loc()
	p.increaseNesting()
	defer p.decreaseNesting()

	p.expect(TokenLBrace)
	p.advance()
	p.skipWsAndDocs()

	var entries []ObjectEntry
	for p.tok.Kind != TokenEOF {
		tok := p.expect(TokenRBrace, TokenString, TokenIdentifier)
		if tok.Kind == TokenRBrace {
			break
		}

		key := p.tokenText()
		keySource := p.loc()
		p.advance()
		p.skipWsAndDocs()
		p.expect(TokenColon)
		p.advance()
		p.skipWsAndDocs()

		value := p.parseNode()
		for _, e := range entries {
			if e.Key == key {
				p.failAt(keySource, false, "Duplicate member: '%s'", key)
			}
		}
		entries = append(entries, ObjectEntry{Key: key, KeySource: keySource, Value: value})
		p.skipWsAndDocs()
	}

	p.expect(TokenRBrace)
	p.advance()
	return Node{Kind: NodeObject, Obj: entries, Source: loc}
}

func (p *parser) parseTaggedLiteral() Node {
	loc := p.loc()
	if !p.version.atLeast(V2_1) {
		p.fail("Tagged string literals require Smithy IDL version 2.1 or later")
	}

	tag := p.tok.Tag
	p.advance()
	p.expect(TokenRawString, TokenRawTextBlock)
	isTextBlock := p.tok.Kind == TokenRawTextBlock
	raw := p.tok.StringValue
	p.advance()

	value, err := scanTaggedLiteral(tag, raw, isTextBlock)
	if err != nil {
		p.failAt(loc, false, "%s", err.Error())
	}
	value.Source = loc
	value.Tag = tag
	return value
}

// --- shape IDs -------------------------------------------------------------

// parseShapeID parses a relative or absolute shape ID, including an optional
// member suffix.
func (p *parser) parseShapeID(absolute bool) string {
	start := p.loc().Start
	p.parseNamespace()
	end := p.lastEnd

	if absolute {
		p.expect(TokenPound)
		p.advance()
		end = p.parseRelativeRootShapeID()
		return p.src[start:end]
	}

	switch p.tok.Kind {
	case TokenPound, TokenDollar:
		p.advance()
		end = p.parseRelativeRootShapeID()
	}

	return p.src[start:end]
}

func (p *parser) parseAbsoluteShapeID() string {
	return p.parseShapeID(true)
}

func (p *parser) parseNamespace() string {
	start := p.loc().Start
	p.expect(TokenIdentifier)
	p.lastEnd = p.tok.Source.End
	p.advance()

	for p.tok.Kind == TokenDot {
		p.advance()
		p.expect(TokenIdentifier)
		p.lastEnd = p.tok.Source.End
		p.advance()
	}

	return p.src[start:p.lastEnd]
}

func (p *parser) parseRelativeRootShapeID() int {
	p.expect(TokenIdentifier)
	end := p.tok.Source.End
	p.advance()

	if p.tok.Kind == TokenDollar {
		p.advance()
		p.expect(TokenIdentifier)
		end = p.tok.Source.End
		p.advance()
	}

	return end
}
