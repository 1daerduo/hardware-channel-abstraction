// Package console provides a transport-agnostic byte-stream console device:
// a single-reader pump over any io.ReadWriteCloser (serial port, net.Conn,
// net.Pipe...), with command/response execution and a live line stream.
//
// It is the reusable core behind both transport/serial and transport/tcp: a new
// byte-stream transport only wraps its connection in console.New.
package console

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/1daerduo/hardware-channel-abstraction/plugin/sdk"
)

// Prompt is the shell prompt used to delimit command responses.
const Prompt = "=> "

// Config describes the transport identity of a console.
type Config struct {
	Kind    string // "serial" / "tcp" ...（stream id 前缀）
	Name    string // 传输 locator（串口路径 / tcp 地址），也作为 serial 身份
	Model   string // 设备型号（身份提示，缺省 "embedded"）
	Console string // 控制台类型（身份提示，缺省 "uboot"）
}

// Console drives a byte-stream console as a sdk.ConsoleDevice.
type Console struct {
	mu        sync.Mutex
	cfg       Config
	rwc       io.ReadWriteCloser
	bootState string
	identity  map[string]string
	buf       []byte // accumulated console output (guarded by mu)

	subs    map[string]*consoleStream
	nextSub uint64
	closed  bool
}

// New builds a Console over an established byte-stream connection and starts
// the reader pump.
func New(rwc io.ReadWriteCloser, cfg Config) *Console {
	if cfg.Model == "" {
		cfg.Model = "embedded"
	}
	if cfg.Console == "" {
		cfg.Console = "uboot"
	}
	if cfg.Kind == "" {
		cfg.Kind = "console"
	}
	con := &Console{
		cfg:       cfg,
		rwc:       rwc,
		bootState: "uboot",
		identity:  map[string]string{"serial": cfg.Name, "model": cfg.Model, "console": cfg.Console},
		subs:      map[string]*consoleStream{},
	}
	go con.readLoop()
	return con
}

// Path returns the transport locator.
func (c *Console) Path() string { return c.cfg.Name }

// Close stops the pump and closes the underlying connection.
func (c *Console) Close() error {
	c.mu.Lock()
	if c.rwc == nil {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	rwc := c.rwc
	c.rwc = nil
	err := rwc.Close()
	c.mu.Unlock()
	return err
}

// Identity returns strong identity signals for correlation.
func (c *Console) Identity() map[string]string { return c.identity }

// IsOnline reports whether the connection is open.
func (c *Console) IsOnline() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.rwc != nil
}

// CurrentBootState returns the observed bootloader state.
func (c *Console) CurrentBootState() string { return c.bootState }

// Console returns the accumulated console output.
func (c *Console) Console() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.rwc == nil {
		return "", fmt.Errorf("console: not connected")
	}
	return string(c.buf), nil
}

// Log returns the console log.
func (c *Console) Log() (string, error) { return c.Console() }

// Execute sends a command and waits for the prompt to return (or a timeout).
func (c *Console) Execute(cmd string) (string, error) {
	c.mu.Lock()
	if c.rwc == nil {
		c.mu.Unlock()
		return "", fmt.Errorf("console: not connected")
	}
	start := len(c.buf)
	if _, err := c.rwc.Write([]byte(cmd + "\r")); err != nil {
		c.mu.Unlock()
		return "", fmt.Errorf("console: write: %w", err)
	}
	c.mu.Unlock()

	deadline := time.Now().Add(5 * time.Second)
	for {
		c.mu.Lock()
		raw := append([]byte(nil), c.buf[start:]...)
		c.mu.Unlock()
		if strings.Contains(string(raw), Prompt) || time.Now().After(deadline) {
			return CleanResponse(cmd, string(raw)), nil
		}
		time.Sleep(30 * time.Millisecond)
	}
}

// Reset issues the reset command.
func (c *Console) Reset() error { _, err := c.Execute("reset"); return err }

// PowerCycle performs a hard reset via the console.
func (c *Console) PowerCycle() error { return c.Reset() }

// OpenStream returns a live line-buffered console stream.
func (c *Console) OpenStream(_ context.Context) (sdk.Stream, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.rwc == nil {
		return nil, fmt.Errorf("console: not connected")
	}
	s := newConsoleStream(c)
	s.id = fmt.Sprintf("%s-%s-%d", c.cfg.Kind, c.cfg.Name, c.nextSub)
	c.nextSub++
	c.subs[s.id] = s
	return s, nil
}

// readLoop is the single reader: appends to the shared buffer and fans new
// bytes out to active streams.
func (c *Console) readLoop() {
	buf := make([]byte, 4096)
	for {
		c.mu.Lock()
		rwc := c.rwc
		done := c.closed
		c.mu.Unlock()
		if done || rwc == nil {
			return
		}
		n, err := rwc.Read(buf)
		if n > 0 {
			c.mu.Lock()
			c.buf = append(c.buf, buf[:n]...)
			for _, s := range c.subs {
				s.push(buf[:n])
			}
			c.mu.Unlock()
		}
		if err == io.EOF {
			// Peer closed the connection; stop the pump. A serial read
			// timeout is NOT io.EOF, so idle serial ports keep polling.
			return
		}
	}
}

// CleanResponse strips the echoed command and trailing prompt, and normalizes
// line endings.
func CleanResponse(cmd, raw string) string {
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
