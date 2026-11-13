package idl

import "fmt"

// TokenKind is the lexical class of a token in Smithy IDL.
type TokenKind int

const (
	TokenEOF TokenKind = iota
	TokenSpace
	TokenNewline
	TokenComma
	TokenComment
	TokenDocComment
	TokenAt
	TokenString
	TokenTextBlock
	TokenRawString
	TokenRawTextBlock
	TokenTag
	TokenColon
	TokenWalrus
	TokenIdentifier
	TokenDot
	TokenPound
	TokenDollar
	TokenNumber
	TokenLBrace
	TokenRBrace
	TokenLBracket
	TokenRBracket
	TokenLParen
	TokenRParen
	TokenEqual
	TokenError
)

// String returns the token name used in parser diagnostics.
func (k TokenKind) String() string {
	switch k {
	case TokenEOF:
		return "EOF"
	case TokenSpace:
		return "SPACE"
	case TokenNewline:
		return "NEWLINE"
	case TokenComma:
		return "COMMA"
	case TokenComment:
		return "COMMENT"
	case TokenDocComment:
		return "DOC_COMMENT"
	case TokenAt:
		return "AT"
	case TokenString:
		return "STRING"
	case TokenTextBlock:
		return "TEXT_BLOCK"
	case TokenRawString:
		return "RAW_STRING"
	case TokenRawTextBlock:
		return "RAW_TEXT_BLOCK"
	case TokenTag:
		return "TAG"
	case TokenColon:
		return "COLON"
	case TokenWalrus:
		return "WALRUS"
	case TokenIdentifier:
		return "IDENTIFIER"
	case TokenDot:
		return "DOT"
	case TokenPound:
		return "POUND"
	case TokenDollar:
		return "DOLLAR"
	case TokenNumber:
		return "NUMBER"
	case TokenLBrace:
		return "LBRACE"
	case TokenRBrace:
		return "RBRACE"
	case TokenLBracket:
		return "LBRACKET"
	case TokenRBracket:
		return "RBRACKET"
	case TokenLParen:
		return "LPAREN"
	case TokenRParen:
		return "RPAREN"
	case TokenEqual:
		return "EQUAL"
	case TokenError:
		return "ERROR"
	default:
		return fmt.Sprintf("TokenKind(%d)", int(k))
	}
}

// exampleLexeme is the canonical lexeme shown in diagnostics when a token has
// no meaningful source text.
func (k TokenKind) exampleLexeme() string {
	switch k {
	case TokenSpace:
		return " "
	case TokenNewline:
		return "\\n"
	case TokenComma:
		return ","
	case TokenComment:
		return "//"
	case TokenDocComment:
		return "///"
	case TokenAt:
		return "@"
	case TokenString, TokenRawString:
		return "\""
	case TokenTextBlock, TokenRawTextBlock:
		return "\"\"\""
	case TokenTag:
		return "#"
	case TokenColon:
		return ":"
	case TokenWalrus:
		return ":="
	case TokenDot:
		return "."
	case TokenPound:
		return "#"
	case TokenDollar:
		return "$"
	case TokenLBrace:
		return "{"
	case TokenRBrace:
		return "}"
	case TokenLBracket:
		return "["
	case TokenRBracket:
		return "]"
	case TokenLParen:
		return "("
	case TokenRParen:
		return ")"
	case TokenEqual:
		return "="
	default:
		return ""
	}
}

// debug renders a token as NAME('lexeme'), or just NAME when it has no
// meaningful lexeme. This matches the reference implementation's diagnostics.
func (k TokenKind) debug(lexeme string) string {
	if lexeme == "" {
		return k.String()
	}
	return fmt.Sprintf("%s('%s')", k, lexeme)
}

// isWhitespace reports whether the token is insignificant whitespace. Doc
// comments are deliberately excluded: they are buffered for attachment to the
// next shape or member.
func (k TokenKind) isWhitespace() bool {
	switch k {
	case TokenSpace, TokenNewline, TokenComma, TokenComment:
		return true
	default:
		return false
	}
}

func (k TokenKind) isString() bool {
	return k == TokenString || k == TokenRawString
}

func (k TokenKind) isTextBlock() bool {
	return k == TokenTextBlock || k == TokenRawTextBlock
}

// Token is one lexical token together with its source span.
type Token struct {
	Kind   TokenKind
	Lexeme string
	// StringValue holds the decoded contents of string and text block tokens,
	// or the unprocessed contents of raw string tokens.
	StringValue string
	// Tag holds the tag name (without '#') for TAG tokens.
	Tag string
	// Err holds the diagnostic for ERROR tokens.
	Err string
	// Source identifies the token's start; End is the exclusive end offset.
	Source SourceLocation
}

func (t Token) debug() string {
	if t.Kind == TokenEOF {
		return "EOF"
	}
	return t.Kind.debug(t.Lexeme)
}
