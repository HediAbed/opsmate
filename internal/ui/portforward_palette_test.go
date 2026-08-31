package ui

import (
	"errors"
	"testing"

	"github.com/HediAbed/opsmate/internal/failure"
)

func TestParsePortSpec_Valid(t *testing.T) {
	local, remote, err := parsePortSpec("8080:80")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if local.Int() != 8080 || remote.Int() != 80 {
		t.Errorf("got (%d, %d); want (8080, 80)", local.Int(), remote.Int())
	}
}

func TestParsePortSpec_MissingColon(t *testing.T) {
	_, _, err := parsePortSpec("8080")
	if !errors.Is(err, ErrPortMappingInvalid) {
		t.Fatalf("parsePortSpec() error = %v, want ErrPortMappingInvalid", err)
	}
	if failure.CodeOf(err) != failure.CodeInvalidArgument {
		t.Fatalf("failure.CodeOf() = %q, want %q", failure.CodeOf(err), failure.CodeInvalidArgument)
	}
}

func TestParsePortSpec_NonNumeric(t *testing.T) {
	tests := []struct {
		input string
		want  error
	}{
		{input: "abc:80", want: ErrLocalPortInvalid},
		{input: "80:xyz", want: ErrRemotePortInvalid},
		{input: "80::80", want: ErrRemotePortInvalid},
	}
	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			_, _, err := parsePortSpec(test.input)
			if !errors.Is(err, test.want) {
				t.Errorf("parsePortSpec(%q) error = %v, want %v", test.input, err, test.want)
			}
		})
	}
}

func TestParsePortSpec_NegativeOrZero(t *testing.T) {
	tests := []struct {
		input string
		want  error
	}{
		{input: "0:80", want: ErrLocalPortInvalid},
		{input: "80:0", want: ErrRemotePortInvalid},
		{input: "-1:80", want: ErrLocalPortInvalid},
		{input: "80:-1", want: ErrRemotePortInvalid},
		{input: "65536:80", want: ErrLocalPortInvalid},
		{input: "80:65536", want: ErrRemotePortInvalid},
	}
	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			_, _, err := parsePortSpec(test.input)
			if !errors.Is(err, test.want) {
				t.Errorf("parsePortSpec(%q) error = %v, want %v", test.input, err, test.want)
			}
		})
	}
}

func TestPortForwardInputErrorContract(t *testing.T) {
	tests := []struct {
		name    string
		err     PortForwardInputError
		want    string
		unwraps error
	}{
		{name: "zero value", err: PortForwardInputError{}, want: "port-forward input is invalid"},
		{
			name:    "with input and cause",
			err:     PortForwardInputError{Input: "bad", Err: ErrPortMappingInvalid},
			want:    `port-forward input "bad": expected <local>:<remote>`,
			unwraps: ErrPortMappingInvalid,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.err.Error(); got != test.want {
				t.Fatalf("Error() = %q, want %q", got, test.want)
			}
			if got := test.err.Unwrap(); !errors.Is(got, test.unwraps) {
				t.Fatalf("Unwrap() = %v, want %v", got, test.unwraps)
			}
			if code := test.err.FailureCode(); code != failure.CodeInvalidArgument {
				t.Fatalf("FailureCode() = %q, want %q", code, failure.CodeInvalidArgument)
			}
		})
	}
}
