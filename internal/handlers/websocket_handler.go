package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"time"
	"whatsapp-bot/internal/wsnotify"
)

func WebSocketHandler(w http.ResponseWriter, r *http.Request) {
	// Extrair sector_id do parâmetro de consulta
	sectorIDStr := r.URL.Query().Get("sector_id")
	if sectorIDStr == "" {
		fmt.Printf("[DEBUG-WS] Tentativa de conexão sem sector_id\n")
		http.Error(w, "Missing sector_id parameter", http.StatusBadRequest)
		return
	}

	sectorID, err := strconv.Atoi(sectorIDStr)
	if err != nil {
		fmt.Printf("[DEBUG-WS] sector_id inválido: %s\n", sectorIDStr)
		http.Error(w, "Invalid sector_id parameter", http.StatusBadRequest)
		return
	}

	fmt.Printf("[DEBUG-WS] Tentando estabelecer conexão WebSocket para setor %d\n", sectorID)
	conn, err := wsnotify.Upgrader().Upgrade(w, r, nil)
	if err != nil {
		fmt.Printf("[DEBUG-WS] Erro ao fazer upgrade da conexão para setor %d: %v\n", sectorID, err)
		return
	}

	// Adicionar cliente com o sectorID
	wsnotify.Manager.AddClient(conn, sectorID)
	fmt.Printf("[DEBUG-WS] Conexão WebSocket estabelecida para setor %d\n", sectorID)

	defer func() {
		wsnotify.Manager.RemoveClient(conn)
		conn.Close()
		fmt.Printf("[DEBUG-WS] Conexão WebSocket fechada para setor %d\n", sectorID)
	}()

	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			fmt.Printf("[DEBUG-WS] Erro na leitura da mensagem do setor %d: %v\n", sectorID, err)
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

// Novo tipo para evento de contato
type ContactPayload struct {
	ID            int       `json:"id"`
	SectorID      int       `json:"sectorId"`
	Name          string    `json:"name"`
	Number        string    `json:"number"`
	Avatar        string    `json:"avatarUrl"`
	IsViewed      bool      `json:"isViewed"`
	ContactStatus string    `json:"contactStatus"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

type ContactEvent struct {
	Type    string         `json:"type"`
	Payload ContactPayload `json:"payload"`
}

// Novo tipo para evento de status não lido
type UnreadStatusPayload struct {
	SectorID     int          `json:"sectorId"`
	UnreadStatus map[int]bool `json:"unreadStatus"` // ID do contato -> status de visualização
}

type UnreadStatusEvent struct {
	Type    string              `json:"type"`
	Payload UnreadStatusPayload `json:"payload"`
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

// Nova função para enviar evento de novo contato ou contato atualizado
func SendContactEvent(
	id int,
	sectorID int,
	name string,
	number string,
	avatar string,
	isViewed bool,
	contactStatus string,
	createdAt time.Time,
	updatedAt time.Time,
) {
	payload := ContactPayload{
		ID:            id,
		SectorID:      sectorID,
		Name:          name,
		Number:        number,
		Avatar:        avatar,
		IsViewed:      isViewed,
		ContactStatus: contactStatus,
		CreatedAt:     createdAt,
		UpdatedAt:     updatedAt,
	}
	event := ContactEvent{
		Type:    "contact",
		Payload: payload,
	}
	wsnotify.Manager.Broadcast(event)
}

// Nova função para enviar status de mensagens não lidas
func SendUnreadStatusEvent(
	sectorID int,
	unreadStatus map[int]bool,
) {
	payload := UnreadStatusPayload{
		SectorID:     sectorID,
		UnreadStatus: unreadStatus,
	}
	event := UnreadStatusEvent{
		Type:    "unread_status",
		Payload: payload,
	}
	wsnotify.Manager.Broadcast(event)
}
