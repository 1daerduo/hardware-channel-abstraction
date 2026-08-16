// Package serial is a thin wrapper: a real serial port backed by the generic
// byte-stream console. It satisfies sdk.ConsoleDevice by wrapping an opened
// serial.Port in console.New.
package serial

import (
	"time"

	"example.com/embedded-loop-channel/transport/console"
	serialpkg "github.com/tarm/serial"
)

// Console is the byte-stream console over a serial port.
type Console = console.Console

// Open opens a serial port at the given baud and returns a Console over it.
func Open(path string, baud int) (*Console, error) {
	c := &serialpkg.Config{Name: path, Baud: baud, ReadTimeout: 200 * time.Millisecond}
	port, err := serialpkg.OpenPort(c)
	if err != nil {
		return nil, err
	}
	return console.New(port, console.Config{Kind: "serial", Name: path}), nil
}
