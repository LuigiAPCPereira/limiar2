package dashboard

import "sync"

const clientBuffer = 64

// Event is a single SSE event pushed to connected dashboard clients.
type Event struct {
	Type string
	Data []byte
}

// Broker is a pub/sub hub for SSE events. It manages client subscriptions
// and fan-out. Publish is non-blocking: if a client's buffer is full the
// event is dropped for that client to avoid blocking the publisher.
type Broker struct {
	mu      sync.RWMutex
	clients map[chan Event]struct{}
}

// NewBroker creates a ready-to-use Broker.
func NewBroker() *Broker {
	return &Broker{clients: make(map[chan Event]struct{})}
}

// Subscribe registers a new client and returns its event channel.
// The caller must call Unsubscribe when done.
func (b *Broker) Subscribe() chan Event {
	ch := make(chan Event, clientBuffer)
	b.mu.Lock()
	b.clients[ch] = struct{}{}
	b.mu.Unlock()
	return ch
}

// Unsubscribe removes a client channel and closes it.
func (b *Broker) Unsubscribe(ch chan Event) {
	b.mu.Lock()
	delete(b.clients, ch)
	b.mu.Unlock()
	close(ch)
}

// Publish sends an event to all connected clients. If a client's buffer is
// full the event is dropped for that client (non-blocking).
func (b *Broker) Publish(event Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.clients {
		select {
		case ch <- event:
		default:
			// Client buffer full — drop event for this client.
		}
	}
}
