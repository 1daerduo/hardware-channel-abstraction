package serial

import (
	"bytes"
	"context"
	"sync"

	"example.com/embedded-loop-channel/plugin/sdk"
)

// consoleStream is a line-buffered, sequenced stream over a Console's reader
// pump. It satisfies sdk.Stream with at-least-once delivery and a close
// reason.
type consoleStream struct {
	console *Console
	id      string

	mu          sync.Mutex
	ch          chan []byte
	seq         uint64
	pending     []byte
	closed      bool
	closeReason string
}

func newConsoleStream(c *Console) *consoleStream {
	return &consoleStream{console: c, ch: make(chan []byte, 256)}
}

// ID returns the stable stream id.
func (s *consoleStream) ID() string { return s.id }

// push delivers raw bytes from the pump (non-blocking; drops on backpressure).
func (s *consoleStream) push(b []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	select {
	case s.ch <- append([]byte(nil), b...):
	default: // backpressure: drop rather than block the pump
	}
}

// Read returns the next full line as a sequenced chunk.
func (s *consoleStream) Read(ctx context.Context) (sdk.StreamChunk, error) {
	for {
		s.mu.Lock()
		if s.closed {
			reason := s.closeReason
			s.mu.Unlock()
			return sdk.StreamChunk{StreamID: s.id, Closed: true, CloseReason: reason}, nil
		}
		if line := extractLine(&s.pending); line != nil {
			s.seq++
			seq := s.seq
			s.mu.Unlock()
			return sdk.StreamChunk{StreamID: s.id, Sequence: seq, Data: line}, nil
		}
		s.mu.Unlock()

		select {
		case b := <-s.ch:
			s.mu.Lock()
			s.pending = append(s.pending, b...)
			s.mu.Unlock()
		case <-ctx.Done():
			return sdk.StreamChunk{}, ctx.Err()
		}
	}
}

// Cursor returns the highest sequence delivered so far.
func (s *consoleStream) Cursor() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.seq
}

// Close terminates the stream with a close reason and unregisters it.
func (s *consoleStream) Close(reason string) error {
	s.console.mu.Lock()
	delete(s.console.subs, s.id)
	s.console.mu.Unlock()

	s.mu.Lock()
	s.closed = true
	s.closeReason = reason
	s.mu.Unlock()
	return nil
}

// extractLine pops a full line (up to '\n') from the pending buffer and trims
// both leading and trailing '\r'. U-Boot uses "\n\r" line endings, so the '\r'
// of a line break lands at the start of the NEXT line.
func extractLine(p *[]byte) []byte {
	idx := bytes.IndexByte(*p, '\n')
	if idx < 0 {
		return nil
	}
	line := (*p)[:idx]
	*p = (*p)[idx+1:]
	return bytes.Trim(line, "\r")
}
