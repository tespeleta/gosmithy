package idl

import (
	"strings"
	"testing"
)

func collectTokens(t *testing.T, src string) []Token {
	t.Helper()
	tok := newTokenizer("test.smithy", src)
	var tokens []Token
	for {
		tk := tok.next()
		tokens = append(tokens, tk)
		if tk.Kind == TokenEOF {
			break
		}
	}
	return tokens
}

func TestTokenizerTokensAndPositions(t *testing.T) {
	tokens := collectTokens(t, "$version: \"2.0\"\nnamespace a.b\n")
	want := []TokenKind{
		TokenDollar, TokenIdentifier, TokenColon, TokenSpace, TokenString,
		TokenNewline, TokenIdentifier, TokenSpace, TokenIdentifier, TokenDot,
		TokenIdentifier, TokenNewline, TokenEOF,
	}
	if len(tokens) != len(want) {
		t.Fatalf("got %d tokens, want %d: %+v", len(tokens), len(want), tokens)
	}
	for i, kind := range want {
		if tokens[i].Kind != kind {
			t.Fatalf("token %d = %s, want %s", i, tokens[i].Kind, kind)
		}
	}
	if tokens[0].Source.Line != 1 || tokens[0].Source.Column != 1 {
		t.Fatalf("$ location = %+v", tokens[0].Source)
	}
	if tokens[5].Source.Line != 1 || tokens[5].Source.Column != 16 {
		t.Fatalf("newline location = %+v", tokens[5].Source)
	}
	if tokens[6].Source.Line != 2 || tokens[6].Source.Column != 1 {
		t.Fatalf("namespace location = %+v", tokens[6].Source)
	}
}

func TestTokenizerComments(t *testing.T) {
	src := "// line\n/// doc\n//// also doc\n"
	tokens := collectTokens(t, src)
	if tokens[0].Kind != TokenComment {
		t.Fatalf("token 0 = %s", tokens[0].Kind)
	}
	if tokens[0].Lexeme != "// line\n" {
		t.Fatalf("line comment = %q", tokens[0].Lexeme)
	}
	if tokens[1].Kind != TokenDocComment {
		t.Fatalf("token 1 = %s", tokens[1].Kind)
	}
	if got := docCommentText(src, tokens[1]); got != "doc" {
		t.Fatalf("doc = %q", got)
	}
	if tokens[2].Kind != TokenDocComment {
		t.Fatalf("token 2 = %s", tokens[2].Kind)
	}
	if got := docCommentText(src, tokens[2]); got != "/ also doc" {
		t.Fatalf("doc = %q", got)
	}

	errTok := collectTokens(t, "/ not a comment")[0]
	if errTok.Kind != TokenError || !strings.Contains(errTok.Err, "Expected a '/'") {
		t.Fatalf("error token = %+v", errTok)
	}
}

func TestTokenizerTagRecognition(t *testing.T) {
	tokens := collectTokens(t, `#re "^\d+$" #timestamp "2024-01-01T00:00:00Z"`)
	if tokens[0].Kind != TokenTag || tokens[0].Tag != "re" {
		t.Fatalf("token 0 = %+v", tokens[0])
	}
	if tokens[1].Kind != TokenRawString || tokens[1].StringValue != `^\d+$` {
		t.Fatalf("raw string = %+v", tokens[1])
	}
	if tokens[3].Kind != TokenTag || tokens[3].Tag != "timestamp" {
		t.Fatalf("token 3 = %+v", tokens[3])
	}

	// A known tag not followed by a string is a plain '#' token.
	tokens = collectTokens(t, "#re")
	if tokens[0].Kind != TokenPound || tokens[1].Kind != TokenIdentifier {
		t.Fatalf("tokens = %+v", tokens)
	}

	// An unknown tag is also a plain '#' token.
	tokens = collectTokens(t, "#unknown")
	if tokens[0].Kind != TokenPound || tokens[1].Kind != TokenIdentifier {
		t.Fatalf("tokens = %+v", tokens)
	}
}

func TestTokenizerNumbers(t *testing.T) {
	cases := []struct {
		src  string
		kind TokenKind
		err  string
	}{
		{"0", TokenNumber, ""},
		{"-12", TokenNumber, ""},
		{"1.5", TokenNumber, ""},
		{"1e10", TokenNumber, ""},
		{"1.5e-3", TokenNumber, ""},
		{"1.", TokenNumber, ""}, // trailing dot is left to the node parser
		{"1..", TokenNumber, ""},
		{"1.0e", TokenError, "'e', '+', and '-' must be followed by a digit"},
		{"1.0e+", TokenError, "'e', '+', and '-' must be followed by a digit"},
		{"-", TokenError, "'-' must be followed by a digit"},
		{"1.e", TokenNumber, ""},
	}

	for _, tc := range cases {
		t.Run(tc.src, func(t *testing.T) {
			tok := collectTokens(t, tc.src)[0]
			if tok.Kind != tc.kind {
				t.Fatalf("%q: kind = %s, want %s (err=%q)", tc.src, tok.Kind, tc.kind, tok.Err)
			}
			if tc.err != "" && !strings.Contains(tok.Err, tc.err) {
				t.Fatalf("%q: err = %q, want %q", tc.src, tok.Err, tc.err)
			}
		})
	}
}

func TestScanStringContents(t *testing.T) {
	cases := []struct {
		name      string
		raw       string
		textBlock bool
		want      string
		wantErr   string
	}{
		{name: "plain", raw: "hello", want: "hello"},
		{name: "escapes", raw: `a\nb\tc\\d\"e\/f`, want: "a\nb\tc\\d\"e/f"},
		{name: "unicode", raw: `\u00e9`, want: "é"},
		{name: "escaped newline", raw: "a\\\nb", want: "ab"},
		{name: "invalid escape", raw: `\q`, wantErr: "Invalid escape found in string: `\\q`"},
		{name: "unclosed unicode", raw: `\u12`, wantErr: "Invalid unclosed unicode escape found in string"},
		{name: "bad unicode", raw: `\u12g4`, wantErr: "Invalid unicode escape character: `g`"},
		{
			name:      "text block indentation",
			raw:       "\n    Hello\n    World\n    ",
			textBlock: true,
			want:      "Hello\nWorld\n",
		},
		{
			name:      "text block strips trailing spaces",
			raw:       "\n    Hello   \n    World  \n    ",
			textBlock: true,
			want:      "Hello\nWorld\n",
		},
		{
			name:      "text block empty",
			raw:       "",
			textBlock: true,
			wantErr:   "Text block is empty",
		},
		{
			name:      "text block missing newline",
			raw:       "Hello",
			textBlock: true,
			wantErr:   "Text block must start with a new line",
		},
		{
			name:      "text block embedded quotes",
			raw:       "\n    \"Hi\\\"",
			textBlock: true,
			want:      `"Hi"`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := scanStringContents(tc.raw, tc.textBlock)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNormalizeLineEndings(t *testing.T) {
	if got := normalizeLineEndings("a\r\nb\rc\nd"); got != "a\nb\nc\nd" {
		t.Fatalf("got %q", got)
	}
}
