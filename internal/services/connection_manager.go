package services

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"encoding/base64"
	"whatsapp-bot/config"
	"whatsapp-bot/internal/repositories"
	"whatsapp-bot/internal/utils"

	"github.com/skip2/go-qrcode"
)

type ConnectionManager struct {
	db                *sql.DB
	mutex             sync.RWMutex
	connections       map[int]*WhatsAppService
	config            *config.Config
	messageRepository *repositories.MySQLMessageRepository
	contactRepository *repositories.MySQLContactRepository
}

func NewConnectionManager(db *sql.DB, config *config.Config) *ConnectionManager {
	return &ConnectionManager{
		connections:       make(map[int]*WhatsAppService),
		db:                db,
		config:            config,
		messageRepository: repositories.NewMySQLMessageRepository(db),
		contactRepository: repositories.NewMySQLContactRepository(db),
	}
}

func (cm *ConnectionManager) GetConnection(sectorID int) (*WhatsAppService, error) {
	cm.mutex.RLock()
	if service, exists := cm.connections[sectorID]; exists {
		cm.mutex.RUnlock()
		return service, nil
	}
	cm.mutex.RUnlock()

	utils.LogDebug("Verificando setor %d", sectorID)
	var isOfficial bool
	err := cm.db.QueryRow("SELECT is_official FROM setores WHERE id = ?", sectorID).Scan(&isOfficial)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("setor não encontrado")
		}
		return nil, fmt.Errorf("erro ao verificar setor: %v", err)
	}

	if isOfficial {
		return nil, fmt.Errorf("este setor está configurado para usar WhatsApp oficial")
	}

	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	if service, exists := cm.connections[sectorID]; exists {
		return service, nil
	}

	utils.LogDebug("Criando nova conexão do WhatsApp para setor %d", sectorID)
	service := NewWhatsAppService(cm.config, cm, cm.messageRepository, cm.contactRepository)
	service.SetSectorAndManager(sectorID, cm)

	_, err = cm.db.Exec(`
		INSERT INTO whatsapp_connections 
		(setor_id, status, created_at, updated_at) 
		VALUES (?, 'disconnected', NOW(), NOW())
		ON DUPLICATE KEY UPDATE 
		status = 'disconnected',
		updated_at = NOW()`,
		sectorID)
	if err != nil {
		utils.LogError("Erro ao atualizar status da conexão para setor %d: %v", sectorID, err)
		return nil, fmt.Errorf("erro ao atualizar status da conexão: %v", err)
	}

	utils.LogDebug("Iniciando conexão para setor %d", sectorID)
	err = service.Connect()
	if err != nil {
		cm.updateConnectionStatus(sectorID, "disconnected", err.Error())
		utils.LogError("Erro ao conectar WhatsApp para setor %d: %v", sectorID, err)
		return nil, fmt.Errorf("erro ao conectar WhatsApp: %v", err)
	}

	cm.connections[sectorID] = service
	utils.LogInfo("Conexão criada com sucesso para setor %d", sectorID)
	return service, nil
}

func (cm *ConnectionManager) updateConnectionStatus(sectorID int, status string, errorMsg string) error {
	_, err := cm.db.Exec(`
		UPDATE whatsapp_connections 
		SET status = ?, 
			updated_at = NOW(),
			last_error = ?
		WHERE setor_id = ?`,
		status, errorMsg, sectorID)
	return err
}

func (cm *ConnectionManager) UpdateQRCode(sectorID int, qrcodeText string) error {
	utils.LogDebug("Convertendo QR code para base64 para setor %d", sectorID)

	// Gerar QR code em PNG
	qr, err := qrcode.Encode(qrcodeText, qrcode.Medium, 256)
	if err != nil {
		utils.LogError("Erro ao gerar QR code em PNG para setor %d: %v", sectorID, err)
		return fmt.Errorf("erro ao gerar QR code: %v", err)
	}

	// Converter para base64
	qrcodeBase64 := "data:image/png;base64," + base64.StdEncoding.EncodeToString(qr)

	utils.LogDebug("Atualizando QR code no banco para setor %d", sectorID)
	_, err = cm.db.Exec(`
		UPDATE whatsapp_connections 
		SET qrcode_base64 = ?,
			last_qrcode_generated_at = NOW(),
			status = 'connecting',
			updated_at = NOW()
		WHERE setor_id = ?`,
		qrcodeBase64, sectorID)

	if err != nil {
		utils.LogError("Erro ao atualizar QR code no banco para setor %d: %v", sectorID, err)
		return err
	}

	utils.LogInfo("QR code atualizado com sucesso para setor %d", sectorID)
	return nil
}

