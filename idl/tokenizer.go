package idl

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// tokenizer turns Smithy IDL source into tokens. It mirrors the reference
// implementation's token model, including its significant-whitespace rules.
type tokenizer struct {
	filename string
	src      string
	pos      int
	line     int
	column   int

	nextIsRawString bool
	identifierError string
	lastError       *ParseError
}

func newTokenizer(filename, src string) *tokenizer {
	return &tokenizer{filename: filename, src: src, line: 1, column: 1}
}

func (t *tokenizer) eof() bool {
	return t.pos >= len(t.src)
}

// peek returns the rune at the current position, or 0 at EOF.
func (t *tokenizer) peek() rune {
	if t.eof() {
		return 0
	}
	r, _ := utf8.DecodeRuneInString(t.src[t.pos:])
	return r
}

// peekByte returns the byte at pos+offset, or 0 when out of range. It is only
// used to look for ASCII delimiters such as quotes.
func (t *tokenizer) peekByte(offset int) byte {
	i := t.pos + offset
	if i < 0 || i >= len(t.src) {
		return 0
	}
	return t.src[i]
}

// advance consumes one rune, treating CRLF as a single line break.
func (t *tokenizer) advance() {
	if t.eof() {
		return
	}
	r, size := utf8.DecodeRuneInString(t.src[t.pos:])
	if r == '\r' {
		if t.peekByte(size) == '\n' {
			size++
		}
		t.line++
		t.column = 1
	} else if r == '\n' {
		t.line++
		t.column = 1
	} else {
		t.column++
	}
	t.pos += size
}

// skipRune consumes the current rune unconditionally.
func (t *tokenizer) skipRune() { t.advance() }

func (t *tokenizer) consumeIf(r rune) bool {
	if t.peek() == r {
		t.advance()
		return true
	}
	return false
}

func (t *tokenizer) consumeWhile(pred func(rune) bool) int {
	n := 0
	for !t.eof() && pred(t.peek()) {
		t.advance()
		n++
	}
	return n
}

func (t *tokenizer) sourceAt(start int, line, column int) SourceLocation {
	return SourceLocation{
		Filename: t.filename,
		Line:     line,
		Column:   column,
		Start:    start,
		End:      t.pos,
	}
}

// next returns the next token.
func (t *tokenizer) next() Token {
	if t.eof() {
		return Token{
			Kind:   TokenEOF,
			Source: t.sourceAt(t.pos, t.line, t.column),
		}
	}

	if t.nextIsRawString {
		return t.parseRawString()
	}

	start, line, column := t.pos, t.line, t.column
	r := t.peek()

	switch r {
	case ' ', '\t':
		t.consumeWhile(func(r rune) bool { return r == ' ' || r == '\t' })
		return t.token(TokenSpace, start, line, column)
	case '\r', '\n':
		t.advance()
		return t.token(TokenNewline, start, line, column)
	case ',':
		return t.single(TokenComma, start, line, column)
	case '@':
		return t.single(TokenAt, start, line, column)
	case '$':
		return t.single(TokenDollar, start, line, column)
	case '.':
		return t.single(TokenDot, start, line, column)
	case '{':
		return t.single(TokenLBrace, start, line, column)
	case '}':
		return t.single(TokenRBrace, start, line, column)
	case '[':
		return t.single(TokenLBracket, start, line, column)
	case ']':
		return t.single(TokenRBracket, start, line, column)
	case '(':
		return t.single(TokenLParen, start, line, column)
	case ')':
		return t.single(TokenRParen, start, line, column)
	case '#':
		return t.parsePoundOrTag(start, line, column)
	case '=':
		return t.single(TokenEqual, start, line, column)
	case ':':
		return t.parseColon(start, line, column)
	case '"':
		return t.parseString(start, line, column)
	case '/':
		return t.parseComment(start, line, column)
	case '-', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		return t.parseNumber(start, line, column)
	}

	if isAlphabetic(r) || r == '_' {
		return t.parseIdentifier(start, line, column)
	}

	t.advance()
	return Token{
		Kind:   TokenError,
		Lexeme: t.src[start:t.pos],
		Err:    fmt.Sprintf("Unexpected character: '%c'", r),
		Source: t.sourceAt(start, line, column),
	}
}

