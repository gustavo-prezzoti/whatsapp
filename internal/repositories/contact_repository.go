package repositories

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
	"whatsapp-bot/internal/models"
	"whatsapp-bot/internal/utils"
	"whatsapp-bot/internal/wsnotify"
)

type MySQLContactRepository struct {
	db *sql.DB
}

func NewMySQLContactRepository(db *sql.DB) *MySQLContactRepository {
	return &MySQLContactRepository{db: db}
}

func (r *MySQLContactRepository) Save(contact *models.Contact) error {
	query := `
		INSERT INTO contacts (
			name, number, avatar_url, sector_id, tag_id,
			is_active, email, notes, ai_active, assigned_to,
			priority, contact_status, created_at, updated_at, is_official, is_viewed
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NOW(), NOW(), ?, ?)`

	result, err := r.db.Exec(query,
		contact.Name,
		contact.Number,
		utils.NullString(contact.AvatarURL),
		contact.SectorID,
		utils.NullInt(contact.TagID),
		utils.BoolToInt(contact.IsActive),
		utils.NullString(contact.Email),
		utils.NullString(contact.Notes),
		utils.BoolToInt(contact.AIActive),
		utils.NullInt(contact.AssignedTo),
		contact.Priority,
		contact.ContactStatus,
		utils.BoolToInt(contact.IsOfficial),
		utils.BoolToInt(contact.IsViewed),
	)

	if err != nil {
		return fmt.Errorf("error saving contact: %v", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("error getting last insert id: %v", err)
	}

	contact.ID = int(id)
	return nil
}

func (r *MySQLContactRepository) GetByNumber(sectorID int, number string) (*models.Contact, error) {
	// Normalizar o número usando o mesmo padrão
	normalizedNumber := number
	normalizedNumber = strings.TrimSuffix(normalizedNumber, "@s.whatsapp.net")
	if len(normalizedNumber) == 11 || len(normalizedNumber) == 10 {
		normalizedNumber = "55" + normalizedNumber
	}

	query := `
		SELECT 
			id, name, number, avatar_url, sector_id, tag_id,
			is_active, email, notes, ai_active, assigned_to,
			priority, contact_status, created_at, updated_at, is_official, is_viewed
		FROM contacts 
		WHERE sector_id = ? 
		AND (number = ? OR number LIKE ?)`

	contact := &models.Contact{}
	var avatarURL, email, notes sql.NullString
	var tagID, assignedTo sql.NullInt64

	err := r.db.QueryRow(query, sectorID, normalizedNumber, "%"+normalizedNumber+"%").Scan(
		&contact.ID,
		&contact.Name,
		&contact.Number,
		&avatarURL,
		&contact.SectorID,
		&tagID,
		&contact.IsActive,
		&email,
		&notes,
		&contact.AIActive,
		&assignedTo,
		&contact.Priority,
		&contact.ContactStatus,
		&contact.CreatedAt,
		&contact.UpdatedAt,
		&contact.IsOfficial,
		&contact.IsViewed,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("error getting contact: %v", err)
	}

	contact.AvatarURL = avatarURL.String
	contact.Email = email.String
	contact.Notes = notes.String
	if tagID.Valid {
		contact.TagID = int(tagID.Int64)
	}
	if assignedTo.Valid {
		contact.AssignedTo = int(assignedTo.Int64)
	}

	return contact, nil
}

func (r *MySQLContactRepository) GetBySector(sectorID int) ([]*models.Contact, error) {
	query := `
		SELECT 
			id, name, number, avatar_url, sector_id, tag_id, 
			is_active, email, notes, ai_active, assigned_to,
			priority, contact_status, created_at, updated_at, is_official, is_viewed, COALESCE(` + "`order`" + `, id) AS contact_order
		FROM contacts 
		WHERE sector_id = ? 
		ORDER BY contact_order ASC`

	rows, err := r.db.Query(query, sectorID)
	if err != nil {
		return nil, fmt.Errorf("error querying contacts: %v", err)
	}
	defer rows.Close()

	var contacts []*models.Contact

	for rows.Next() {
		contact := &models.Contact{}
		var avatarURL, email, notes sql.NullString
		var tagID, assignedTo sql.NullInt64

		err := rows.Scan(
			&contact.ID,
			&contact.Name,
			&contact.Number,
			&avatarURL,
			&contact.SectorID,
			&tagID,
			&contact.IsActive,
			&email,
			&notes,
			&contact.AIActive,
			&assignedTo,
			&contact.Priority,
			&contact.ContactStatus,
			&contact.CreatedAt,
			&contact.UpdatedAt,
			&contact.IsOfficial,
			&contact.IsViewed,
			&contact.Order,
		)

		if err != nil {
			return nil, fmt.Errorf("error scanning contact: %v", err)
		}

		contact.AvatarURL = avatarURL.String
		contact.Email = email.String
		contact.Notes = notes.String

		if tagID.Valid {
			contact.TagID = int(tagID.Int64)
		}

		if assignedTo.Valid {
			contact.AssignedTo = int(assignedTo.Int64)
		}

		contacts = append(contacts, contact)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating contacts: %v", err)
	}

	return contacts, nil
}

func (r *MySQLContactRepository) Update(contact *models.Contact) error {
	query := `
		UPDATE contacts 
		SET name = ?,
			number = ?,
			avatar_url = ?,
			sector_id = ?,
			tag_id = ?,
			is_active = ?,
			email = ?,
			notes = ?,
			ai_active = ?,
			assigned_to = ?,
			priority = ?,
			contact_status = ?,
			updated_at = NOW(),
			is_official = ?,
			is_viewed = ?
		WHERE id = ?`

	_, err := r.db.Exec(query,
		contact.Name,
		contact.Number,
		utils.NullString(contact.AvatarURL),
		contact.SectorID,
		utils.NullInt(contact.TagID),
		utils.BoolToInt(contact.IsActive),
		utils.NullString(contact.Email),
		utils.NullString(contact.Notes),
		utils.BoolToInt(contact.AIActive),
		utils.NullInt(contact.AssignedTo),
		contact.Priority,
		contact.ContactStatus,
		utils.BoolToInt(contact.IsOfficial),
		utils.BoolToInt(contact.IsViewed),
		contact.ID,
	)

	if err != nil {
		return fmt.Errorf("error updating contact: %v", err)
	}

	return nil
}

func (r *MySQLContactRepository) CreateIfNotExists(sectorID int, number string) (*models.Contact, error) {
	normalizedNumber := number
	normalizedNumber = strings.TrimSuffix(normalizedNumber, "@s.whatsapp.net")
	if len(normalizedNumber) == 11 || len(normalizedNumber) == 10 {
		normalizedNumber = "55" + normalizedNumber
	}

	contact, err := r.GetByNumber(sectorID, normalizedNumber)
	if err != nil {
		return nil, err
	}

	if contact != nil {
		// Verificar se o contato já existe em algum card
		var cardExists bool
		err = r.db.QueryRow(`
			SELECT EXISTS(
				SELECT 1 FROM cards 
				WHERE contact_id = ? AND sector_id = ?
			)`, contact.ID, sectorID).Scan(&cardExists)
		if err != nil {
			return nil, fmt.Errorf("error checking if card exists: %v", err)
		}

		if cardExists {
			return contact, nil
		}
	}

	tx, err := r.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("error starting transaction: %v", err)
	}
	defer tx.Rollback()

	var columnExists bool
	var columnID int
	err = tx.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM colunas 
			WHERE sector_id = ? AND name = 'Primeiro Atendimento'
		)`, sectorID).Scan(&columnExists)
	if err != nil {
		return nil, fmt.Errorf("error checking if column exists: %v", err)
	}

	var nextPosition int
	if !columnExists {
		result, err := tx.Exec(`
			INSERT INTO colunas (name, sector_id, position)
			VALUES ('Primeiro Atendimento', ?, 0)`,
			sectorID)
		if err != nil {
			return nil, fmt.Errorf("error creating column: %v", err)
		}

		id, err := result.LastInsertId()
		if err != nil {
			return nil, fmt.Errorf("error getting column id: %v", err)
		}
		columnID = int(id)
		nextPosition = 0
	} else {
		err = tx.QueryRow(`
			SELECT id FROM colunas 
			WHERE sector_id = ? AND name = 'Primeiro Atendimento'`,
			sectorID).Scan(&columnID)
		if err != nil {
			return nil, fmt.Errorf("error getting column id: %v", err)
		}

		// Buscar a próxima posição disponível
		err = tx.QueryRow(`
			SELECT COALESCE(MAX(position) + 1, 0)
			FROM cards
			WHERE column_id = ? AND sector_id = ?`,
			columnID, sectorID).Scan(&nextPosition)
		if err != nil {
			return nil, fmt.Errorf("error getting next position: %v", err)
		}
	}

	var contactID int64
	if contact == nil {
		newContact := &models.Contact{
			Name:          normalizedNumber,
			Number:        normalizedNumber,
			SectorID:      sectorID,
			IsActive:      true,
			Priority:      "low",
			ContactStatus: "Novo",
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
			IsViewed:      false,
		}

		utils.LogInfo("Criando novo contato: %s (%s) para setor %d", normalizedNumber, normalizedNumber, sectorID)

		result, err := tx.Exec(`
			INSERT INTO contacts (
				name, number, avatar_url, sector_id, tag_id,
				is_active, email, notes, ai_active, assigned_to,
				priority, contact_status, created_at, updated_at, is_official, is_viewed, `+"`order`"+`
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NOW(), NOW(), ?, ?, 1)`,
			newContact.Name,
			newContact.Number,
			utils.NullString(newContact.AvatarURL),
			newContact.SectorID,
			utils.NullInt(newContact.TagID),
			utils.BoolToInt(newContact.IsActive),
			utils.NullString(newContact.Email),
			utils.NullString(newContact.Notes),
			utils.BoolToInt(newContact.AIActive),
			utils.NullInt(newContact.AssignedTo),
			newContact.Priority,
			newContact.ContactStatus,
			utils.BoolToInt(newContact.IsOfficial),
			utils.BoolToInt(newContact.IsViewed),
		)

		if err != nil {
			return nil, fmt.Errorf("error saving contact: %v", err)
		}

		contactID, err = result.LastInsertId()
		if err != nil {
			return nil, fmt.Errorf("error getting contact id: %v", err)
		}
		newContact.ID = int(contactID)
		contact = newContact
	} else {
		contactID = int64(contact.ID)
	}

	_, err = tx.Exec(`
		INSERT INTO cards (contact_id, column_id, sector_id, position, created_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		contactID, columnID, sectorID, nextPosition)
	if err != nil {
		return nil, fmt.Errorf("error creating card: %v", err)
	}

	// Incrementar a ordem de todos os outros contatos
	_, err = tx.Exec(`
		UPDATE contacts 
		SET `+"`order`"+` = `+"`order`"+` + 1 
		WHERE sector_id = ? AND id != LAST_INSERT_ID()`,
		sectorID)

	if err != nil {
		return nil, fmt.Errorf("erro ao atualizar ordenação: %v", err)
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("error committing transaction: %v", err)
	}

	return contact, nil
}

