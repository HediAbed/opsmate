package model

import (
	"strings"
	"testing"
)

func TestParsePortSpec_Valid(t *testing.T) {
	local, remote, err := parsePortSpec("8080:80")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if local != 8080 || remote != 80 {
		t.Errorf("got (%d, %d); want (8080, 80)", local, remote)
	}
}

func TestParsePortSpec_MissingColon(t *testing.T) {
	_, _, err := parsePortSpec("8080")
	if err == nil {
		t.Fatal("expected error for missing colon")
	}
	if !strings.Contains(err.Error(), "local") && !strings.Contains(err.Error(), "expected") {
		t.Errorf("unhelpful error: %v", err)
	}
}

func TestParsePortSpec_NonNumeric(t *testing.T) {
	cases := []string{"abc:80", "80:xyz", "80::80"}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			if _, _, err := parsePortSpec(in); err == nil {
				t.Errorf("parsePortSpec(%q) should fail", in)
			}
		})
	}
}

func TestParsePortSpec_NegativeOrZero(t *testing.T) {
	cases := []string{"0:80", "80:0", "-1:80", "80:-1"}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			if _, _, err := parsePortSpec(in); err == nil {
				t.Errorf("parsePortSpec(%q) should reject non-positive port", in)
			}
		})
	}
}
