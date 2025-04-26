package handlers

import (
	"net/http"
	"time"
	"whatsapp-bot/internal/wsnotify"
)

func WebSocketHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := wsnotify.Upgrader().Upgrade(w, r, nil)
	if err != nil {
		return
	}
	wsnotify.Manager.AddClient(conn)
	defer func() {
		wsnotify.Manager.RemoveClient(conn)
		conn.Close()
	}()
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
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
	wsnotify.Manager.Broadcast(event)
}