func (r *MySQLContactRepository) SetViewed(sectorID int, number string) error {
	query := `
		UPDATE contacts 
		SET is_viewed = 1,
			updated_at = NOW()
		WHERE sector_id = ? AND number = ?`

	result, err := r.db.Exec(query, sectorID, number)
	if err != nil {
		return fmt.Errorf("error updating contact viewed status: %v", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("error getting rows affected: %v", err)
	}

	if rows == 0 {
		return fmt.Errorf("contact not found")
	}

	// Enviar lista completa de contatos para atualizar a ordenação no frontend
	go r.sendContactsList(sectorID)

	return nil
}

func (r *MySQLContactRepository) SetViewedByID(sectorID int, contactID int) error {
	query := `
		UPDATE contacts 
		SET is_viewed = 1,
			updated_at = NOW()
		WHERE sector_id = ? AND id = ?`

	result, err := r.db.Exec(query, sectorID, contactID)
	if err != nil {
		return fmt.Errorf("error updating contact viewed status: %v", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("error getting rows affected: %v", err)
	}

	if rows == 0 {
		return fmt.Errorf("contact not found")
	}

	// Buscar dados atualizados do contato para enviar via WebSocket
	contact, err := r.getContactByID(sectorID, contactID)
	if err == nil && contact != nil {
		wsnotify.SendContactEvent(
			contact.ID,
			contact.SectorID,
			contact.Name,
			contact.Number,
			contact.AvatarURL,
			contact.IsViewed,
			contact.ContactStatus,
			contact.CreatedAt,
			contact.UpdatedAt,
			contact.Order,
		)
	}

	// Enviar lista completa de contatos para atualizar a ordenação no frontend
	go r.sendContactsList(sectorID)

	return nil
}

// Função auxiliar para buscar contato por ID
func (r *MySQLContactRepository) getContactByID(sectorID int, contactID int) (*models.Contact, error) {
	query := `
		SELECT 
			id, name, number, avatar_url, sector_id, tag_id, 
			is_active, email, notes, ai_active, assigned_to,
			priority, contact_status, created_at, updated_at, is_official, is_viewed, COALESCE(` + "`order`" + `, id) AS contact_order
		FROM contacts 
		WHERE sector_id = ? AND id = ?`

	contact := &models.Contact{}
	var avatarURL, email, notes sql.NullString
	var tagID, assignedTo sql.NullInt64

	err := r.db.QueryRow(query, sectorID, contactID).Scan(
		&contact.ID,
		&contact.Name,
		&contact.Number,
		&avatarURL,
		&contact.SectorID,
		&tagID,
		&contact.IsActive,
		&email,
		&notes,
		&contact.AIActive,
		&assignedTo,
		&contact.Priority,
		&contact.ContactStatus,
		&contact.CreatedAt,
		&contact.UpdatedAt,
		&contact.IsOfficial,
		&contact.IsViewed,
		&contact.Order,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("error getting contact: %v", err)
	}

	contact.AvatarURL = avatarURL.String
	contact.Email = email.String
	contact.Notes = notes.String

	if tagID.Valid {
		contact.TagID = int(tagID.Int64)
	}

	if assignedTo.Valid {
		contact.AssignedTo = int(assignedTo.Int64)
	}

	return contact, nil
}

func (r *MySQLContactRepository) GetViewedStatus(sectorID int) (map[int]bool, error) {
	// Verifica se o setor existe
	var exists bool
	err := r.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM setores WHERE id = ?)`, sectorID).Scan(&exists)
	if err != nil {
		return nil, fmt.Errorf("erro ao verificar setor: %v", err)
	}
	if !exists {
		return nil, fmt.Errorf("setor não encontrado")
	}

	// Busca todos os contatos do setor com seus status
	rows, err := r.db.Query(`
		SELECT id, is_viewed 
		FROM contacts 
		WHERE sector_id = ?`,
		sectorID)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar contatos: %v", err)
	}
	defer rows.Close()

	contactStatus := make(map[int]bool)
	for rows.Next() {
		var id int
		var isViewed bool
		if err := rows.Scan(&id, &isViewed); err != nil {
			return nil, fmt.Errorf("erro ao ler status do contato: %v", err)
		}
		contactStatus[id] = isViewed
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("erro ao iterar sobre os contatos: %v", err)
	}

	return contactStatus, nil
}

func (r *MySQLContactRepository) SetUnviewed(sectorID int, number string) error {
	// Normalizar o número usando o mesmo padrão do CreateIfNotExists
	normalizedNumber := number
	normalizedNumber = strings.TrimSuffix(normalizedNumber, "@s.whatsapp.net")
	if len(normalizedNumber) == 11 || len(normalizedNumber) == 10 {
		normalizedNumber = "55" + normalizedNumber
	}

	query := `
		UPDATE contacts 
		SET is_viewed = 0,
			updated_at = NOW()
		WHERE sector_id = ? 
		AND (number = ? OR number LIKE ?)`

	result, err := r.db.Exec(query, sectorID, normalizedNumber, "%"+normalizedNumber+"%")
	if err != nil {
		return fmt.Errorf("error updating contact unviewed status: %v", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("error getting rows affected: %v", err)
	}

	if rows == 0 {
		return fmt.Errorf("contact not found")
	}

	// Buscar o contato atualizado para enviar via WebSocket
	contact, err := r.GetByNumber(sectorID, normalizedNumber)
	if err == nil && contact != nil {
		wsnotify.SendContactEvent(
			contact.ID,
			contact.SectorID,
			contact.Name,
			contact.Number,
			contact.AvatarURL,
			contact.IsViewed,
			contact.ContactStatus,
			contact.CreatedAt,
			contact.UpdatedAt,
			contact.Order,
		)
	}

	// Enviar lista completa de contatos para atualizar a ordenação no frontend
	go r.sendContactsList(sectorID)

	return nil
}

// SendUnreadStatusUpdate busca e envia o status de leitura dos contatos via WebSocket
// sem modificar nenhum dado no banco. Use este método separadamente quando precisar
// enviar atualizações de status sem risco de criar loops.
func (r *MySQLContactRepository) SendUnreadStatusUpdate(sectorID int) error {
	viewedStatus, err := r.GetViewedStatus(sectorID)
	if err != nil {
		return fmt.Errorf("erro ao obter status de leitura: %v", err)
	}

	wsnotify.SendUnreadStatusEvent(sectorID, viewedStatus)
	return nil
}

// UpdateContactOrder move um contato para o topo da lista (ordem 1) e incrementa a ordem dos outros contatos
func (r *MySQLContactRepository) UpdateContactOrder(sectorID int, contactID int) error {
	// Iniciar uma transação para garantir consistência
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("erro ao iniciar transação: %v", err)
	}
	defer tx.Rollback()

	// 1. Verificar se já existe ordenação, se não, inicializar todos com ordem = ID
	var count int
	err = tx.QueryRow("SELECT COUNT(*) FROM contacts WHERE sector_id = ? AND `order` > 0", sectorID).Scan(&count)
	if err != nil {
		return fmt.Errorf("erro ao verificar contatos: %v", err)
	}

	// Se não existirem contatos com ordenação, inicializar todos
	if count == 0 {
		_, err = tx.Exec(`
			UPDATE contacts 
			SET `+"`order`"+` = id 
			WHERE sector_id = ?`,
			sectorID)

		if err != nil {
			return fmt.Errorf("erro ao inicializar ordenação: %v", err)
		}
	}

	// 2. Incrementar a ordem de todos os contatos que estão acima do alvo
	_, err = tx.Exec(`
		UPDATE contacts 
		SET `+"`order`"+` = `+"`order`"+` + 1 
		WHERE sector_id = ? AND `+"`order`"+` < (
			SELECT `+"`order`"+` FROM (
				SELECT `+"`order`"+` FROM contacts WHERE id = ? AND sector_id = ?
			) AS temp
		)`,
		sectorID, contactID, sectorID)

	if err != nil {
		return fmt.Errorf("erro ao incrementar ordenação: %v", err)
	}

	// 3. Mover o contato alvo para o topo (ordem = 1)
	_, err = tx.Exec(`
		UPDATE contacts 
		SET `+"`order`"+` = 1, updated_at = NOW()
		WHERE id = ? AND sector_id = ?`,
		contactID, sectorID)

	if err != nil {
		return fmt.Errorf("erro ao atualizar ordem do contato: %v", err)
	}

	// Commit da transação
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("erro ao finalizar transação: %v", err)
	}

	// Buscar contato atualizado para enviar via WebSocket
	contact, err := r.getContactByID(sectorID, contactID)
	if err == nil && contact != nil {
		wsnotify.SendContactEvent(
			contact.ID,
			contact.SectorID,
			contact.Name,
			contact.Number,
			contact.AvatarURL,
			contact.IsViewed,
			contact.ContactStatus,
			contact.CreatedAt,
			contact.UpdatedAt,
			contact.Order,
		)
	}

	// Enviar uma única atualização de status de não lidos após a atualização
	go func() {
		time.Sleep(500 * time.Millisecond)
		err := r.SendUnreadStatusUpdate(sectorID)
		if err != nil {
			utils.LogError("Erro ao enviar atualização de status após UpdateContactOrder: %v", err)
		}

		// Buscar e enviar lista completa de contatos
		r.sendContactsList(sectorID)
	}()

	return nil
}

// Novo método para enviar a lista completa de contatos via WebSocket
func (r *MySQLContactRepository) sendContactsList(sectorID int) {
	contacts, err := r.GetBySector(sectorID)
	if err != nil {
		utils.LogError("Erro ao buscar lista de contatos para enviar via WebSocket: %v", err)
		return
	}

	wsnotify.SendContactsList(sectorID, contacts)
}

func nullInt(i int) sql.NullInt64 {
	if i == 0 {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(i), Valid: true}
}
