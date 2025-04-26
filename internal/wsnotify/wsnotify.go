package wsnotify

import (
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type WebSocketManager struct {
	clients map[*websocket.Conn]bool
	lock    sync.RWMutex
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func Upgrader() *websocket.Upgrader {
	return &upgrader
}

var Manager = &WebSocketManager{
	clients: make(map[*websocket.Conn]bool),
}

func (m *WebSocketManager) AddClient(conn *websocket.Conn) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.clients[conn] = true
}

func (m *WebSocketManager) RemoveClient(conn *websocket.Conn) {
	m.lock.Lock()
	defer m.lock.Unlock()
	delete(m.clients, conn)
}

func (m *WebSocketManager) Broadcast(event interface{}) {
	m.lock.RLock()
	defer m.lock.RUnlock()
	for client := range m.clients {
		client.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if err := client.WriteJSON(event); err != nil {
			client.Close()
			go m.RemoveClient(client)
		}
	}
}

type MessagePayload struct {
	ID        int     `json:"id"`
	ContactID int     `json:"contactID"`
	SectorID  int     `json:"sectorId"`
	Content   string  `json:"content"`
	MediaType string  `json:"mediaType"`
	MediaUrl  *string `json:"mediaUrl"`
	FileName  *string `json:"fileName"`
	MimeType  *string `json:"mimeType"`
	SentAt    string  `json:"sentAt"`
	IsSent    bool    `json:"isSent"`
	IsRead    bool    `json:"isRead"`
}

type MessageEvent struct {
	Type    string         `json:"type"`
	Payload MessagePayload `json:"payload"`
}

func SendMessageEvent(
	id int,
	contactID int,
	sectorID int,
	content string,
	mediaType string,
	mediaUrl *string,
	fileName *string,
	mimeType *string,
	sentAt time.Time,
	isSent bool,
	isRead bool,
) {
	payload := MessagePayload{
		ID:        id,
		ContactID: contactID,
		SectorID:  sectorID,
		Content:   content,
		MediaType: mediaType,
		MediaUrl:  mediaUrl,
		FileName:  fileName,
		MimeType:  mimeType,
		SentAt:    sentAt.UTC().Format(time.RFC3339Nano),
		IsSent:    isSent,
		IsRead:    isRead,
	}
	event := MessageEvent{
		Type:    "message",
		Payload: payload,
	}
	Manager.Broadcast(event)
}
