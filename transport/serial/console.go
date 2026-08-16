// Package serial provides a real serial-port ConsoleDevice for the UART
// plugin. It speaks the interactive command/response protocol of an embedded
// bootloader (U-Boot): write a command, read until the prompt returns.
//
// The Console runs a single reader goroutine (the "pump") that is the only
// reader of the port. Commands (Execute) write and then poll the shared
// buffer for the prompt; live streams subscribe to the same pump, so console
// output can be consumed as an ordered, sequenced Stream.
package serial

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"example.com/embedded-loop-channel/plugin/sdk"

	"github.com/tarm/serial"
)

// Prompt is the U-Boot shell prompt used to delimit command responses.
const Prompt = "=> "

// Console drives a real serial port as a sdk.ConsoleDevice.
type Console struct {
	mu   sync.Mutex
	path string
	baud int
	port *serial.Port

	bootState string
	identity  map[string]string
	buf       []byte // accumulated console output (guarded by mu)

	subs    map[string]*consoleStream
	nextSub uint64
	closed  bool
}

// Open opens a serial port, starts the reader pump and returns a Console.
func Open(path string, baud int) (*Console, error) {
	c := &serial.Config{Name: path, Baud: baud, ReadTimeout: 200 * time.Millisecond}
	port, err := serial.OpenPort(c)
	if err != nil {
		return nil, fmt.Errorf("serial: open %s: %w", path, err)
	}
	con := &Console{
		path:      path,
		baud:      baud,
		port:      port,
		bootState: "uboot",
		identity:  map[string]string{"serial": path, "model": "embedded", "console": "uboot"},
		subs:      map[string]*consoleStream{},
	}
	go con.readLoop()
	return con, nil
}

// Path returns the serial port path.
func (c *Console) Path() string { return c.path }

// Close stops the pump and closes the port.
func (c *Console) Close() error {
	c.mu.Lock()
	if c.port == nil {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	port := c.port
	c.port = nil
	// Close the port so a blocked Read returns.
	err := port.Close()
	c.mu.Unlock()
	return err
}

// Identity returns strong identity signals for correlation.
func (c *Console) Identity() map[string]string { return c.identity }

// IsOnline reports whether the port is open.
func (c *Console) IsOnline() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.port != nil
}

// CurrentBootState returns the observed bootloader state.
func (c *Console) CurrentBootState() string { return c.bootState }

// Console returns the accumulated console output.
func (c *Console) Console() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.port == nil {
		return "", fmt.Errorf("serial: port not open")
	}
	return string(c.buf), nil
}

// Log returns the console log (alias of Console for the serial transport).
func (c *Console) Log() (string, error) { return c.Console() }

// Execute sends a command and waits for the U-Boot prompt to return (or a
// timeout). It returns the cleaned response text.
func (c *Console) Execute(cmd string) (string, error) {
	c.mu.Lock()
	if c.port == nil {
		c.mu.Unlock()
		return "", fmt.Errorf("serial: port not open")
	}
	start := len(c.buf)
	if _, err := c.port.Write([]byte(cmd + "\r")); err != nil {
		c.mu.Unlock()
		return "", fmt.Errorf("serial: write: %w", err)
	}
	c.mu.Unlock()

	deadline := time.Now().Add(5 * time.Second)
	for {
		c.mu.Lock()
		raw := append([]byte(nil), c.buf[start:]...)
		c.mu.Unlock()
		if strings.Contains(string(raw), Prompt) || time.Now().After(deadline) {
			return cleanResponse(cmd, string(raw)), nil
		}
		time.Sleep(30 * time.Millisecond)
	}
}

// Reset issues the U-Boot reset command.
func (c *Console) Reset() error {
	_, err := c.Execute("reset")
	return err
}

// PowerCycle performs a hard reset via the bootloader.
func (c *Console) PowerCycle() error { return c.Reset() }

// OpenStream returns a live line-buffered console stream.
func (c *Console) OpenStream(_ context.Context) (sdk.Stream, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.port == nil {
		return nil, fmt.Errorf("serial: port not open")
	}
	s := newConsoleStream(c)
	s.id = fmt.Sprintf("serial-%s-%d", c.path, c.nextSub)
	c.nextSub++
	c.subs[s.id] = s
	return s, nil
}

// readLoop is the single reader of the port: it appends to the shared buffer
// and fans new bytes out to active streams.
func (c *Console) readLoop() {
	buf := make([]byte, 4096)
	for {
		n, _ := c.port.Read(buf)
		if n > 0 {
			c.mu.Lock()
			c.buf = append(c.buf, buf[:n]...)
			for _, s := range c.subs {
				s.push(buf[:n])
			}
			c.mu.Unlock()
		}
		c.mu.Lock()
		done := c.closed
		c.mu.Unlock()
		if done {
			return
		}
	}
}

// cleanResponse strips the echoed command and the trailing prompt, and
// normalizes line endings.
func cleanResponse(cmd, raw string) string {
	s := strings.ReplaceAll(raw, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\n\r", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")

	if i := strings.Index(s, "\n"); i >= 0 {
		first := s[:i]
		if strings.Contains(first, cmd) {
			s = s[i+1:]
		}
	} else if strings.HasPrefix(s, cmd) {
		s = strings.TrimPrefix(s, cmd)
	}

	if idx := strings.LastIndex(s, Prompt); idx >= 0 {
		s = s[:idx]
	}
	return strings.TrimSpace(s)
}
