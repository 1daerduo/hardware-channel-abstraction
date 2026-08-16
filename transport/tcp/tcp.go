// Package tcp is a thin wrapper: a TCP-connected console device backed by the
// generic byte-stream console. It proves a new byte-stream transport is ~5
// lines: dial a net.Conn and wrap it in console.New.
package tcp

import (
	"net"

	"github.com/1daerduo/hardware-channel-abstraction/transport/console"
)

// Console is the byte-stream console over a TCP connection.
type Console = console.Console

// Dial connects to a TCP console device and returns a Console over it.
func Dial(addr string) (*Console, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	return console.New(conn, console.Config{Kind: "tcp", Name: addr, Model: "tcp-device", Console: "console"}), nil
}
