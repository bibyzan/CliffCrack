// Package online connects players for a match: the lobby (rooms, via the
// coordinator), the WebRTC links from the host to each guest, and the
// messages that travel over them.
//
// The host runs the match (package arena) and is the only one that steps
// it; guests send their inputs and apply what the host sends back. Each
// link has two data channels: a fast, unordered, unreliable one for the
// stream of inputs and snapshots (only the latest matters), and a reliable,
// ordered one for events (every break of the site, every hit and kill) and
// control (the match starting).
package online

import (
	"errors"
	"sync"
)

// Channel picks how a message travels.
type Channel int

const (
	Fast     Channel = iota // unordered, unreliable: inputs and snapshots
	Reliable                // ordered, reliable: events and control
)

// Packet is a message received on a link.
type Packet struct {
	Channel Channel
	Data    []byte
}

// Link is a connection to one other player.
type Link interface {
	Send(ch Channel, data []byte) error
	// Recv is where the other side's messages arrive. It's closed when the
	// link goes down.
	Recv() <-chan Packet
	Close() error
}

// ErrClosed is returned for sends on a closed link.
var ErrClosed = errors.New("online: link closed")

// memLink is one end of an in-memory link, for tests.
type memLink struct {
	in     chan Packet
	peer   *memLink
	mu     sync.Mutex
	closed bool
	// drop, if set, loses Fast packets it returns true for (simulated loss).
	drop func() bool
}

// MemLinks makes a connected pair of in-memory links.
func MemLinks() (Link, Link) {
	a := &memLink{in: make(chan Packet, 1024)}
	b := &memLink{in: make(chan Packet, 1024)}
	a.peer, b.peer = b, a
	return a, b
}

func (l *memLink) Send(ch Channel, data []byte) error {
	l.mu.Lock()
	closed := l.closed
	l.mu.Unlock()
	if closed {
		return ErrClosed
	}
	if ch == Fast && l.drop != nil && l.drop() {
		return nil
	}
	p := l.peer
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return ErrClosed
	}
	select {
	case p.in <- Packet{Channel: ch, Data: append([]byte(nil), data...)}:
	default:
		if ch == Reliable {
			return errors.New("online: in-memory link backed up")
		}
	}
	return nil
}

func (l *memLink) Recv() <-chan Packet { return l.in }

func (l *memLink) Close() error {
	for _, e := range []*memLink{l, l.peer} {
		e.mu.Lock()
		if !e.closed {
			e.closed = true
			close(e.in)
		}
		e.mu.Unlock()
	}
	return nil
}
