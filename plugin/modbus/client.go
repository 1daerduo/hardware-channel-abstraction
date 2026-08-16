package modbus

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
)

func errModbus(msg string) error { return fmt.Errorf("modbus: %s", msg) }

// client is a minimal Modbus TCP client (read/write registers over MBAP).
type client struct {
	mu   sync.Mutex
	conn net.Conn
}

func dial(addr string) (*client, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	return &client{conn: conn}, nil
}

func (c *client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return nil
	}
	err := c.conn.Close()
	c.conn = nil
	return err
}

// ReadHoldingRegisters reads holding registers (FC 0x03).
func (c *client) ReadHoldingRegisters(start, qty uint16) ([]uint16, error) {
	return c.transact(0x03, start, qty)
}

// ReadInputRegisters reads input registers (FC 0x04).
func (c *client) ReadInputRegisters(start, qty uint16) ([]uint16, error) {
	return c.transact(0x04, start, qty)
}

// WriteRegister writes a single holding register (FC 0x06).
func (c *client) WriteRegister(addr, val uint16) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	pdu := []byte{0x06, byte(addr >> 8), byte(addr), byte(val >> 8), byte(val)}
	if err := c.writePDU(pdu); err != nil {
		return err
	}
	resp, err := c.readResp()
	if err != nil {
		return err
	}
	if len(resp) < 5 || resp[0] != 0x06 {
		return errModbus("unexpected write response")
	}
	return nil
}

// transact performs a read-registers transaction (FC 0x03/0x04).
func (c *client) transact(fc byte, start, qty uint16) ([]uint16, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	pdu := []byte{fc, byte(start >> 8), byte(start), byte(qty >> 8), byte(qty)}
	if err := c.writePDU(pdu); err != nil {
		return nil, err
	}
	resp, err := c.readResp()
	if err != nil {
		return nil, err
	}
	if len(resp) < 3 || resp[0] != fc {
		return nil, errModbus("unexpected read response")
	}
	count := int(resp[1])
	vals := make([]uint16, 0, count/2)
	for i := 0; i+1 < count; i += 2 {
		vals = append(vals, binary.BigEndian.Uint16(resp[2+i:4+i]))
	}
	return vals, nil
}

// writePDU wraps a PDU in MBAP and writes it.
func (c *client) writePDU(pdu []byte) error {
	out := make([]byte, 7+len(pdu))
	binary.BigEndian.PutUint16(out[0:2], 1) // transaction id
	binary.BigEndian.PutUint16(out[4:6], uint16(1+len(pdu)))
	out[6] = 1 // unit id
	copy(out[7:], pdu)
	_, err := c.conn.Write(out)
	return err
}

// readResp reads a full MBAP response and returns its PDU.
func (c *client) readResp() ([]byte, error) {
	hdr := make([]byte, 7)
	if _, err := io.ReadFull(c.conn, hdr); err != nil {
		return nil, err
	}
	length := binary.BigEndian.Uint16(hdr[4:6])
	pdu := make([]byte, length-1)
	if _, err := io.ReadFull(c.conn, pdu); err != nil {
		return nil, err
	}
	return pdu, nil
}