func (cm *ConnectionManager) SetConnected(sectorID int) error {
	_, err := cm.db.Exec(`
		UPDATE whatsapp_connections 
		SET status = 'connected',
			last_connected_at = NOW(),
			updated_at = NOW()
		WHERE setor_id = ?`,
		sectorID)
	return err
}

func (cm *ConnectionManager) SetDisconnected(sectorID int) error {
	_, err := cm.db.Exec(`
		UPDATE whatsapp_connections 
		SET status = 'disconnected',
			last_disconnected_at = NOW(),
			updated_at = NOW()
		WHERE setor_id = ?`,
		sectorID)
	return err
}

func (cm *ConnectionManager) GetConnectionStatus(sectorID int) (string, error) {
	var status string
	err := cm.db.QueryRow(`
		SELECT status 
		FROM whatsapp_connections 
		WHERE setor_id = ?`,
		sectorID).Scan(&status)
	if err == sql.ErrNoRows {
		return "not_found", nil
	}
	if err != nil {
		return "", err
	}
	return status, nil
}

func (cm *ConnectionManager) GetQRCode(sectorID int) (string, error) {
	utils.LogDebug("Iniciando GetQRCode para setor %d", sectorID)

	// Verificar status atual no banco
	status, err := cm.GetConnectionStatus(sectorID)
	if err != nil {
		utils.LogError("Erro ao verificar status da conexão: %v", err)
		return "", fmt.Errorf("erro ao verificar status da conexão: %v", err)
	}

	// Se estiver conectado, verificar se a conexão ainda é válida
	if status == "connected" {
		if service, exists := cm.connections[sectorID]; exists && service != nil {
			if service.IsConnected() {
				utils.LogInfo("WhatsApp já está conectado para setor %d", sectorID)
				return "", fmt.Errorf("este whatsapp já está conectado. não é necessário escanear QR code")
			}
		}
	}

	// Remover conexão antiga se existir
	if service, exists := cm.connections[sectorID]; exists {
		utils.LogDebug("Removendo conexão antiga para setor %d", sectorID)
		if service != nil {
			if service.client != nil {
				service.client.Disconnect()
			}
			service.SetConnected(false)
		}
		delete(cm.connections, sectorID)
	}

	// Atualizar status para disconnected e limpar QR code antigo
	_, err = cm.db.Exec(`
		UPDATE whatsapp_connections 
		SET status = 'disconnected',
			qrcode_base64 = NULL,
			last_disconnected_at = NOW(),
			updated_at = NOW()
		WHERE setor_id = ?`,
		sectorID)
	if err != nil {
		utils.LogWarning("Erro ao atualizar status para disconnected: %v", err)
	}

	// Criar nova conexão
	utils.LogDebug("Criando nova conexão para setor %d", sectorID)
	_, err = cm.GetConnection(sectorID)
	if err != nil {
		utils.LogError("Erro ao criar nova conexão para setor %d: %v", sectorID, err)
		return "", fmt.Errorf("erro ao gerar novo QR code: %v", err)
	}

	// Aguardar o QR code ser gerado com timeout mais curto
	utils.LogDebug("Aguardando geração do QR code para setor %d", sectorID)
	maxAttempts := 10
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		utils.LogDebug("Tentativa %d/%d de buscar QR code para setor %d", attempt, maxAttempts, sectorID)

		time.Sleep(1 * time.Second)

		// Verificar se não conectou durante a espera
		currentStatus, _ := cm.GetConnectionStatus(sectorID)
		if currentStatus == "connected" {
			if service, exists := cm.connections[sectorID]; exists && service != nil && service.IsConnected() {
				return "", fmt.Errorf("whatsapp conectado com sucesso durante a geração do QR code")
			}
		}

		var qrcode sql.NullString
		var lastGenerated sql.NullTime

		err = cm.db.QueryRow(`
			SELECT qrcode_base64, last_qrcode_generated_at 
			FROM whatsapp_connections 
			WHERE setor_id = ? AND qrcode_base64 IS NOT NULL`,
			sectorID).Scan(&qrcode, &lastGenerated)

		if err == nil && qrcode.Valid && lastGenerated.Valid {
			// Verificar se o QR code foi gerado nos últimos 30 segundos
			if time.Since(lastGenerated.Time) <= 30*time.Second {
				utils.LogInfo("QR code gerado com sucesso para setor %d na tentativa %d", sectorID, attempt)
				return qrcode.String, nil
			} else {
				utils.LogDebug("QR code expirado, gerando novo para setor %d", sectorID)
				// Limpar QR code antigo e continuar o loop
				_, err = cm.db.Exec(`
					UPDATE whatsapp_connections 
					SET qrcode_base64 = NULL,
						updated_at = NOW()
					WHERE setor_id = ?`,
					sectorID)
				if err != nil {
					utils.LogWarning("Erro ao limpar QR code antigo: %v", err)
				}
				continue
			}
		}

		utils.LogDebug("QR code ainda não disponível na tentativa %d para setor %d", attempt, sectorID)
	}

	utils.LogError("Tempo limite excedido aguardando QR code para setor %d", sectorID)
	return "", fmt.Errorf("não foi possível gerar o QR code. por favor, tente novamente")
}

