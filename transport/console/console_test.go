package console

import (
	"net"
	"testing"
	"time"
)

func TestExtractLineNormalizesUBCR(t *testing.T) {
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
	if line := extractLine(&p); string(line) != "a" {
		t.Fatalf("line = %q, want %q", line, "a")
	}
	if line := extractLine(&p); string(line) != "b" {
		t.Fatalf("line = %q, want %q", line, "b")
	}
	if extractLine(&p) != nil {
		t.Fatalf("expected exhausted buffer")
	}
}

func TestCleanResponseStripsEchoAndPrompt(t *testing.T) {
	raw := "version\n\nU-Boot 2016.03\narm-gcc 4.9.4\n=> "
	if out := CleanResponse("version", raw); out != "U-Boot 2016.03\narm-gcc 4.9.4" {
		t.Fatalf("CleanResponse = %q", out)
	}
}

// TestExecuteOverNetPipe verifies the console works over ANY byte stream — no
// real serial port or TCP socket needed. This is what makes a new transport a
// thin wrapper: net.Pipe (a net.Conn) behaves exactly like a serial port.
func TestExecuteOverNetPipe(t *testing.T) {
	server, client := net.Pipe()
	defer client.Close()

	// A canned device that echoes the command and answers "version".
	go func() {
		buf := make([]byte, 256)
		for {
			n, err := server.Read(buf)
			if n > 0 {
				cmd := string(buf[:n])
				server.Write([]byte(cmd)) // echo
				if cmd == "version\r" {
					server.Write([]byte("\nTCP-Device 1.0\n=> "))
				} else {
					server.Write([]byte("\nunknown\n=> "))
				}
			}
			if err != nil {
				return
			}
		}
	}()

	con := New(client, Config{Kind: "test", Name: "net-pipe"})
	defer con.Close()
	if !con.IsOnline() {
		t.Fatalf("console should be online")
	}

	out, err := con.Execute("version")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if out != "TCP-Device 1.0" {
		t.Fatalf("output = %q, want %q", out, "TCP-Device 1.0")
	}
	_ = time.Now
}
