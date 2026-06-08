package dashboard

import (
	"testing"
)

func TestBroker_SubscribeUnsubscribe(t *testing.T) {
	broker := NewBroker()

	// Test Subscribe
	ch := broker.Subscribe()
	if ch == nil {
		t.Fatal("Subscribe() returned nil channel")
	}

	broker.mu.RLock()
	numClients := len(broker.clients)
	_, exists := broker.clients[ch]
	broker.mu.RUnlock()

	if numClients != 1 {
		t.Fatalf("expected 1 client, got %d", numClients)
	}
	if !exists {
		t.Fatal("client channel not found in broker.clients")
	}

	// Test Unsubscribe
	broker.Unsubscribe(ch)

	broker.mu.RLock()
	numClients = len(broker.clients)
	broker.mu.RUnlock()

	if numClients != 0 {
		t.Fatalf("expected 0 clients, got %d", numClients)
	}

	// Verify channel is closed
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected channel to be closed, but received a value")
		}
	default:
		t.Fatal("expected channel to be closed, but it is blocking")
	}
}

func TestBroker_Publish(t *testing.T) {
	broker := NewBroker()

	// Subscribe 3 clients
	ch1 := broker.Subscribe()
	ch2 := broker.Subscribe()
	ch3 := broker.Subscribe()

	// Publish an event
	testEvent := Event{Type: "test", Data: []byte("test data")}
	broker.Publish(testEvent)

	// Verify all clients received the event
	checkEvent := func(ch chan Event, clientName string) {
		select {
		case ev := <-ch:
			if ev.Type != testEvent.Type || string(ev.Data) != string(testEvent.Data) {
				t.Fatalf("%s received incorrect event: expected %v, got %v", clientName, testEvent, ev)
			}
		default:
			t.Fatalf("%s did not receive event", clientName)
		}
	}

	checkEvent(ch1, "client 1")
	checkEvent(ch2, "client 2")
	checkEvent(ch3, "client 3")

	// Ensure no extra events are left
	for i, ch := range []chan Event{ch1, ch2, ch3} {
		select {
		case <-ch:
			t.Fatalf("client %d received unexpected extra event", i+1)
		default:
		}
	}
}

func TestBroker_Publish_NonBlocking_FullBuffer(t *testing.T) {
	broker := NewBroker()
	ch := broker.Subscribe()

	// Fill the buffer
	for i := 0; i < clientBuffer; i++ {
		broker.Publish(Event{Type: "fill", Data: []byte{byte(i)}})
	}

	// This should not block, but the event will be dropped
	broker.Publish(Event{Type: "dropped", Data: []byte("dropped data")})

	// Verify we can read exactly clientBuffer events
	for i := 0; i < clientBuffer; i++ {
		select {
		case ev := <-ch:
			if ev.Type != "fill" || ev.Data[0] != byte(i) {
				t.Fatalf("expected fill event %d, got %v", i, ev)
			}
		default:
			t.Fatalf("expected event %d, but channel is empty", i)
		}
	}

	// Verify there are no more events
	select {
	case ev := <-ch:
		t.Fatalf("expected channel to be empty, but got %v", ev)
	default:
	}
}

func TestBroker_Publish_NoSubscribers(t *testing.T) {
	broker := NewBroker()

	// This should simply do nothing and not panic or block
	broker.Publish(Event{Type: "test", Data: []byte("no one is listening")})
}
