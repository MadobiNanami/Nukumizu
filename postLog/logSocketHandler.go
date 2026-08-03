package postLog

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type LogSocketHandler struct {
	broadcaster *LogBroadcaster
}

func NewLogSocketHandler(b *LogBroadcaster) *LogSocketHandler {
	return &LogSocketHandler{
		broadcaster: b,
	}
}

func (h *LogSocketHandler) Handle(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade connection: %v", err)
		return
	}
	defer conn.Close()

	clientID := conn.RemoteAddr().String() + "-" + time.Now().Format("20060102150405")
	clientCh := h.broadcaster.AddClient(clientID)
	defer h.broadcaster.RemoveClient(clientID)

	h.broadcaster.SendHistory(clientCh)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			msg := <-clientCh
			data, err := json.Marshal(msg)
			if err != nil {
				log.Printf("Failed to marshal log message: %v", err)
				return
			}
			if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
				return
			}
		}
	}()

	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			return
		}
	}
}
