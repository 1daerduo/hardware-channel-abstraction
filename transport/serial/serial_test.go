package serial

import (
	"bytes"
	"testing"
)

func TestExtractLineNormalizesUBCR(t *testing.T) {
	// U-Boot uses "\n\r" line endings: the '\r' lands at the start of the
	// next line and must be trimmed.
	p := []byte("printenv bootargs\n\rbootargs=console=ttymxc0,115200\n\r=> \n")
	var got []string
	for len(p) > 0 {
		line := extractLine(&p)
		if line == nil {
			break
		}
		got = append(got, string(line))
	}
	want := []string{"printenv bootargs", "bootargs=console=ttymxc0,115200", "=> "}
	if len(got) != len(want) {
		t.Fatalf("got %d lines, want %d: %q", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("line %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestExtractLineCRLF(t *testing.T) {
	p := []byte("a\r\nb\r\n")
	line := extractLine(&p)
	if string(line) != "a" {
		t.Fatalf("line = %q, want %q", line, "a")
	}
	line = extractLine(&p)
	if string(line) != "b" {
		t.Fatalf("line = %q, want %q", line, "b")
	}
	if extractLine(&p) != nil {
		t.Fatalf("expected exhausted buffer")
	}
}

func TestCleanResponseStripsEchoAndPrompt(t *testing.T) {
	raw := "version\n\nU-Boot 2016.03\narm-gcc 4.9.4\n=> "
	out := cleanResponse("version", raw)
	want := "U-Boot 2016.03\narm-gcc 4.9.4"
	if out != want {
		t.Fatalf("cleanResponse = %q, want %q", out, want)
	}
}

func TestCleanResponseHandlesUBCR(t *testing.T) {
	// The raw bytes use "\n\r"; cleanResponse normalizes them.
	raw := "printenv bootargs\n\rbootargs=console=ttymxc0\n\r=> "
	out := cleanResponse("printenv bootargs", raw)
	if !bytes.Contains([]byte(out), []byte("bootargs=console=ttymxc0")) {
		t.Fatalf("cleanResponse lost content: %q", out)
	}
}
