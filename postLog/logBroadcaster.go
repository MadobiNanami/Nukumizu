package postLog

import (
	"sync"
)

type LogMessage struct {
	Level     int    `json:"level"`
	Content   string `json:"content"`
	Timestamp string `json:"timestamp"`
}

type Client struct {
	ID       string
	Messages chan LogMessage
}

type LogBroadcaster struct {
	mu       sync.RWMutex
	clients  map[string]chan LogMessage
	history  []LogMessage
	historyM sync.RWMutex
}

var broadcaster *LogBroadcaster

func InitLogBroadcaster() {
	broadcaster = &LogBroadcaster{
		clients: make(map[string]chan LogMessage),
		history: make([]LogMessage, 0),
	}
}

func GetLogBroadcaster() *LogBroadcaster {
	return broadcaster
}

func (lb *LogBroadcaster) AddClient(id string) chan LogMessage {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	ch := make(chan LogMessage, 100)
	lb.clients[id] = ch
	return ch
}

func (lb *LogBroadcaster) RemoveClient(id string) {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	if ch, exists := lb.clients[id]; exists {
		close(ch)
		delete(lb.clients, id)
	}
}

func (lb *LogBroadcaster) Broadcast(msg LogMessage) {
	lb.mu.RLock()
	defer lb.mu.RUnlock()

	for _, ch := range lb.clients {
		select {
		case ch <- msg:
		default:
		}
	}
}

func (lb *LogBroadcaster) GetHistory() []LogMessage {
	lb.historyM.RLock()
	defer lb.historyM.RUnlock()

	result := make([]LogMessage, len(lb.history))
	copy(result, lb.history)
	return result
}

func (lb *LogBroadcaster) AddToHistory(msg LogMessage) {
	lb.historyM.Lock()
	defer lb.historyM.Unlock()

	lb.history = append(lb.history, msg)
	if len(lb.history) > 100 {
		lb.history = lb.history[1:]
	}
}

func (lb *LogBroadcaster) SendHistory(clientCh chan LogMessage) {
	history := lb.GetHistory()
	for _, msg := range history {
		select {
		case clientCh <- msg:
		default:
		}
	}
}