func (cm *ConnectionManager) SendImage(sectorID int, recipient string, imagePath string, caption string) error {
	service, err := cm.GetConnection(sectorID)
	if err != nil {
		return fmt.Errorf("erro ao obter conexão: %v", err)
	}

	imageBytes, err := os.ReadFile(imagePath)
	if err != nil {
		return fmt.Errorf("erro ao ler arquivo de imagem: %v", err)
	}

	return service.SendImage(sectorID, recipient, imageBytes, caption)
}

func (cm *ConnectionManager) SendAudio(sectorID int, recipient string, audioPath string) error {
	service, err := cm.GetConnection(sectorID)
	if err != nil {
		return fmt.Errorf("erro ao obter conexão: %v", err)
	}

	audioBytes, err := os.ReadFile(audioPath)
	if err != nil {
		return fmt.Errorf("erro ao ler arquivo de áudio: %v", err)
	}

	return service.SendAudio(sectorID, recipient, audioBytes)
}

func (cm *ConnectionManager) SendDocument(sectorID int, recipient string, filePath string) error {
	service, err := cm.GetConnection(sectorID)
	if err != nil {
		return fmt.Errorf("erro ao obter conexão: %v", err)
	}

	fileBytes, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("erro ao ler arquivo: %v", err)
	}

	fileName := filepath.Base(filePath)
	return service.SendDocument(sectorID, recipient, fileBytes, fileName)
}

func (cm *ConnectionManager) SendTyping(sectorID int, recipient string, duration int) error {
	service, err := cm.GetConnection(sectorID)
	if err != nil {
		return fmt.Errorf("erro ao obter conexão: %v", err)
	}

	return service.SendTyping(recipient, duration)
}