func (t *tokenizer) token(kind TokenKind, start, line, column int) Token {
	return Token{
		Kind:   kind,
		Lexeme: t.src[start:t.pos],
		Source: t.sourceAt(start, line, column),
	}
}

func (t *tokenizer) single(kind TokenKind, start, line, column int) Token {
	t.advance()
	return t.token(kind, start, line, column)
}

func (t *tokenizer) parseColon(start, line, column int) Token {
	t.advance()
	kind := TokenColon
	if t.consumeIf('=') {
		kind = TokenWalrus
	}
	return t.token(kind, start, line, column)
}

func (t *tokenizer) parseComment(start, line, column int) Token {
	// first '/'
	t.advance()
	if !t.consumeIf('/') {
		return Token{
			Kind:   TokenError,
			Lexeme: t.src[start:t.pos],
			Err:    "Expected a '/' to follow '/' to form a comment.",
			Source: t.sourceAt(start, line, column),
		}
	}

	kind := TokenComment
	if t.consumeIf('/') {
		kind = TokenDocComment
	}

	t.consumeWhile(func(r rune) bool { return r != '\n' && r != '\r' })
	// Include the line break in the comment lexeme.
	if t.consumeIf('\r') {
		t.consumeIf('\n')
	} else {
		t.consumeIf('\n')
	}

	return t.token(kind, start, line, column)
}

func (t *tokenizer) parseNumber(start, line, column int) Token {
	numStart := t.pos
	if t.peek() == '-' {
		t.advance()
		if !isDigit(t.peek()) {
			return t.numberError(start, line, column, numStart,
				"'-' must be followed by a digit")
		}
	}

	t.consumeWhile(isDigit)

	// A trailing "." is left for the caller when it is not followed by a
	// digit. This lets the node parser report the more specific "Invalid
	// number '1.'" error.
	if t.peek() == '.' && isDigit(rune(t.peekByte(1))) {
		t.advance()
		if t.consumeWhile(isDigit) == 0 {
			return t.numberError(start, line, column, numStart,
				"'.' must be followed by a digit")
		}
	}

	if r := t.peek(); r == 'e' || r == 'E' {
		t.advance()
		if r := t.peek(); r == '+' || r == '-' {
			t.advance()
		}
		if t.consumeWhile(isDigit) == 0 {
			return t.numberError(start, line, column, numStart,
				"'e', '+', and '-' must be followed by a digit")
		}
	}

	return t.token(TokenNumber, start, line, column)
}

func (t *tokenizer) numberError(start, line, column, numStart int, reason string) Token {
	lexeme := t.src[numStart:t.pos]
	return Token{
		Kind:   TokenError,
		Lexeme: t.src[start:t.pos],
		Err:    fmt.Sprintf("Invalid number '%s': %s", lexeme, reason),
		Source: t.sourceAt(start, line, column),
	}
}

func (t *tokenizer) parseIdentifier(start, line, column int) Token {
	if !consumeIdentifier(t) {
		// consumeIdentifier leaves the tokenizer at the offending character.
		// The reference implementation reports the inner location, so include
		// it in the token diagnostic.
		inner := &ParseError{
			Location: SourceLocation{
				Filename: t.filename,
				Line:     t.line,
				Column:   t.column,
				Start:    t.pos,
				End:      t.pos,
			},
			Message:  t.identifierError,
			Prefixed: true,
		}
		return Token{
			Kind:   TokenError,
			Lexeme: t.src[start:t.pos],
			Err:    inner.Error(),
			Source: t.sourceAt(start, line, column),
		}
	}
	return t.token(TokenIdentifier, start, line, column)
}

// identifierError is set by consumeIdentifier on failure. It is kept on the
// tokenizer only to avoid threading an error return through the matcher.
// Callers must not rely on it across calls.

