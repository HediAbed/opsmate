package ui

import (
	"bytes"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
)

const (
	yamlLexerName     = "yaml"
	yamlStyleName     = "dracula"
	yamlFormatterName = "terminal16m"
)

func HighlightYAML(src string) string {
	return highlightYAMLWithNames(src, yamlLexerName, yamlStyleName, yamlFormatterName)
}

func highlightYAMLWithNames(src, lexerName, styleName, formatterName string) string {
	if src == "" {
		return src
	}
	lexer, style, formatter, ok := resolveYAMLHighlighter(lexerName, styleName, formatterName)
	if !ok {
		return src
	}
	return renderHighlightedYAML(src, lexer, style, formatter)
}

func resolveYAMLHighlighter(
	lexerName string,
	styleName string,
	formatterName string,
) (chroma.Lexer, *chroma.Style, chroma.Formatter, bool) {
	lexer := lexers.Get(lexerName)
	if lexer == nil {
		return nil, nil, nil, false
	}
	style := styles.Get(styleName)
	formatter := formatters.Get(formatterName)
	return lexer, style, formatter, true
}

func renderHighlightedYAML(src string, lexer chroma.Lexer, style *chroma.Style, formatter chroma.Formatter) string {
	iter, err := lexer.Tokenise(nil, src)
	if err != nil {
		return src
	}
	var buf bytes.Buffer
	if err := formatter.Format(&buf, style, iter); err != nil {
		return src
	}
	return buf.String()
}