func (cm *ConnectionManager) SaveWhatsAppSession(sectorID int, sessionData []byte) error {
	utils.LogDebug("Salvando referência de sessão do WhatsApp no banco para setor %d", sectorID)

	// Verificar quais colunas existem na tabela
	var sessionFilePathExists bool
	var sessionDataExists bool

	// Verificar se a coluna session_file_path existe
	err := cm.db.QueryRow(`
		SELECT COUNT(*) > 0 
		FROM information_schema.COLUMNS 
		WHERE TABLE_SCHEMA = DATABASE() 
		AND TABLE_NAME = 'whatsapp_connections' 
		AND COLUMN_NAME = 'session_file_path'`).Scan(&sessionFilePathExists)

	if err != nil {
		utils.LogError("Erro ao verificar coluna session_file_path: %v", err)
	}

	// Verificar se a coluna session_data existe
	err = cm.db.QueryRow(`
		SELECT COUNT(*) > 0 
		FROM information_schema.COLUMNS 
		WHERE TABLE_SCHEMA = DATABASE() 
		AND TABLE_NAME = 'whatsapp_connections' 
		AND COLUMN_NAME = 'session_data'`).Scan(&sessionDataExists)

	if err != nil {
		utils.LogError("Erro ao verificar coluna session_data: %v", err)
	}

	// Verificar se a tabela tem a coluna session_updated_at
	var sessionUpdatedAtExists bool
	err = cm.db.QueryRow(`
		SELECT COUNT(*) > 0 
		FROM information_schema.COLUMNS 
		WHERE TABLE_SCHEMA = DATABASE() 
		AND TABLE_NAME = 'whatsapp_connections' 
		AND COLUMN_NAME = 'session_updated_at'`).Scan(&sessionUpdatedAtExists)

	if err != nil {
		utils.LogError("Erro ao verificar coluna session_updated_at: %v", err)
	}

	// Priorizar usar session_file_path se existir, senão usar session_data
	var sqlQuery string
	var sessionFilePath = string(sessionData)

	if sessionFilePathExists {
		if sessionUpdatedAtExists {
			sqlQuery = `
				UPDATE whatsapp_connections 
				SET session_file_path = ?,
					session_updated_at = NOW(),
					updated_at = NOW()
				WHERE setor_id = ?`
		} else {
			sqlQuery = `
				UPDATE whatsapp_connections 
				SET session_file_path = ?,
					updated_at = NOW()
				WHERE setor_id = ?`
		}
		_, err = cm.db.Exec(sqlQuery, sessionFilePath, sectorID)
	} else if sessionDataExists {
		// Usar coluna session_data se session_file_path não existir
		utils.LogInfo("Usando coluna session_data pois session_file_path não existe")
		if sessionUpdatedAtExists {
			sqlQuery = `
				UPDATE whatsapp_connections 
				SET session_data = ?,
					session_updated_at = NOW(),
					updated_at = NOW()
				WHERE setor_id = ?`
		} else {
			sqlQuery = `
				UPDATE whatsapp_connections 
				SET session_data = ?,
					updated_at = NOW()
				WHERE setor_id = ?`
		}
		_, err = cm.db.Exec(sqlQuery, sessionFilePath, sectorID)
	} else {
		return fmt.Errorf("nenhuma coluna de sessão válida encontrada na tabela whatsapp_connections")
	}

	if err != nil {
		utils.LogError("Erro ao salvar referência de sessão no banco para setor %d: %v", sectorID, err)
		return err
	}

	utils.LogInfo("Referência de sessão salva com sucesso no banco para setor %d", sectorID)
	return nil
}

func (cm *ConnectionManager) GetWhatsAppSession(sectorID int) ([]byte, error) {
	utils.LogDebug("Buscando referência de sessão do WhatsApp no banco para setor %d", sectorID)

	// Verificar quais colunas existem na tabela
	var sessionFilePathExists bool
	var sessionDataExists bool

	// Verificar se a coluna session_file_path existe
	err := cm.db.QueryRow(`
		SELECT COUNT(*) > 0 
		FROM information_schema.COLUMNS 
		WHERE TABLE_SCHEMA = DATABASE() 
		AND TABLE_NAME = 'whatsapp_connections' 
		AND COLUMN_NAME = 'session_file_path'`).Scan(&sessionFilePathExists)

	if err != nil {
		utils.LogError("Erro ao verificar coluna session_file_path: %v", err)
	}

	// Verificar se a coluna session_data existe
	err = cm.db.QueryRow(`
		SELECT COUNT(*) > 0 
		FROM information_schema.COLUMNS 
		WHERE TABLE_SCHEMA = DATABASE() 
		AND TABLE_NAME = 'whatsapp_connections' 
		AND COLUMN_NAME = 'session_data'`).Scan(&sessionDataExists)

	if err != nil {
		utils.LogError("Erro ao verificar coluna session_data: %v", err)
	}

	var sessionData []byte

	// Tentar usar session_file_path primeiro, depois session_data
	if sessionFilePathExists {
		var sessionFilePath string
		err = cm.db.QueryRow(`
			SELECT session_file_path 
			FROM whatsapp_connections 
			WHERE setor_id = ? AND session_file_path IS NOT NULL`,
			sectorID).Scan(&sessionFilePath)

		if err == nil {
			utils.LogInfo("Referência de sessão recuperada com sucesso do banco para setor %d", sectorID)
			return []byte(sessionFilePath), nil
		} else if err != sql.ErrNoRows {
			utils.LogError("Erro ao buscar session_file_path para setor %d: %v", sectorID, err)
		}
	}

	// Se não encontrou na primeira coluna, tentar a segunda
	if sessionDataExists {
		err = cm.db.QueryRow(`
			SELECT session_data 
			FROM whatsapp_connections 
			WHERE setor_id = ? AND session_data IS NOT NULL`,
			sectorID).Scan(&sessionData)

		if err == nil {
			utils.LogInfo("Dados de sessão recuperados com sucesso do banco para setor %d", sectorID)
			return sessionData, nil
		} else if err != sql.ErrNoRows {
			utils.LogError("Erro ao buscar session_data para setor %d: %v", sectorID, err)
		}
	}

	if err == sql.ErrNoRows {
		utils.LogDebug("Nenhuma referência de sessão encontrada no banco para setor %d", sectorID)
		return nil, nil
	}

	if !sessionFilePathExists && !sessionDataExists {
		return nil, fmt.Errorf("nenhuma coluna de sessão válida encontrada na tabela whatsapp_connections")
	}

	return nil, nil
}

