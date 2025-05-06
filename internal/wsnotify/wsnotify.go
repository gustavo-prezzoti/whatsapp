package wsnotify

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"whatsapp-bot/internal/models"

	"github.com/gorilla/websocket"
)

type WebSocketClient struct {
	SectorID int
	Conn     *websocket.Conn
}

type WebSocketManager struct {
	clients map[*websocket.Conn]*WebSocketClient
	lock    sync.RWMutex
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func Upgrader() *websocket.Upgrader {
	return &upgrader
}

var Manager = &WebSocketManager{
	clients: make(map[*websocket.Conn]*WebSocketClient),
}

func (m *WebSocketManager) AddClient(conn *websocket.Conn, sectorID int) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.clients[conn] = &WebSocketClient{
		SectorID: sectorID,
		Conn:     conn,
	}
	fmt.Printf("[DEBUG-WS] Cliente adicionado para setor %d. Total de clientes: %d\n", sectorID, len(m.clients))
}

func (m *WebSocketManager) RemoveClient(conn *websocket.Conn) {
	m.lock.Lock()
	defer m.lock.Unlock()
	if client, exists := m.clients[conn]; exists {
		fmt.Printf("[DEBUG-WS] Cliente removido do setor %d\n", client.SectorID)
	}
	delete(m.clients, conn)
}

// Broadcast envia mensagem para todos os clientes conectados
func (m *WebSocketManager) Broadcast(event interface{}) {
	m.lock.RLock()
	defer m.lock.RUnlock()
	fmt.Printf("[DEBUG-WS] Broadcast chamado. Total de clientes: %d\n", len(m.clients))
	for conn := range m.clients {
		conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if err := conn.WriteJSON(event); err != nil {
			fmt.Printf("[DEBUG-WS] Erro ao enviar broadcast: %v\n", err)
			conn.Close()
			go m.RemoveClient(conn)
		}
	}
}

// BroadcastToSector envia mensagem apenas para clientes do setor especificado
func (m *WebSocketManager) BroadcastToSector(event interface{}, sectorID int) {
	m.lock.RLock()
	defer m.lock.RUnlock()
	fmt.Printf("[DEBUG-WS] BroadcastToSector chamado para setor %d. Total de clientes: %d\n", sectorID, len(m.clients))
	for conn, client := range m.clients {
		if client.SectorID == sectorID {
			conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if err := conn.WriteJSON(event); err != nil {
				fmt.Printf("[DEBUG-WS] Erro ao enviar broadcast para setor %d: %v\n", sectorID, err)
				conn.Close()
				go m.RemoveClient(conn)
			}
		}
	}
}

type MessagePayload struct {
	ID            int     `json:"id"`
	ContactID     int     `json:"contactID"`
	SectorID      int     `json:"sectorId"`
	Content       string  `json:"content"`
	MediaType     string  `json:"mediaType"`
	MediaUrl      *string `json:"mediaUrl"`
	FileName      *string `json:"fileName"`
	MimeType      *string `json:"mimeType"`
	SentAt        string  `json:"sentAt"`
	IsSent        bool    `json:"isSent"`
	IsRead        bool    `json:"isRead"`
	MessageStatus string  `json:"messageStatus"`
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
	messageStatus string,
) {
	// Remover 1 hora do timestamp para exibição correta no front-end
	adjustedTime := sentAt.Add(-time.Hour)

	payload := MessagePayload{
		ID:            id,
		ContactID:     contactID,
		SectorID:      sectorID,
		Content:       content,
		MediaType:     mediaType,
		MediaUrl:      mediaUrl,
		FileName:      fileName,
		MimeType:      mimeType,
		SentAt:        adjustedTime.Format(time.RFC3339Nano),
		IsSent:        isSent,
		IsRead:        isRead,
		MessageStatus: messageStatus,
	}
	event := MessageEvent{
		Type:    "message",
		Payload: payload,
	}
	Manager.BroadcastToSector(event, sectorID)
}

// ContactPayload define os dados de um contato para eventos WebSocket
type ContactPayload struct {
	ID            int       `json:"id"`
	SectorID      int       `json:"sectorId"`
	Name          string    `json:"name"`
	Number        string    `json:"number"`
	AvatarUrl     string    `json:"avatarUrl"`
	IsViewed      bool      `json:"isViewed"`
	ContactStatus string    `json:"contactStatus"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
	Order         int       `json:"order"`
}

type ContactEvent struct {
	Type    string         `json:"type"`
	Payload ContactPayload `json:"payload"`
}

// SendContactEvent envia um evento de atualização de contato via WebSocket
func SendContactEvent(
	id int,
	sectorID int,
	name string,
	number string,
	avatarUrl string,
	isViewed bool,
	contactStatus string,
	createdAt time.Time,
	updatedAt time.Time,
	order int,
) {
	payload := ContactPayload{
		ID:            id,
		SectorID:      sectorID,
		Name:          name,
		Number:        number,
		AvatarUrl:     avatarUrl,
		IsViewed:      isViewed,
		ContactStatus: contactStatus,
		CreatedAt:     createdAt,
		UpdatedAt:     updatedAt,
		Order:         order,
	}

	event := ContactEvent{
		Type:    "contact",
		Payload: payload,
	}

	Manager.BroadcastToSector(event, sectorID)
}

// UnreadStatusPayload define os dados de status não lidos para eventos WebSocket
type UnreadStatusPayload struct {
	SectorID     int          `json:"sectorId"`
	UnreadStatus map[int]bool `json:"unreadStatus"`
}

type UnreadStatusEvent struct {
	Type    string              `json:"type"`
	Payload UnreadStatusPayload `json:"payload"`
}

// SendUnreadStatusEvent envia um evento de status de leitura via WebSocket
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
	Manager.BroadcastToSector(event, sectorID)
}

// ContactsListPayload define a estrutura da lista completa de contatos
type ContactsListPayload struct {
	SectorID int              `json:"sectorId"`
	Contacts []ContactPayload `json:"contacts"`
}

type ContactsListEvent struct {
	Type    string              `json:"type"`
	Payload ContactsListPayload `json:"payload"`
}

// SendContactsList envia a lista completa de contatos pelo WebSocket
func SendContactsList(sectorID int, contacts []*models.Contact) {
	// Converter a lista de contatos para o formato do payload
	contactsPayload := make([]ContactPayload, 0, len(contacts))

	for _, contact := range contacts {
		contactsPayload = append(contactsPayload, ContactPayload{
			ID:            contact.ID,
			SectorID:      contact.SectorID,
			Name:          contact.Name,
			Number:        contact.Number,
			AvatarUrl:     contact.AvatarURL,
			IsViewed:      contact.IsViewed,
			ContactStatus: contact.ContactStatus,
			CreatedAt:     contact.CreatedAt,
			UpdatedAt:     contact.UpdatedAt,
			Order:         contact.Order,
		})
	}

	payload := ContactsListPayload{
		SectorID: sectorID,
		Contacts: contactsPayload,
	}

	event := ContactsListEvent{
		Type:    "contacts_list",
		Payload: payload,
	}

	Manager.BroadcastToSector(event, sectorID)
}
