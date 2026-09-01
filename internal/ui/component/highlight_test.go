package component

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/x/ansi"
)

func TestHighlightYAML_EmptyInput_ReturnsEmpty(t *testing.T) {
	if got := HighlightYAML(""); got != "" {
		t.Errorf("empty input produced %q; want empty", got)
	}
}

func TestHighlightYAML_AddsANSIEscapes(t *testing.T) {
	const src = "apiVersion: v1\nkind: Pod\nmetadata:\n  name: foo\n"
	got := HighlightYAML(src)
	if got == src {
		t.Fatal("output should differ from input when highlighting succeeds")
	}
	if !strings.Contains(got, "\x1b[") {
		t.Error("output should contain ANSI escape sequences")
	}
	for _, keyword := range []string{"apiVersion", "Pod", "metadata", "name", "foo"} {
		if !strings.Contains(got, keyword) {
			t.Errorf("highlighted output lost token %q", keyword)
		}
	}
}

func TestHighlightYAML_UnknownStyleFallsBack(t *testing.T) {
	got := highlightYAMLWithNames("key: value\n", yamlLexerName, "missing-style", yamlFormatterName)
	if got == "" {
		t.Error("fallback style should still produce output")
	}
}

func TestHighlightYAML_IdempotentTokenPreservation(t *testing.T) {
	const src = "name: example\nlist:\n  - a\n  - b\n"
	got := HighlightYAML(src)

	plain := ansi.Strip(got)
	if plain != src {
		t.Errorf("token content changed after highlighting\n got:  %q\n want: %q", plain, src)
	}
}

func TestResolveYAMLHighlighterHandlesMissingComponents(t *testing.T) {
	if _, _, _, ok := resolveYAMLHighlighter("missing-lexer", yamlStyleName, "terminal16m"); ok {
		t.Fatal("missing lexer unexpectedly resolved")
	}
	_, style, formatter, ok := resolveYAMLHighlighter("yaml", "missing-style", "missing-formatter")
	if !ok || style != styles.Fallback || formatter == nil {
		t.Fatal("missing style and formatter did not use fallbacks")
	}
}

func TestHighlightYAMLReturnsSourceWhenLexerIsUnavailable(t *testing.T) {
	const source = "key: value\n"
	if got := highlightYAMLWithNames(source, "missing-lexer", yamlStyleName, yamlFormatterName); got != source {
		t.Fatalf("output = %q, want source", got)
	}
}

func TestHighlightYAMLReturnsSourceAfterLexerOrFormatterFailure(t *testing.T) {
	const source = "key: value\n"
	failure := errors.New("highlight failed")
	if got := renderHighlightedYAML(source, errorLexer{err: failure}, styles.Fallback, formatters.Fallback); got != source {
		t.Fatalf("lexer failure output = %q, want source", got)
	}

	failingFormatter := chroma.FormatterFunc(func(io.Writer, *chroma.Style, chroma.Iterator) error {
		return failure
	})
	if got := renderHighlightedYAML(source, lexers.Get("yaml"), styles.Fallback, failingFormatter); got != source {
		t.Fatalf("formatter failure output = %q, want source", got)
	}
}

type errorLexer struct {
	err error
}

func (errorLexer) Config() *chroma.Config { return &chroma.Config{Name: "error"} }

func (lexer errorLexer) Tokenise(*chroma.TokeniseOptions, string) (chroma.Iterator, error) {
	return nil, lexer.err
}

func (lexer errorLexer) SetRegistry(*chroma.LexerRegistry) chroma.Lexer { return lexer }

func (lexer errorLexer) SetAnalyser(func(string) float32) chroma.Lexer { return lexer }

func (errorLexer) AnalyseText(string) float32 { return 0 }
