package repositories

import (
	"database/sql"
	"fmt"
	"strings"
	"whatsapp-bot/internal/models"
	"whatsapp-bot/internal/utils"
)

type MySQLMessageRepository struct {
	db *sql.DB
}

func NewMySQLMessageRepository(db *sql.DB) *MySQLMessageRepository {
	return &MySQLMessageRepository{db: db}
}

func (r *MySQLMessageRepository) Save(message *models.Message) error {
	query := `
		INSERT INTO messages (
			conteudo, tipo, url, nome_arquivo, mime_type, 
			id_setor, contato_id, data_envio, enviado, lido, 
			WhatsAppMessageId, is_official
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	result, err := r.db.Exec(query,
		message.Conteudo,
		message.Tipo,
		utils.NullString(message.URL),
		utils.NullString(message.NomeArquivo),
		utils.NullString(message.MimeType),
		message.IDSetor,
		message.ContatoID,
		message.DataEnvio,
		utils.BoolToInt(message.Enviado),
		utils.BoolToInt(message.Lido),
		utils.NullString(message.WhatsAppMessageID),
		utils.BoolToInt(message.IsOfficial),
	)

	if err != nil {
		return fmt.Errorf("error saving message: %v", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("error getting last insert id: %v", err)
	}

	message.ID = int(id)
	return nil
}

func (r *MySQLMessageRepository) GetByID(id int) (*models.Message, error) {
	query := `
		SELECT 
			id, conteudo, tipo, url, nome_arquivo, mime_type,
			id_setor, contato_id, data_envio, enviado, lido,
			WhatsAppMessageId, is_official, created_at
		FROM messages 
		WHERE id = ?`

	message := &models.Message{}
	var url, nomeArquivo, mimeType, whatsappMessageID sql.NullString

	err := r.db.QueryRow(query, id).Scan(
		&message.ID,
		&message.Conteudo,
		&message.Tipo,
		&url,
		&nomeArquivo,
		&mimeType,
		&message.IDSetor,
		&message.ContatoID,
		&message.DataEnvio,
		&message.Enviado,
		&message.Lido,
		&whatsappMessageID,
		&message.IsOfficial,
		&message.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("error getting message: %v", err)
	}

	message.URL = url.String
	message.NomeArquivo = nomeArquivo.String
	message.MimeType = mimeType.String
	message.WhatsAppMessageID = whatsappMessageID.String

	return message, nil
}

func (r *MySQLMessageRepository) GetBySector(sectorID int, limit int) ([]*models.Message, error) {
	query := `
		SELECT 
			id, conteudo, tipo, url, nome_arquivo, mime_type,
			id_setor, contato_id, data_envio, enviado, lido,
			WhatsAppMessageId, is_official, created_at
		FROM messages 
		WHERE id_setor = ?
		ORDER BY data_envio DESC
		LIMIT ?`

	return r.fetchMessages(query, sectorID, limit)
}

func (r *MySQLMessageRepository) GetByContact(sectorID int, contactID string, limit int) ([]*models.Message, error) {
	query := `
		SELECT 
			id, conteudo, tipo, url, nome_arquivo, mime_type,
			id_setor, contato_id, data_envio, enviado, lido,
			WhatsAppMessageId, is_official, created_at
		FROM messages 
		WHERE id_setor = ? AND contato_id = ?
		ORDER BY data_envio DESC
		LIMIT ?`

	return r.fetchMessages(query, sectorID, contactID, limit)
}

func (r *MySQLMessageRepository) fetchMessages(query string, args ...interface{}) ([]*models.Message, error) {
	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("error querying messages: %v", err)
	}
	defer rows.Close()

	var messages []*models.Message

	for rows.Next() {
		message := &models.Message{}
		var url, nomeArquivo, mimeType, whatsappMessageID sql.NullString

		err := rows.Scan(
			&message.ID,
			&message.Conteudo,
			&message.Tipo,
			&url,
			&nomeArquivo,
			&mimeType,
			&message.IDSetor,
			&message.ContatoID,
			&message.DataEnvio,
			&message.Enviado,
			&message.Lido,
			&whatsappMessageID,
			&message.IsOfficial,
			&message.CreatedAt,
		)

		if err != nil {
			return nil, fmt.Errorf("error scanning message: %v", err)
		}

		message.URL = url.String
		message.NomeArquivo = nomeArquivo.String
		message.MimeType = mimeType.String
		message.WhatsAppMessageID = whatsappMessageID.String

		messages = append(messages, message)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating messages: %v", err)
	}

	return messages, nil
}

func (r *MySQLMessageRepository) UpdateMessageStatus(messageID int, status string) error {
	return nil
}

func (r *MySQLMessageRepository) MarkMessagesAsRead(messageIDs []int) error {
	if len(messageIDs) == 0 {
		return nil
	}
	query := "UPDATE messages SET lido = 1 WHERE id IN (?" + strings.Repeat(",?", len(messageIDs)-1) + ")"
	args := make([]interface{}, len(messageIDs))
	for i, id := range messageIDs {
		args[i] = id
	}
	_, err := r.db.Exec(query, args...)
	return err
}
