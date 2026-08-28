package service

import (
	"testing"
)

func TestStripANSI_PlainText(t *testing.T) {
	input := "hello world"
	got := stripANSI(input)
	if got != input {
		t.Errorf("stripANSI(%q) = %q; want %q", input, got, input)
	}
}

func TestStripANSI_SGR(t *testing.T) {
	input := "\x1b[31mERROR\x1b[0m: something failed"
	want := "ERROR: something failed"
	got := stripANSI(input)
	if got != want {
		t.Errorf("stripANSI SGR = %q; want %q", got, want)
	}
}

func TestStripANSI_SGRComplex(t *testing.T) {
	input := "\x1b[1;33;40mWARNING\x1b[0m text"
	want := "WARNING text"
	got := stripANSI(input)
	if got != want {
		t.Errorf("stripANSI complex SGR = %q; want %q", got, want)
	}
}

func TestStripANSI_OSCSequenceBEL(t *testing.T) {
	input := "\x1b]0;My Title\x07actual content"
	want := "actual content"
	got := stripANSI(input)
	if got != want {
		t.Errorf("stripANSI OSC+BEL = %q; want %q", got, want)
	}
}

func TestStripANSI_OSCSequenceST(t *testing.T) {
	input := "\x1b]2;Window Title\x1b\\content here"
	want := "content here"
	got := stripANSI(input)
	if got != want {
		t.Errorf("stripANSI OSC+ST = %q; want %q", got, want)
	}
}

func TestStripANSI_AlternateScreen(t *testing.T) {
	input := "\x1b[?1049hcontent\x1b[?1049l"
	want := "content"
	got := stripANSI(input)
	if got != want {
		t.Errorf("stripANSI alternate screen = %q; want %q", got, want)
	}
}

func TestStripANSI_CursorMovement(t *testing.T) {
	input := "\x1b[2Ahello\x1b[1;1Hworld"
	want := "helloworld"
	got := stripANSI(input)
	if got != want {
		t.Errorf("stripANSI cursor movement = %q; want %q", got, want)
	}
}

func TestStripANSI_ClearScreen(t *testing.T) {
	input := "\x1b[2Jhello\x1b[Kworld"
	want := "helloworld"
	got := stripANSI(input)
	if got != want {
		t.Errorf("stripANSI clear screen/line = %q; want %q", got, want)
	}
}

func TestStripANSI_CharsetDesignation(t *testing.T) {
	input := "\x1b(Bhello"
	want := "hello"
	got := stripANSI(input)
	if got != want {
		t.Errorf("stripANSI charset = %q; want %q", got, want)
	}
}

func TestStripANSI_NullBytes(t *testing.T) {
	input := "hello\x00world"
	want := "helloworld"
	got := stripANSI(input)
	if got != want {
		t.Errorf("stripANSI null bytes = %q; want %q", got, want)
	}
}

func TestStripANSI_ControlChars(t *testing.T) {
	input := "he\x07ll\x08o\x01"
	want := "hello"
	got := stripANSI(input)
	if got != want {
		t.Errorf("stripANSI control chars = %q; want %q", got, want)
	}
}

func TestStripANSI_MalformedBytesDoNotCorruptUnicode(t *testing.T) {
	input := "🙂\x9fmiddle\xff"
	want := "🙂middle�"
	if got := stripANSI(input); got != want {
		t.Errorf("stripANSI malformed bytes = %q; want %q", got, want)
	}
}

func TestStripANSI_PreservesNewlineAndTab(t *testing.T) {
	input := "line1\nline2\tindented"
	want := "line1\nline2\tindented"
	got := stripANSI(input)
	if got != want {
		t.Errorf("stripANSI preserve newline/tab = %q; want %q", got, want)
	}
}

func TestStripANSI_EmptyString(t *testing.T) {
	got := stripANSI("")
	if got != "" {
		t.Errorf("stripANSI empty = %q; want %q", got, "")
	}
}

func TestStripANSI_TrailingEsc(t *testing.T) {
	input := "text\x1b"
	want := "text"
	got := stripANSI(input)
	if got != want {
		t.Errorf("stripANSI trailing ESC = %q; want %q", got, want)
	}
}

func TestStripANSI_GenericEscSequence(t *testing.T) {
	input := "\x1b=keypad\x1b>normal"
	want := "keypadnormal"
	got := stripANSI(input)
	if got != want {
		t.Errorf("stripANSI generic ESC = %q; want %q", got, want)
	}
}

func TestStripANSI_MixedSequences(t *testing.T) {
	input := "\x1b]0;Claude\x07\x1b[1;36m## Analysis\x1b[0m\n\x00This \x07is \x1b[?25hcontent\x1b[?25l"
	want := "## Analysis\nThis is content"
	got := stripANSI(input)
	if got != want {
		t.Errorf("stripANSI mixed = %q; want %q", got, want)
	}
}

func TestParseCommandResponse_SingleLine(t *testing.T) {
	cmd, expl := parseCommandResponse("kubectl get pods -n default")
	if cmd != "kubectl get pods -n default" {
		t.Errorf("command = %q; want %q", cmd, "kubectl get pods -n default")
	}
	if expl != "" {
		t.Errorf("explanation = %q; want empty", expl)
	}
}

func TestParseCommandResponse_MultiLine(t *testing.T) {
	input := "kubectl get pods -n prod\nThis lists all pods in the prod namespace.\nIt shows status and readiness."
	cmd, expl := parseCommandResponse(input)
	wantCmd := "kubectl get pods -n prod"
	wantExpl := "This lists all pods in the prod namespace.\nIt shows status and readiness."
	if cmd != wantCmd {
		t.Errorf("command = %q; want %q", cmd, wantCmd)
	}
	if expl != wantExpl {
		t.Errorf("explanation = %q; want %q", expl, wantExpl)
	}
}

func TestParseCommandResponse_Empty(t *testing.T) {
	cmd, expl := parseCommandResponse("")
	if cmd != "" {
		t.Errorf("command = %q; want empty", cmd)
	}
	if expl != "" {
		t.Errorf("explanation = %q; want empty", expl)
	}
}

func TestParseCommandResponse_WhitespaceOnly(t *testing.T) {
	cmd, expl := parseCommandResponse("   \n   ")
	if cmd != "" {
		t.Errorf("command = %q; want empty", cmd)
	}
	if expl != "" {
		t.Errorf("explanation = %q; want empty", expl)
	}
}

func TestParseCommandResponse_LeadingTrailingWhitespace(t *testing.T) {
	input := "  kubectl delete pod foo  \n  This deletes the foo pod.  "
	cmd, expl := parseCommandResponse(input)
	if cmd != "kubectl delete pod foo" {
		t.Errorf("command = %q; want %q", cmd, "kubectl delete pod foo")
	}
	if expl != "This deletes the foo pod." {
		t.Errorf("explanation = %q; want %q", expl, "This deletes the foo pod.")
	}
}
