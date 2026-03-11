package transport

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"
)

// Bus is a non-blocking event emitter backed by a buffered channel.
// The consumer reads from the receive-only channel returned by NewBus.
type Bus struct {
	ch chan<- Event
}

// NewBus creates a Bus and the corresponding receive channel.
// The caller owns closing via Bus.Close.
func NewBus(size int) (*Bus, <-chan Event) {
	ch := make(chan Event, size)
	return &Bus{ch: ch}, ch
}

// Emit sends an event without blocking. If the channel is full the event
// is dropped and a warning is logged.
func (b *Bus) Emit(kind Kind, data any) {
	if b == nil || b.ch == nil {
		return
	}
	select {
	case b.ch <- Event{
		Timestamp: time.Now(),
		Kind:      kind,
		Data:      data,
	}:
	default:
		log.Warn().Any("kind", kind).Msg("event dropped — bus full")
	}
}

// EmitBlocking sends an event, blocking until it is delivered or the context
// is cancelled. Use for events that must not be dropped (e.g. Ask).
func (b *Bus) EmitBlocking(ctx context.Context, kind Kind, data any) error {
	if b == nil || b.ch == nil {
		return context.Canceled
	}
	select {
	case b.ch <- Event{Timestamp: time.Now(), Kind: kind, Data: data}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Close closes the underlying channel, signalling consumers to stop.
func (b *Bus) Close() {
	if b != nil && b.ch != nil {
		close(b.ch)
	}
}