func consumeIdentifier(t *tokenizer) bool {
	c := t.peek()
	if c == '_' {
		for t.peek() == '_' {
			t.advance()
		}
		c := t.peek()
		if !isValidIdentifierCharacter(c) {
			t.identifierError = fmt.Sprintf("Expected a valid identifier character, but found '%s'", singleCharForMessage(c))
			return false
		}
	} else if !isAlphabetic(c) {
		t.identifierError = fmt.Sprintf("Expected a valid identifier character, but found '%s'", singleCharForMessage(c))
		return false
	}

	t.advance()
	for isValidIdentifierCharacter(t.peek()) {
		t.advance()
	}
	return true
}

func singleCharForMessage(r rune) string {
	if r == 0 {
		return "[EOF]"
	}
	return string(r)
}

func isDigit(r rune) bool {
	return r >= '0' && r <= '9'
}

func isAlphabetic(r rune) bool {
	return (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z')
}

func isIdentifierStart(r rune) bool {
	return r == '_' || isAlphabetic(r)
}

func isValidIdentifierCharacter(r rune) bool {
	return isIdentifierStart(r) || isDigit(r)
}

// knownTags lists the tagged string literal handlers defined by Smithy 2.1.
// Recognizing them avoids treating a shape ID like #re as a tag when it is not
// followed by a string.
var knownTags = map[string]bool{
	"re":        true,
	"b":         true,
	"timestamp": true,
	"hex":       true,
}

func (t *tokenizer) parsePoundOrTag(start, line, column int) Token {
	savedPos, savedLine, savedCol := t.pos, t.line, t.column
	t.advance() // '#'

	if !isIdentifierStart(t.peek()) {
		return t.token(TokenPound, start, line, column)
	}

	tagStart := t.pos
	t.consumeWhile(isValidIdentifierCharacter)
	afterTag := t.pos
	tag := t.src[tagStart:afterTag]

	if !knownTags[tag] {
		t.rewind(savedPos, savedLine, savedCol)
		return t.single(TokenPound, start, line, column)
	}

	// Optional spaces between the tag and the string.
	for t.peek() == ' ' || t.peek() == '\t' {
		t.advance()
	}

	if t.peek() != '"' {
		t.rewind(savedPos, savedLine, savedCol)
		return t.single(TokenPound, start, line, column)
	}

	t.nextIsRawString = true
	return Token{
		Kind:   TokenTag,
		Lexeme: t.src[start:afterTag],
		Tag:    tag,
		Source: SourceLocation{
			Filename: t.filename,
			Line:     line,
			Column:   column,
			Start:    start,
			End:      afterTag,
		},
	}
}

func (t *tokenizer) rewind(pos, line, column int) {
	t.pos, t.line, t.column = pos, line, column
}

func (t *tokenizer) parseRawString() Token {
	t.nextIsRawString = false
	start, line, column := t.pos, t.line, t.column
	t.advance() // first quote

	triple := false
	if t.peek() == '"' {
		t.advance()
		if t.peek() == '"' {
			t.advance()
			triple = true
		} else {
			return t.token(TokenRawString, start, line, column)
		}
	}

	contentStart := t.pos
	for !t.eof() {
		r := t.peek()
		if r == '"' && (!triple || (t.peekByte(1) == '"' && t.peekByte(2) == '"')) {
			break
		}
		t.advance()
		if r == '\\' {
			t.advance()
		}
	}
	raw := t.src[contentStart:t.pos]

	if !t.consumeClosingQuotes(triple, start, line, column) {
		err := t.lastError
		return Token{
			Kind:   TokenError,
			Lexeme: t.src[start:t.pos],
			Err:    "Error parsing quoted string: " + err.Error(),
			Source: t.sourceAt(start, line, column),
		}
	}

	kind := TokenRawString
	if triple {
		kind = TokenRawTextBlock
	}
	tok := t.token(kind, start, line, column)
	tok.StringValue = raw
	return tok
}

// consumeClosingQuotes consumes the closing quote(s), returning a synthetic
// ERROR token when the input ends first.
func (t *tokenizer) consumeClosingQuotes(triple bool, start, line, column int) bool {
	n := 1
	if triple {
		n = 3
	}
	for i := 0; i < n; i++ {
		if !t.consumeIf('"') {
			t.lastError = &ParseError{
				Location: SourceLocation{
					Filename: t.filename,
					Line:     t.line,
					Column:   t.column,
					Start:    t.pos,
					End:      t.pos,
				},
				Message:  "Expected: '\"', but found '[EOF]'",
				Prefixed: true,
			}
			return false
		}
	}
	return true
}

func (t *tokenizer) parseString(start, line, column int) Token {
	t.advance() // first quote

	if t.peek() == '"' {
		t.advance()
		if t.peek() == '"' {
			t.advance()
			return t.parseTextBlockToken(start, line, column)
		}
		tok := t.token(TokenString, start, line, column)
		return tok
	}

	contentStart := t.pos
	for !t.eof() {
		r := t.peek()
		if r == '"' {
			break
		}
		t.advance()
		if r == '\\' {
			t.advance()
		}
	}
	raw := t.src[contentStart:t.pos]
	if !t.consumeClosingQuotes(false, start, line, column) {
		err := t.lastError
		return Token{
			Kind:   TokenError,
			Lexeme: t.src[start:t.pos],
			Err:    "Error parsing quoted string: " + err.Error(),
			Source: t.sourceAt(start, line, column),
		}
	}

	value, err := scanStringContents(raw, false)
	if err != nil {
		return Token{
			Kind:   TokenError,
			Lexeme: t.src[start:t.pos],
			Err:    "Error parsing quoted string: " + err.Error(),
			Source: t.sourceAt(start, line, column),
		}
	}
	tok := t.token(TokenString, start, line, column)
	tok.StringValue = value
	return tok
}

func (t *tokenizer) parseTextBlockToken(start, line, column int) Token {
	contentStart := t.pos
	for !t.eof() {
		if t.peek() == '"' && t.peekByte(1) == '"' && t.peekByte(2) == '"' {
			break
		}
		r := t.peek()
		t.advance()
		if r == '\\' {
			t.advance()
		}
	}
	raw := t.src[contentStart:t.pos]
	if !t.consumeClosingQuotes(true, start, line, column) {
		err := t.lastError
		return Token{
			Kind:   TokenError,
			Lexeme: t.src[start:t.pos],
			Err:    "Error parsing text block: " + err.Error(),
			Source: t.sourceAt(start, line, column),
		}
	}

	value, err := scanStringContents(raw, true)
	if err != nil {
		return Token{
			Kind:   TokenError,
			Lexeme: t.src[start:t.pos],
			Err:    "Error parsing text block: " + err.Error(),
			Source: t.sourceAt(start, line, column),
		}
	}
	tok := t.token(TokenTextBlock, start, line, column)
	tok.StringValue = value
	return tok
}

// stringScanError is a diagnostic produced while decoding string contents.
type stringScanError string

func (e stringScanError) Error() string { return string(e) }

// scanStringContents decodes escapes and, for text blocks, removes incidental
// indentation. It mirrors Smithy's IdlStringLexer.
func scanStringContents(raw string, textBlock bool) (string, error) {
	raw = normalizeLineEndings(raw)
	if textBlock {
		formatted, err := formatTextBlock(raw)
		if err != nil {
			return "", err
		}
		raw = formatted
	}

	var b strings.Builder
	b.Grow(len(raw))

	const (
		stateNormal = iota
		stateAfterEscape
		stateUnicode
	)
	state := stateNormal
	hexCount := 0
	var unicode rune

	runes := []rune(raw)
	for _, c := range runes {
		switch state {
		case stateNormal:
			if c == '\\' {
				state = stateAfterEscape
			} else if isValidNormalCharacter(c, textBlock) {
				b.WriteRune(c)
			} else {
				return "", stringScanError(fmt.Sprintf("Invalid string character: `%c`", c))
			}
		case stateAfterEscape:
			state = stateNormal
			switch c {
			case '"':
				b.WriteRune('"')
			case '\\':
				b.WriteRune('\\')
			case '/':
				b.WriteRune('/')
			case 'b':
				b.WriteRune('\b')
			case 'f':
				b.WriteRune('\f')
			case 'n':
				b.WriteRune('\n')
			case 'r':
				b.WriteRune('\r')
			case 't':
				b.WriteRune('\t')
			case 'u':
				state = stateUnicode
				hexCount = 0
				unicode = 0
			case '\n':
				// Escaped newline: do not write either character.
			default:
				return "", stringScanError(fmt.Sprintf("Invalid escape found in string: `\\%c`", c))
			}
		case stateUnicode:
			digit := hexValue(c)
			if digit < 0 {
				return "", stringScanError(fmt.Sprintf("Invalid unicode escape character: `%c`", c))
			}
			unicode = unicode<<4 | rune(digit)
			hexCount++
			if hexCount == 4 {
				b.WriteRune(unicode)
				state = stateNormal
				hexCount = 0
				unicode = 0
			}
		}
	}

	if state == stateUnicode {
		return "", stringScanError("Invalid unclosed unicode escape found in string")
	}

	return b.String(), nil
}

func hexValue(c rune) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return 10 + int(c-'a')
	case c >= 'A' && c <= 'F':
		return 10 + int(c-'A')
	default:
		return -1
	}
}

