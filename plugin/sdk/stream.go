package sdk

import (
	"context"

	"example.com/embedded-loop-channel/domain"
)

// StreamChunk is one piece of streamed data. Sequence is monotonic within a
// stream and supports cursor-based resume (Design doc 08 §8). Closed marks the
// terminal chunk; CloseReason explains why the stream ended.
type StreamChunk struct {
	StreamID    string
	Sequence    uint64
	Data        []byte
	Closed      bool
	CloseReason string
}

// Stream is a continuous data stream (console / log / trace). Consumers read
// chunks in order; delivery is at-least-once and consumers dedupe by
// (StreamID, Sequence).
type Stream interface {
	// ID returns the stable stream id.
	ID() string
	// Read returns the next chunk, blocking until data arrives, the stream
	// closes, or ctx is done. A Closed chunk is the terminal chunk.
	Read(ctx context.Context) (StreamChunk, error)
	// Cursor returns the highest sequence delivered so far.
	Cursor() uint64
	// Close terminates the stream with a close reason.
	Close(reason string) error
}

// StreamRequest describes what to stream.
type StreamRequest struct {
	Capability domain.CapabilityName
	Subject    string
}

// Streamer is an OPTIONAL plugin capability: not every protocol streams. The
// Core type-asserts a plugin against it before opening a stream.
type Streamer interface {
	Stream(ctx context.Context, channel *domain.Channel, req StreamRequest) (Stream, error)
}

// StreamProvider is implemented by transports that can produce a live stream.
// A plugin can assert its transport against it.
type StreamProvider interface {
	OpenStream(ctx context.Context) (Stream, error)
}

// Canceller is an OPTIONAL plugin capability for cooperative cancellation of
// an in-flight operation. A plugin that cannot cancel a protocol action
// returns a normalized NOT_CANCELLABLE error (Design doc 06 §12).
type Canceller interface {
	Cancel(ctx context.Context, channel *domain.Channel, operationID domain.OperationID) error
}
