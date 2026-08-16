package fake

import (
	"encoding/binary"
	"io"
	"net"
	"sync"
)

// ModbusServer is a simulated Modbus TCP device: it listens and answers read/
// write register requests (function codes 0x03/0x04/0x06). It stands in for a
// real PLC/sensor in Modbus transport tests and demos.
type ModbusServer struct {
	mu      sync.RWMutex
	ln      net.Listener
	addr    string
	holding map[uint16]uint16
	input   map[uint16]uint16
	closed  bool
}

// NewModbusServer listens on addr (e.g. "127.0.0.1:0") and starts serving.
func NewModbusServer(addr string) (*ModbusServer, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	s := &ModbusServer{
		ln:      ln,
		addr:    ln.Addr().String(),
		holding: map[uint16]uint16{0: 0x1234, 1: 0x5678, 2: 0x9ABC},
		input:   map[uint16]uint16{0: 0x00FF, 1: 0x0100, 2: 0x0037},
	}
	go s.serve()
	return s, nil
}

// Addr returns the bound listen address.
func (s *ModbusServer) Addr() string { return s.addr }

// SetHolding sets a holding-register value (test/setup).
func (s *ModbusServer) SetHolding(addr, value uint16) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.holding[addr] = value
}

// Holding returns a holding-register value.
func (s *ModbusServer) Holding(addr uint16) uint16 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.holding[addr]
}

// Close stops the server.
func (s *ModbusServer) Close() error {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	return s.ln.Close()
}

func (s *ModbusServer) serve() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		go s.handle(conn)
	}
}

func (s *ModbusServer) handle(conn net.Conn) {
	defer conn.Close()
	hdr := make([]byte, 7)
	for {
		if _, err := io.ReadFull(conn, hdr); err != nil {
			return
		}
		length := binary.BigEndian.Uint16(hdr[4:6])
		pdu := make([]byte, length-1)
		if _, err := io.ReadFull(conn, pdu); err != nil {
			return
		}
		if _, err := conn.Write(s.handlePDU(hdr, pdu)); err != nil {
			return
		}
	}
}

// handlePDU answers a Modbus request PDU with a response PDU (framed in MBAP).
func (s *ModbusServer) handlePDU(hdr, pdu []byte) []byte {
	if len(pdu) == 0 {
		return mbap(hdr, []byte{pdu[0] | 0x80, 0x01})
	}
	switch pdu[0] {
	case 0x03: // read holding registers
		return s.readRegs(hdr, pdu, s.holding)
	case 0x04: // read input registers
		return s.readRegs(hdr, pdu, s.input)
	case 0x06: // write single register
		return s.writeSingle(hdr, pdu)
	default:
		return mbap(hdr, []byte{pdu[0] | 0x80, 0x01}) // illegal function
	}
}

func (s *ModbusServer) readRegs(hdr, pdu []byte, regs map[uint16]uint16) []byte {
	start := binary.BigEndian.Uint16(pdu[1:3])
	qty := binary.BigEndian.Uint16(pdu[3:5])
	s.mu.RLock()
	vals := make([]uint16, qty)
	for i := 0; i < int(qty); i++ {
		vals[i] = regs[start+uint16(i)]
	}
	s.mu.RUnlock()

	rpdu := []byte{pdu[0], byte(qty * 2)}
	for _, v := range vals {
		rpdu = append(rpdu, byte(v>>8), byte(v&0xFF))
	}
	return mbap(hdr, rpdu)
}

func (s *ModbusServer) writeSingle(hdr, pdu []byte) []byte {
	addr := binary.BigEndian.Uint16(pdu[1:3])
	val := binary.BigEndian.Uint16(pdu[3:5])
	s.mu.Lock()
	s.holding[addr] = val
	s.mu.Unlock()
	return mbap(hdr, pdu) // echo the request
}

// mbap wraps a response PDU in the MBAP header (echoes txid + unit id).
func mbap(hdr, pdu []byte) []byte {
	out := make([]byte, 7+len(pdu))
	copy(out[0:2], hdr[0:2]) // transaction id
	// protocol id = 0
	binary.BigEndian.PutUint16(out[4:6], uint16(1+len(pdu)))
	out[6] = hdr[6] // unit id
	copy(out[7:], pdu)
	return out
}