func isValidNormalCharacter(c rune, textBlock bool) bool {
	return c == '\t' ||
		c == '\n' ||
		c == '\r' ||
		(c >= 0x20 && c <= 0x21) ||
		(textBlock && c == '"') ||
		(c >= 0x23 && c <= 0x5b) ||
		c >= 0x5d
}

func normalizeLineEndings(s string) string {
	if !strings.ContainsRune(s, '\r') {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '\r' {
			b.WriteByte(s[i])
			continue
		}
		if i+1 < len(s) && s[i+1] == '\n' {
			i++
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func formatTextBlock(raw string) (string, error) {
	if len(raw) == 0 {
		return "", stringScanError("Text block is empty")
	}
	if raw[0] != '\n' {
		return "", stringScanError("Text block must start with a new line")
	}

	lines := splitLines(raw)
	longestPadding := int(^uint(0) >> 1) // max int
	for i := 1; i < len(lines); i++ {
		padding := computeLeadingWhitespace(lines[i], i == len(lines)-1)
		if padding > -1 && padding < longestPadding {
			longestPadding = padding
		}
	}

	var b strings.Builder
	b.Grow(len(raw))
	for i := 1; i < len(lines); i++ {
		line := lines[i]
		if len(line) > 0 {
			if formatted, ok := createTextBlockLine(line, longestPadding); ok {
				b.WriteString(formatted)
			}
		}
		if i < len(lines)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String(), nil
}

func splitLines(s string) []string {
	lines := make([]string, 0, strings.Count(s, "\n")+1)
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	lines = append(lines, s[start:])
	return lines
}

func computeLeadingWhitespace(line string, isLastLine bool) int {
	for i := 0; i < len(line); i++ {
		if line[i] != ' ' {
			return i
		}
	}
	if isLastLine {
		return len(line)
	}
	return -1
}

func createTextBlockLine(line string, longestPadding int) (string, bool) {
	start := longestPadding
	if start > len(line) {
		start = len(line)
	}
	end := len(line) - 1
	for end > 0 && line[end] == ' ' {
		end--
	}
	if end >= start {
		return line[start : end+1], true
	}
	return "", false
}

// docCommentText extracts the documentation text of a DOC_COMMENT token.
func docCommentText(src string, tok Token) string {
	if tok.Kind != TokenDocComment {
		return ""
	}
	start := tok.Source.Start
	end := tok.Source.End
	if start < 0 || start > len(src) {
		return ""
	}
	if end > len(src) {
		end = len(src)
	}
	docStart := start + 3
	if docStart > len(src) {
		return ""
	}
	if docStart < end && src[docStart] == ' ' {
		docStart++
	}
	if end > 0 && src[end-1] == '\n' {
		end--
		if end > 0 && src[end-1] == '\r' {
			end--
		}
	}
	if docStart > end {
		return ""
	}
	return src[docStart:end]
}
