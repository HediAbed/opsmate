package terminal

import "testing"

func TestNormalizeTerminalUTF8HandlesInvalidBytes(t *testing.T) {
	input := string([]byte{'A', 0x80, 0xff}) + "é"
	if got := normalizeTerminalUTF8(input); got != "A�é" {
		t.Fatalf("normalizeTerminalUTF8() = %q, want replacement only for printable invalid byte", got)
	}
}
