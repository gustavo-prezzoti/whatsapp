package models

import "time"

// Status de mensagem para mostrar barrinhas
const (
	StatusSent     = "sent"     // Uma barra (enviado)
	StatusReceived = "received" // Duas barras (recebido)
	StatusRead     = "read"     // Duas barras azuis (lido)
)

type Message struct {
	ID                int
	Conteudo          string
	Tipo              string
	URL               string
	NomeArquivo       string
	MimeType          string
	IDSetor           int
	ContatoID         int64
	DataEnvio         string
	Enviado           bool
	Lido              bool
	WhatsAppMessageID string
	IsOfficial        bool
	CreatedAt         time.Time
	UserID            *int
	IsAnonymous       bool
	MessageStatus     string // Status da mensagem: sent (uma barra), received (duas barras), read (duas barras azuis)
}

type MessageRepository interface {
	Save(message *Message) error
	GetByID(id int) (*Message, error)
	GetBySector(sectorID int, limit int) ([]*Message, error)
	GetByContact(sectorID int, contactID string, limit int) ([]*Message, error)
	UpdateMessageStatus(messageID int, status string) error
	MarkMessagesAsRead(messageIDs []int) error
}