func (cm *ConnectionManager) CloseAllConnections() error {
	utils.LogInfo("Iniciando limpeza total do sistema e fechamento de todas as conexões")

	done := make(chan bool)
	go func() {
		cm.mutex.Lock()
		defer cm.mutex.Unlock()

		for sectorID, service := range cm.connections {
			if service != nil && service.client != nil {
				utils.LogInfo("Encerrando conexão do setor %d", sectorID)
				service.client.Disconnect()
			}
		}

		dataDir := "data"
		if err := os.RemoveAll(dataDir); err != nil {
			utils.LogWarning("Não foi possível remover o diretório de dados: %v", err)
		} else {
			utils.LogInfo("Diretório de dados removido com sucesso")
		}

		_, err := cm.db.Exec(`
			UPDATE whatsapp_connections 
			SET status = 'disconnected',
				qrcode_base64 = NULL,
				session_data = NULL,
				last_disconnected_at = NOW(),
				updated_at = NOW()`)
		if err != nil {
			utils.LogWarning("Não foi possível limpar os dados no banco: %v", err)
		} else {
			utils.LogInfo("Dados do banco limpos com sucesso")
		}

		for sectorID := range cm.connections {
			delete(cm.connections, sectorID)
		}

		done <- true
	}()

	select {
	case <-done:
		utils.LogInfo("Sistema limpo e todas as conexões encerradas com sucesso")
		return nil
	case <-time.After(5 * time.Second):
		utils.LogWarning("Tempo limite excedido ao limpar o sistema, encerrando mesmo assim")
		return nil
	}
}

func (cm *ConnectionManager) CloseConnection(sectorID int) error {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	service, exists := cm.connections[sectorID]
	if !exists || service == nil {
		return fmt.Errorf("não foi encontrada conexão para o setor %d", sectorID)
	}

	utils.LogInfo("Encerrando conexão do WhatsApp para o setor %d", sectorID)

	if service.client != nil {
		service.client.Disconnect()
	}

	dbPath := service.getDBPath()
	if err := os.Remove(dbPath); err != nil && !os.IsNotExist(err) {
		utils.LogWarning("Não foi possível remover o banco de dados do setor %d: %v", sectorID, err)
	}

	_, err := cm.db.Exec(`
		UPDATE whatsapp_connections 
		SET status = 'disconnected',
			qrcode_base64 = NULL,
			last_disconnected_at = NOW(),
			updated_at = NOW()
		WHERE setor_id = ?`,
		sectorID)
	if err != nil {
		utils.LogWarning("Não foi possível atualizar o status do setor %d para desconectado: %v", sectorID, err)
	}

	delete(cm.connections, sectorID)
	return nil
}

func (cm *ConnectionManager) GetDB() *sql.DB {
	return cm.db
}
