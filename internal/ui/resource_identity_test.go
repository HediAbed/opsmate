package ui

import "testing"

func TestNamespacesMatch_RequiresExactIdentity(t *testing.T) {
	tests := []struct {
		name   string
		first  string
		second string
		want   bool
	}{
		{name: "same namespace", first: "payments", second: "payments", want: true},
		{name: "different namespaces", first: "payments", second: "platform"},
		{name: "missing selected namespace", second: "platform"},
		{name: "missing resource namespace", first: "payments"},
		{name: "both cluster scoped", want: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := namespacesMatch(test.first, test.second); got != test.want {
				t.Fatalf("namespacesMatch(%q, %q) = %v, want %v", test.first, test.second, got, test.want)
			}
		})
	}
}
