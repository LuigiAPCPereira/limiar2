package dashboard

import (
	"errors"
	"sync"
)

const clientBuffer = 64

// maxSubscribers limita o número de clientes SSE simultâneos para evitar
// exaustão de recursos por abertura descontrolada de conexões.
const maxSubscribers = 64

// ErrMaxSubscribers é retornado quando o limite de assinantes SSE é atingido.
var ErrMaxSubscribers = errors.New("limite de assinantes SSE atingido")

// Event é um único evento SSE (Server-Sent Events) enviado aos clientes do dashboard conectados.
type Event struct {
	Type string
	Data []byte
}

// Broker é um hub pub/sub para eventos SSE. Ele gerencia as assinaturas (subscriptions)
// dos clientes e o fan-out. O Publish é não-bloqueante: se o buffer de um cliente estiver cheio,
// o evento é descartado para aquele cliente para evitar o bloqueio de quem publicou.
type Broker struct {
	mu      sync.RWMutex
	clients map[chan Event]struct{}
}

// NewBroker cria um Broker pronto para uso.
func NewBroker() *Broker {
	return &Broker{clients: make(map[chan Event]struct{})}
}

// Subscribe registra um novo cliente e retorna seu canal de eventos.
// Retorna ErrMaxSubscribers se o limite de conexões simultâneas for atingido.
// O chamador deve invocar Unsubscribe quando terminar.
func (b *Broker) Subscribe() (chan Event, error) {
	b.mu.Lock()
	if len(b.clients) >= maxSubscribers {
		b.mu.Unlock()
		return nil, ErrMaxSubscribers
	}
	ch := make(chan Event, clientBuffer)
	b.clients[ch] = struct{}{}
	b.mu.Unlock()
	return ch, nil
}

// Unsubscribe remove o canal de um cliente e o fecha.
func (b *Broker) Unsubscribe(ch chan Event) {
	b.mu.Lock()
	delete(b.clients, ch)
	b.mu.Unlock()
	close(ch)
}

// Publish envia um evento para todos os clientes conectados. Se o buffer de um cliente
// estiver cheio, o evento é descartado para ele (não-bloqueante).
func (b *Broker) Publish(event Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.clients {
		select {
		case ch <- event:
		default:
			// Buffer do cliente cheio — descarta o evento para este cliente.
		}
	}
}
