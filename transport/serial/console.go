// Package serial is a thin wrapper: a real serial port backed by the generic
// byte-stream console. It satisfies sdk.ConsoleDevice by wrapping an opened
// serial.Port in console.New. Uses go.bug.st/serial, which is cross-platform
// (linux / windows / macOS / bsd).
package serial

import (
	"time"

	"github.com/1daerduo/hardware-channel-abstraction/transport/console"

	"go.bug.st/serial"
)

// Console is the byte-stream console over a serial port.
type Console = console.Console

// Open opens a serial port (8N1) at the given baud and returns a Console over
// it.
func Open(path string, baud int) (*Console, error) {
	port, err := serial.Open(path, &serial.Mode{
		BaudRate: baud,
		DataBits: 8,
		Parity:   serial.NoParity,
		StopBits: serial.OneStopBit,
	})
	if err != nil {
		return nil, err
	}
	// A 200ms read timeout lets the console pump observe shutdown promptly.
	_ = port.SetReadTimeout(200 * time.Millisecond)
	return console.New(port, console.Config{Kind: "serial", Name: path}), nil
}
