package component

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

func HighlightYAML(source string) string {
	return highlightYAMLWithNames(source, yamlLexerName, yamlStyleName, yamlFormatterName)
}

func highlightYAMLWithNames(source, lexerName, styleName, formatterName string) string {
	if source == "" {
		return source
	}
	lexer, style, formatter, found := resolveYAMLHighlighter(lexerName, styleName, formatterName)
	if !found {
		return source
	}
	return renderHighlightedYAML(source, lexer, style, formatter)
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

func renderHighlightedYAML(source string, lexer chroma.Lexer, style *chroma.Style, formatter chroma.Formatter) string {
	iterator, err := lexer.Tokenise(nil, source)
	if err != nil {
		return source
	}
	var output bytes.Buffer
	if err := formatter.Format(&output, style, iterator); err != nil {
		return source
	}
	return output.String()
}
