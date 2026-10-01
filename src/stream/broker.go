package stream

import (
	"sync"

	"github.com/anandpsdev/apiwatch/src/types"
)

type Broker struct {
	mu          sync.RWMutex
	subscribers map[chan types.Entry]struct{}
	closed      bool
}

func NewBroker() *Broker {
	return &Broker{
		subscribers: make(map[chan types.Entry]struct{}),
	}
}

func (b *Broker) Subscribe() chan types.Entry {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan types.Entry, 128)
	if b.closed {
		close(ch)
		return ch
	}
	b.subscribers[ch] = struct{}{}
	return ch
}

func (b *Broker) Unsubscribe(ch chan types.Entry) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if _, exists := b.subscribers[ch]; exists {
		delete(b.subscribers, ch)
		close(ch)
	}
}

func (b *Broker) Publish(entry types.Entry) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return
	}

	for ch := range b.subscribers {
		select {
		case ch <- entry:
		default:
		}
	}
}

func (b *Broker) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return
	}
	b.closed = true

	for ch := range b.subscribers {
		close(ch)
	}
	b.subscribers = make(map[chan types.Entry]struct{})
}
