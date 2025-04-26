package models

import "time"

type Message struct {
	ID                int
	Conteudo          string
	Tipo              string
	URL               string
	NomeArquivo       string
	MimeType          string
	IDSetor           int
	ContatoID         int64
	DataEnvio         time.Time
	Enviado           bool
	Lido              bool
	WhatsAppMessageID string
	IsOfficial        bool
	CreatedAt         time.Time
}

type MessageRepository interface {
	Save(message *Message) error
	GetByID(id int) (*Message, error)
	GetBySector(sectorID int, limit int) ([]*Message, error)
	GetByContact(sectorID int, contactID string, limit int) ([]*Message, error)
}
