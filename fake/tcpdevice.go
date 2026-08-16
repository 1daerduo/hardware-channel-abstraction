package fake

import (
	"bufio"
	"net"
	"strings"
	"sync"
)

// ConsoleServer is a simulated TCP console device: it listens and answers
// console commands (version / info / reboot / echo ...) like a network-attached
// device. It stands in for real hardware in TCP transport tests and demos.
type ConsoleServer struct {
	mu     sync.Mutex
	ln     net.Listener
	addr   string
	closed bool
}

// NewConsoleServer listens on addr (e.g. "127.0.0.1:0") and starts serving.
func NewConsoleServer(addr string) (*ConsoleServer, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	s := &ConsoleServer{ln: ln, addr: ln.Addr().String()}
	go s.serve()
	return s, nil
}

// Addr returns the bound listen address.
func (s *ConsoleServer) Addr() string { return s.addr }

// Close stops the server.
func (s *ConsoleServer) Close() error {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	return s.ln.Close()
}

func (s *ConsoleServer) serve() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		go s.handle(conn)
	}
}

// handle serves one connection: read commands terminated by '\r', echo them,
// respond, and print the "=> " prompt.
func (s *ConsoleServer) handle(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	for {
		line, err := reader.ReadString('\r')
		if err != nil {
			return
		}
		cmd := strings.Trim(line, "\r\n\t ")
		if cmd == "" {
			continue
		}
		conn.Write([]byte(cmd + "\n"))          // echo
		conn.Write([]byte(respond(cmd) + "\n")) // response
		conn.Write([]byte("=> "))               // prompt
	}
}

func respond(cmd string) string {
	switch {
	case cmd == "version":
		return "TCP-Device 1.0"
	case cmd == "get_device_info" || cmd == "info":
		return "model=TCP-Device\nfirmware=1.0\nstate=up"
	case cmd == "reboot":
		return "rebooted"
	case cmd == "reset":
		return "reset"
	case strings.HasPrefix(cmd, "echo "):
		return strings.TrimPrefix(cmd, "echo ")
	default:
		return "unknown command: " + cmd
	}
}
