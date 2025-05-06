package services

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"whatsapp-bot/config"
	"whatsapp-bot/internal/models"
	"whatsapp-bot/internal/repositories"
	"whatsapp-bot/internal/utils"
	"whatsapp-bot/internal/wsnotify"

	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
	_ "modernc.org/sqlite"
)

type WhatsAppService struct {
	client            *whatsmeow.Client
	config            *config.Config
	connected         bool
	qrCodeImage       []byte
	qrCodeMutex       sync.RWMutex
	qrCodeReady       bool
	sectorID          int
	manager           *ConnectionManager
	connectionManager *ConnectionManager
	messageRepository models.MessageRepository
	contactRepository models.ContactRepository
	s3Service         *S3Service
	userRepository    models.UserRepository
}

func NewWhatsAppService(config *config.Config, connectionManager *ConnectionManager, messageRepository models.MessageRepository, contactRepository models.ContactRepository) *WhatsAppService {
	s3Service, err := NewS3Service(config.S3Config)
	if err != nil {
		utils.LogError("Error creating S3 service: %v", err)
	}

	userRepository := repositories.NewMySQLUserRepository(connectionManager.db)

	service := &WhatsAppService{
		config:            config,
		qrCodeMutex:       sync.RWMutex{},
		qrCodeReady:       false,
		connectionManager: connectionManager,
		messageRepository: messageRepository,
		contactRepository: contactRepository,
		s3Service:         s3Service,
		userRepository:    userRepository,
	}
	return service
}

func (s *WhatsAppService) SetSectorAndManager(sectorID int, manager *ConnectionManager) {
	s.sectorID = sectorID
	s.manager = manager
}

func (s *WhatsAppService) getDBPath() string {
	return fmt.Sprintf("whatsapp-%d.db", s.sectorID)
}

func (s *WhatsAppService) Connect() error {
	// Definir nome e tipo de plataforma para aparecer como navegador
	store.DeviceProps.Os = proto.String("LigChat")
	store.DeviceProps.PlatformType = waProto.DeviceProps_DESKTOP.Enum()

	utils.LogInfo("Conectando ao WhatsApp para setor %d", s.sectorID)

	if s.client != nil {
		utils.LogDebug("Cliente já existe, tentando reconectar")
		return s.Reconnect()
	}

	dbPath := s.getDBPath()
	utils.LogDebug("Usando banco de dados em: %s", dbPath)

	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return fmt.Errorf("erro ao criar diretório para banco de dados: %v", err)
	}

	if _, err := os.Stat(dbPath); err == nil {
		utils.LogInfo("Removendo arquivo de sessão antigo após reinicialização")
		if err := os.Remove(dbPath); err != nil {
			utils.LogError("Erro ao remover arquivo antigo: %v", err)
		}
	}

	clientLog := waLog.Stdout("Client", "DEBUG", true)

	// Configurar SQLite com pragmas otimizados
	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(10000)&_pragma=cache_size(10000)", dbPath)
	deviceStore, err := sqlstore.New("sqlite", dsn, nil)
	if err != nil {
		return fmt.Errorf("erro ao criar device store: %v", err)
	}

	device := deviceStore.NewDevice()
	client := whatsmeow.NewClient(device, clientLog)
	s.client = client

	s.client.AddEventHandler(s.eventHandler)

	// Definir nome do dispositivo (aparece em Aparelhos Conectados)
	client.Store.Platform = "LigChat"

	utils.LogInfo("Gerando QR code para setor %d", s.sectorID)
	qrChan, _ := s.client.GetQRChannel(context.Background())
	go func() {
		for evt := range qrChan {
			if evt.Event == "code" {
				utils.LogInfo("QR code recebido para setor %d", s.sectorID)
				s.saveQRCode(evt.Code)
			}
		}
	}()

	utils.LogInfo("Tentando conectar ao WhatsApp para setor %d", s.sectorID)
	if err := client.Connect(); err != nil {
		utils.LogError("Erro ao conectar para setor %d: %v", s.sectorID, err)
		return fmt.Errorf("erro ao conectar: %v", err)
	}

	return nil
}

func (s *WhatsAppService) saveQRCode(qrCode string) {
	utils.LogDebug("Salvando QR code no banco de dados")

	if s.manager != nil {
		err := s.manager.UpdateQRCode(s.sectorID, qrCode)
		if err != nil {
			utils.LogError("Erro ao salvar QR code: %v", err)
		}
	}
}

func (s *WhatsAppService) GetQRCodeImage() ([]byte, bool) {
	s.qrCodeMutex.RLock()
	defer s.qrCodeMutex.RUnlock()

	return s.qrCodeImage, s.qrCodeReady
}

func (s *WhatsAppService) IsConnected() bool {
	if s.client == nil {
		return false
	}

	// Verificar se o cliente está conectado e autenticado
	if s.client.IsConnected() && s.client.IsLoggedIn() && s.connected {
		return true
	}

	return false
}

func (s *WhatsAppService) SetConnected(connected bool) {
	s.connected = connected
}

func (s *WhatsAppService) eventHandler(evt interface{}) {
	fmt.Printf("[DEBUG-WS] Evento de mensagem recebido: %+v\n", evt)
	switch evt.(type) {
	case *events.Message:
		s.handleMessage(evt)
	case *events.Connected:
		utils.LogInfo("WhatsApp conectado para setor %d", s.sectorID)
		s.SetConnected(true)
		s.manager.SetConnected(s.sectorID)
	case *events.Disconnected:
		utils.LogWarning("WhatsApp desconectado para setor %d", s.sectorID)
		s.SetConnected(false)
		s.manager.SetDisconnected(s.sectorID)
	case *events.LoggedOut:
		utils.LogWarning("WhatsApp deslogado para setor %d", s.sectorID)
		s.SetConnected(false)
		s.manager.SetDisconnected(s.sectorID)
	}
}

func (s *WhatsAppService) downloadFromUrl(url string) ([]byte, error) {
	utils.LogInfo("Baixando arquivo de %s", url)

	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("erro ao baixar arquivo: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("erro ao baixar arquivo. Status: %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

func (s *WhatsAppService) handleDatabaseLock() error {
	utils.LogWarning("Tentando resolver bloqueio do banco de dados para setor %d", s.sectorID)

	if s.client != nil {
		s.client.Disconnect()
		time.Sleep(2 * time.Second)
		s.client = nil
	}

	dbPath := s.getDBPath()
	timestamp := time.Now().Format("20060102150405")
	backupPath := fmt.Sprintf("%s.backup.%s", dbPath, timestamp)

	// Tentar limpar o arquivo WAL e SHM antes de mover
	walPath := dbPath + "-wal"
	shmPath := dbPath + "-shm"

	// Remover arquivos WAL e SHM se existirem
	if _, err := os.Stat(walPath); err == nil {
		if err := os.Remove(walPath); err != nil {
			utils.LogWarning("Erro ao remover arquivo WAL: %v", err)
		}
	}
	if _, err := os.Stat(shmPath); err == nil {
		if err := os.Remove(shmPath); err != nil {
			utils.LogWarning("Erro ao remover arquivo SHM: %v", err)
		}
	}

	// Aguardar um pouco para garantir que todos os handles foram liberados
	time.Sleep(2 * time.Second)

	if err := os.Rename(dbPath, backupPath); err != nil {
		utils.LogError("Erro ao criar backup do banco de dados: %v", err)
		return fmt.Errorf("erro ao criar backup do banco: %v", err)
	}

	utils.LogInfo("Backup do banco criado em: %s", backupPath)
	time.Sleep(2 * time.Second)

	// Tentar reconectar com novo banco
	if err := s.Connect(); err != nil {
		utils.LogError("Erro ao reconectar após resolver bloqueio: %v", err)
		// Tentar restaurar backup apenas se o novo banco não foi criado
		if _, newDbErr := os.Stat(dbPath); os.IsNotExist(newDbErr) {
			if err := os.Rename(backupPath, dbPath); err != nil {
				utils.LogError("Erro ao restaurar backup após falha na reconexão: %v", err)
			}
		}
		return fmt.Errorf("erro ao reconectar: %v", err)
	}

	utils.LogInfo("Conexão restaurada com sucesso após resolver bloqueio do banco")
	return nil
}

func (s *WhatsAppService) SendMessage(sectorID int, recipient string, message string, userID *int, isAnonymous bool) error {
	prefix := ""
	if userID != nil && !isAnonymous {
		user, err := s.userRepository.GetByID(*userID)
		if err == nil && user != nil {
			prefix = "*" + user.Name + "*:\n\n"
		}
	}
	msgToSend := prefix + message

	conn, err := s.connectionManager.GetConnection(sectorID)
	if err != nil {
		return err
	}

	jid, err := utils.ParseJID(recipient)
	if err != nil {
		return err
	}

	// Buscar ou criar o contato para atualizar sua ordem
	contact, err := s.contactRepository.GetByNumber(sectorID, recipient)
	if err != nil {
		utils.LogError("Erro ao buscar contato: %v", err)
	} else if contact == nil {
		contact, err = s.contactRepository.CreateIfNotExists(sectorID, recipient)
		if err != nil {
			utils.LogError("Erro ao criar contato: %v", err)
		}
	}

	utils.LogInfo("Tentando enviar mensagem para %s", recipient)

	msg, err := conn.client.SendMessage(context.Background(), jid, &waProto.Message{
		Conversation: proto.String(msgToSend),
	})

	if err != nil {
		utils.LogWarning("Erro na primeira tentativa de envio: %v", err)

		if strings.Contains(err.Error(), "database is locked") ||
			strings.Contains(err.Error(), "SQLITE_BUSY") {
			utils.LogInfo("Detectado banco de dados bloqueado, tentando resolver...")
			if fixErr := conn.handleDatabaseLock(); fixErr != nil {
				return fmt.Errorf("erro ao consertar banco de dados: %v (original: %v)", fixErr, err)
			}
		}

		if strings.Contains(err.Error(), "untrusted identity") {
			utils.LogInfo("Detectada identidade não confiável, tentando resolver...")
			if err := conn.Reconnect(); err != nil {
				return fmt.Errorf("erro ao reconectar após identidade não confiável: %v", err)
			}
		}

		utils.LogInfo("Tentando enviar mensagem novamente...")
		msg, err = conn.client.SendMessage(context.Background(), jid, &waProto.Message{
			Conversation: proto.String(msgToSend),
		})

		if err != nil {
			if strings.Contains(err.Error(), "server returned error 479") {
				return fmt.Errorf("erro de conexão com WhatsApp (479), por favor tente novamente em alguns instantes")
			}
			return fmt.Errorf("erro persistente ao enviar mensagem: %v", err)
		}
	}

	utils.LogInfo("Mensagem enviada com sucesso para %s", recipient)

	err = s.SaveMessage(sectorID, recipient, message, "text", "", "", "", msg.ID, true, userID, isAnonymous)
	if err != nil {
		utils.LogError("Error saving message: %v", err)
	}

	// Quando enviamos uma resposta, marca todas as mensagens anteriores desse contato como lidas
	// e envia uma atualização por WebSocket
	go s.markPreviousMessagesAsRead(sectorID, recipient)

	// Mover o contato para o topo da lista
	if contact != nil {
		go s.contactRepository.UpdateContactOrder(sectorID, contact.ID)
	}

	return nil
}

// Nova função para marcar mensagens anteriores como lidas
func (s *WhatsAppService) markPreviousMessagesAsRead(sectorID int, contactJID string) {
	// Buscar o contato
	contact, err := s.contactRepository.GetByNumber(sectorID, contactJID)
	if err != nil || contact == nil {
		utils.LogError("Erro ao buscar contato para marcar mensagens como lidas: %v", err)
		return
	}

	// Buscar mensagens recebidas deste contato (não enviadas pelo sistema)
	// Aqui precisamos fazer uma consulta personalizada
	query := `
		SELECT id FROM messages 
		WHERE id_setor = ? AND contato_id = ? AND enviado = 0
		ORDER BY data_envio DESC 
		LIMIT 20` // Limitar a 20 mensagens mais recentes

	rows, err := s.connectionManager.db.Query(query, sectorID, contact.ID)
	if err != nil {
		utils.LogError("Erro ao buscar mensagens anteriores: %v", err)
		return
	}
	defer rows.Close()

	// Para cada mensagem, enviar uma atualização WebSocket
	var messageIDs []int
	for rows.Next() {
		var messageID int
		if err := rows.Scan(&messageID); err == nil {
			messageIDs = append(messageIDs, messageID)

			// Buscar detalhes da mensagem para enviar atualização
			message, err := s.messageRepository.GetByID(messageID)
			if err == nil && message != nil {
				// Enviar recibo de leitura para o WhatsApp oficial
				if s.client != nil && message.WhatsAppMessageID != "" {
					jid := utils.JIDFromContatoID(message.ContatoID)
					s.client.MarkRead(
						[]types.MessageID{types.MessageID(message.WhatsAppMessageID)},
						time.Now(),
						jid,
						jid,
					)
				}
				// Enviar evento de atualização por WebSocket
				var urlPtr, fileNamePtr, mimeTypePtr *string
				if message.URL != "" {
					urlPtr = &message.URL
				}
				if message.NomeArquivo != "" {
					fileNamePtr = &message.NomeArquivo
				}
				if message.MimeType != "" {
					mimeTypePtr = &message.MimeType
				}

				// Enviar WebSocket com status atualizado para "lido" (duas barras azuis)
				wsnotify.SendMessageEvent(
					message.ID,
					int(message.ContatoID),
					message.IDSetor,
					message.Conteudo,
					message.Tipo,
					urlPtr,
					fileNamePtr,
					mimeTypePtr,
					message.DataEnvio,
					message.Enviado,
					true,              // Marcar como lida
					models.StatusRead, // Status com duas barras azuis
				)
			}
		}
	}

	// Marcar no banco também como lidas
	if len(messageIDs) > 0 {
		utils.LogInfo("Marcando %d mensagens como lidas para o contato %s", len(messageIDs), contactJID)
		if err := s.messageRepository.MarkMessagesAsRead(messageIDs); err != nil {
			utils.LogError("Erro ao marcar mensagens como lidas no banco: %v", err)
		}
	}
}

func (s *WhatsAppService) SendImage(sectorID int, recipient string, imageBytes []byte, caption string, userID *int, isAnonymous bool) error {
	conn, err := s.connectionManager.GetConnection(sectorID)
	if err != nil {
		return err
	}

	jid, err := utils.ParseJID(recipient)
	if err != nil {
		return err
	}

	// Buscar ou criar o contato para atualizar sua ordem
	contact, err := s.contactRepository.GetByNumber(sectorID, recipient)
	if err != nil {
		utils.LogError("Erro ao buscar contato: %v", err)
	} else if contact == nil {
		contact, err = s.contactRepository.CreateIfNotExists(sectorID, recipient)
		if err != nil {
			utils.LogError("Erro ao criar contato: %v", err)
		}
	}

	mimeType := http.DetectContentType(imageBytes)
	fileName := fmt.Sprintf("sector_%d/images/%d.%s", sectorID, time.Now().UnixNano(), utils.GetExtensionFromMime(mimeType))

	utils.LogInfo("Fazendo upload da imagem para S3...")
	s3URL, err := s.s3Service.UploadBytes(imageBytes, fileName, mimeType)
	if err != nil {
		return fmt.Errorf("erro ao fazer upload para S3: %v", err)
	}

	utils.LogInfo("Upload para S3 concluído, URL: %s", s3URL)

	uploaded, err := conn.client.Upload(context.Background(), imageBytes, whatsmeow.MediaImage)
	if err != nil {
		utils.LogError("Erro no upload da imagem: %v", err)
		if strings.Contains(err.Error(), "database is locked") || strings.Contains(err.Error(), "SQLITE_BUSY") {
			if fixErr := conn.handleDatabaseLock(); fixErr != nil {
				return fmt.Errorf("erro ao consertar banco: %v (original: %v)", fixErr, err)
			}
			uploaded, err = conn.client.Upload(context.Background(), imageBytes, whatsmeow.MediaImage)
			if err != nil {
				return fmt.Errorf("erro persistente ao fazer upload: %v", err)
			}
		} else {
			return err
		}
	}

	imgMsg := &waProto.Message{
		ImageMessage: &waProto.ImageMessage{
			Caption:       proto.String(caption),
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			Mimetype:      proto.String(mimeType),
			FileLength:    proto.Uint64(uint64(len(imageBytes))),
			FileSHA256:    uploaded.FileSHA256,
			FileEncSHA256: uploaded.FileEncSHA256,
		},
	}

	msg, err := conn.client.SendMessage(context.Background(), jid, imgMsg)
	if err != nil {
		if strings.Contains(err.Error(), "untrusted identity") ||
			strings.Contains(err.Error(), "database is locked") ||
			strings.Contains(err.Error(), "SQLITE_BUSY") {
			if fixErr := conn.handleDatabaseLock(); fixErr != nil {
				return fmt.Errorf("erro ao consertar banco: %v (original: %v)", fixErr, err)
			}
			msg, err = conn.client.SendMessage(context.Background(), jid, imgMsg)
			if err != nil {
				return fmt.Errorf("erro persistente ao enviar imagem: %v", err)
			}
		} else {
			return err
		}
	}

	err = s.SaveMessage(sectorID, recipient, caption, "image", s3URL, fileName, mimeType, msg.ID, true, userID, isAnonymous)
	if err != nil {
		utils.LogError("Error saving message: %v", err)
	}

	// Quando enviamos uma imagem, marcar mensagens anteriores como lidas
	go s.markPreviousMessagesAsRead(sectorID, recipient)

	// Mover o contato para o topo da lista
	if contact != nil {
		go s.contactRepository.UpdateContactOrder(sectorID, contact.ID)
	}

	return nil
}

// Função para converter qualquer formato de áudio para OGG usando ffmpeg
func (s *WhatsAppService) convertToOgg(audioBytes []byte) ([]byte, float32, []byte, error) {
	// Criar diretório temporário com caminho absoluto
	tempDir, err := os.MkdirTemp("", "whatsapp_audio_*")
	if err != nil {
		return nil, 0, nil, fmt.Errorf("erro ao criar diretório temporário: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Detectar extensão baseado no MIME type
	mimeType := http.DetectContentType(audioBytes)
	ext := ".bin"
	switch {
	case strings.Contains(mimeType, "wav"):
		ext = ".wav"
	case strings.Contains(mimeType, "mp3"):
		ext = ".mp3"
	case strings.Contains(mimeType, "ogg"):
		ext = ".ogg"
	case strings.Contains(mimeType, "webm"):
		ext = ".webm"
	case strings.Contains(mimeType, "mpeg"):
		ext = ".mp3"
	case strings.Contains(mimeType, "x-m4a"):
		ext = ".m4a"
	}

	// Usar caminhos absolutos para os arquivos temporários
	tempInputPath := filepath.Join(tempDir, fmt.Sprintf("input_%d%s", time.Now().UnixNano(), ext))
	if err := os.WriteFile(tempInputPath, audioBytes, 0644); err != nil {
		return nil, 0, nil, fmt.Errorf("erro ao salvar arquivo de áudio temporário: %v", err)
	}

	tempOggPath := filepath.Join(tempDir, fmt.Sprintf("output_%d.ogg", time.Now().UnixNano()))
	tempWavPath := filepath.Join(tempDir, fmt.Sprintf("temp_%d.wav", time.Now().UnixNano()))

	utils.LogInfo("Convertendo áudio para OGG (Opus) usando FFmpeg...")
	utils.LogDebug("Arquivo de entrada: %s", tempInputPath)
	utils.LogDebug("Arquivo de saída: %s", tempOggPath)

	// Primeiro, converter para WAV para análise
	cmdWav := exec.Command("ffmpeg",
		"-i", tempInputPath,
		"-acodec", "pcm_s16le",
		"-ar", "16000",
		"-ac", "1",
		"-y",
		tempWavPath,
	)

	if err := cmdWav.Run(); err != nil {
		return nil, 0, nil, fmt.Errorf("erro ao converter para WAV: %v", err)
	}

	// Obter duração do áudio
	duration, err := s.getAudioDuration(tempWavPath)
	if err != nil {
		utils.LogError("Erro ao obter duração do áudio: %v", err)
		duration = 0
	}

	// Gerar waveform
	waveform, err := s.generateWaveform(tempWavPath)
	if err != nil {
		utils.LogError("Erro ao gerar waveform: %v", err)
		waveform = []byte{0x5, 0x8, 0x10, 0x12, 0x15, 0x12, 0x10, 0x8, 0x5}
	}

	// Converter para OGG final
	cmd := exec.Command("ffmpeg",
		"-i", tempInputPath,
		"-c:a", "libopus",
		"-ar", "48000",
		"-ac", "1",
		"-b:a", "128k",
		"-application", "voip",
		"-y",
		tempOggPath,
	)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		utils.LogError("Erro ao executar FFmpeg: %v\nSaída de erro: %s", err, stderr.String())
		return nil, 0, nil, fmt.Errorf("erro ao converter áudio: %v\nDetalhes: %s", err, stderr.String())
	}

	// Verificar se o arquivo de saída foi criado
	if _, err := os.Stat(tempOggPath); os.IsNotExist(err) {
		return nil, 0, nil, fmt.Errorf("arquivo de saída não foi criado pelo FFmpeg")
	}

	// Ler o arquivo OGG convertido
	oggBytes, err := os.ReadFile(tempOggPath)
	if err != nil {
		return nil, 0, nil, fmt.Errorf("erro ao ler arquivo OGG convertido: %v", err)
	}

	utils.LogInfo("Conversão concluída com sucesso. Tamanho do arquivo: %d bytes, Duração: %.2f segundos", len(oggBytes), duration)
	return oggBytes, duration, waveform, nil
}

func (s *WhatsAppService) getAudioDuration(filePath string) (float32, error) {
	cmd := exec.Command("ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		filePath,
	)

	output, err := cmd.Output()
	if err != nil {
		return 0, err
	}

	duration, err := strconv.ParseFloat(strings.TrimSpace(string(output)), 32)
	if err != nil {
		return 0, err
	}

	return float32(duration), nil
}

func (s *WhatsAppService) generateWaveform(wavPath string) ([]byte, error) {
	cmd := exec.Command("ffmpeg",
		"-i", wavPath,
		"-filter:a", "aformat=channel_layouts=mono,highpass=f=200,lowpass=f=3000,compand=gain=20:attack=0.01:release=0.05:points=-90/-90 -70/-90 -15/-15 0/-10,showwavespic=s=32x32:colors=black:filter=peak:scale=lin",
		"-frames:v", "1",
		"-f", "image2pipe",
		"-vcodec", "rawvideo",
		"-pix_fmt", "gray",
		"pipe:1",
	)

	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	// Converter a imagem em escala de cinza para valores de 0-31
	waveform := make([]byte, 32)
	for i := 0; i < 32; i++ {
		var sum int
		for j := 0; j < 32; j++ {
			sum += int(output[i*32+j])
		}
		avg := sum / 32

		// Aplicar curva de resposta não linear mais agressiva
		normalized := float64(avg) / 255.0

		// Usar uma função de potência com expoente menor para aumentar valores baixos
		// e uma escala maior para aumentar a amplitude geral
		enhanced := math.Pow(normalized, 0.4) // Expoente menor = mais sensível a sons baixos

		// Aumentar a amplitude geral e garantir que valores baixos sejam mais visíveis
		amplified := math.Min(enhanced*1.8+0.2, 1.0) // Adicionar offset e amplificar

		// Converter para byte, garantindo valor mínimo de 3 para ter sempre alguma visualização
		value := int(amplified * 31)
		if value < 3 {
			value = 3
		}
		waveform[i] = byte(value)
	}

	return waveform, nil
}

func (s *WhatsAppService) SendAudio(sectorID int, recipient string, audioBytes []byte, userID *int, isAnonymous bool) error {
	conn, err := s.connectionManager.GetConnection(sectorID)
	if err != nil {
		return err
	}

	jid, err := utils.ParseJID(recipient)
	if err != nil {
		return err
	}

	// Buscar ou criar o contato para atualizar sua ordem
	contact, err := s.contactRepository.GetByNumber(sectorID, recipient)
	if err != nil {
		utils.LogError("Erro ao buscar contato: %v", err)
	} else if contact == nil {
		contact, err = s.contactRepository.CreateIfNotExists(sectorID, recipient)
		if err != nil {
			utils.LogError("Erro ao criar contato: %v", err)
		}
	}

	mimeType := http.DetectContentType(audioBytes)
	utils.LogInfo("Tipo MIME original detectado: %s", mimeType)

	// Sempre converter para OGG com codec Opus
	utils.LogInfo("Convertendo áudio para OGG (Opus)...")
	oggBytes, duration, waveform, err := s.convertToOgg(audioBytes)
	if err != nil {
		utils.LogError("Erro ao converter áudio para OGG (Opus): %v", err)
		// Se falhar a conversão, tentar enviar como documento
		return s.SendDocument(sectorID, recipient, audioBytes, "audio"+filepath.Ext(mimeType), userID, isAnonymous)
	}

	fileName := fmt.Sprintf("sector_%d/audios/%d.ogg", sectorID, time.Now().UnixNano())
	utils.LogInfo("Fazendo upload do áudio para S3...")
	s3URL, err := s.s3Service.UploadBytes(oggBytes, fileName, "audio/ogg; codecs=opus")
	if err != nil {
		return fmt.Errorf("erro ao fazer upload para S3: %v", err)
	}

	utils.LogInfo("Upload para S3 concluído, URL: %s", s3URL)

	uploaded, err := conn.client.Upload(context.Background(), oggBytes, whatsmeow.MediaAudio)
	if err != nil {
		utils.LogError("Erro no upload do áudio: %v", err)
		if strings.Contains(err.Error(), "database is locked") || strings.Contains(err.Error(), "SQLITE_BUSY") {
			if fixErr := conn.handleDatabaseLock(); fixErr != nil {
				return fmt.Errorf("erro ao consertar banco: %v (original: %v)", fixErr, err)
			}
			uploaded, err = conn.client.Upload(context.Background(), oggBytes, whatsmeow.MediaAudio)
			if err != nil {
				return fmt.Errorf("erro persistente ao fazer upload: %v", err)
			}
		} else {
			return err
		}
	}

	audioMsg := &waProto.AudioMessage{
		URL:           proto.String(uploaded.URL),
		DirectPath:    proto.String(uploaded.DirectPath),
		MediaKey:      uploaded.MediaKey,
		Mimetype:      proto.String("audio/ogg; codecs=opus"),
		FileLength:    proto.Uint64(uint64(len(oggBytes))),
		FileSHA256:    uploaded.FileSHA256,
		FileEncSHA256: uploaded.FileEncSHA256,
		Seconds:       proto.Uint32(uint32(duration)),
		PTT:           proto.Bool(true),
		Waveform:      waveform,
	}

	msg, err := conn.client.SendMessage(context.Background(), jid, &waProto.Message{
		AudioMessage: audioMsg,
	})

	if err != nil {
		if strings.Contains(err.Error(), "untrusted identity") ||
			strings.Contains(err.Error(), "database is locked") ||
			strings.Contains(err.Error(), "SQLITE_BUSY") {
			if fixErr := conn.handleDatabaseLock(); fixErr != nil {
				return fmt.Errorf("erro ao consertar banco: %v (original: %v)", fixErr, err)
			}
			msg, err = conn.client.SendMessage(context.Background(), jid, &waProto.Message{
				AudioMessage: audioMsg,
			})
			if err != nil {
				return fmt.Errorf("erro persistente ao enviar áudio: %v", err)
			}
		} else {
			return err
		}
	}

	err = s.SaveMessage(sectorID, recipient, "", "audio", s3URL, fileName, "audio/ogg; codecs=opus", msg.ID, true, userID, isAnonymous)
	if err != nil {
		utils.LogError("Error saving message: %v", err)
	}

	// Quando enviamos um áudio, marcar mensagens anteriores como lidas
	go s.markPreviousMessagesAsRead(sectorID, recipient)

	// Mover o contato para o topo da lista
	if contact != nil {
		go s.contactRepository.UpdateContactOrder(sectorID, contact.ID)
	}

	return nil
}

func (s *WhatsAppService) SendDocument(sectorID int, recipient string, fileBytes []byte, filename string, userID *int, isAnonymous bool) error {
	conn, err := s.connectionManager.GetConnection(sectorID)
	if err != nil {
		return err
	}

	jid, err := utils.ParseJID(recipient)
	if err != nil {
		return err
	}

	// Buscar ou criar o contato para atualizar sua ordem
	contact, err := s.contactRepository.GetByNumber(sectorID, recipient)
	if err != nil {
		utils.LogError("Erro ao buscar contato: %v", err)
	} else if contact == nil {
		contact, err = s.contactRepository.CreateIfNotExists(sectorID, recipient)
		if err != nil {
			utils.LogError("Erro ao criar contato: %v", err)
		}
	}

	mimeType := http.DetectContentType(fileBytes)
	s3FileName := fmt.Sprintf("sector_%d/documents/%d_%s", sectorID, time.Now().UnixNano(), filename)

	utils.LogInfo("Fazendo upload do documento para S3...")
	s3URL, err := s.s3Service.UploadBytes(fileBytes, s3FileName, mimeType)
	if err != nil {
		return fmt.Errorf("erro ao fazer upload para S3: %v", err)
	}

	utils.LogInfo("Upload para S3 concluído, URL: %s", s3URL)

	uploaded, err := conn.client.Upload(context.Background(), fileBytes, whatsmeow.MediaDocument)
	if err != nil {
		if strings.Contains(err.Error(), "database is locked") || strings.Contains(err.Error(), "SQLITE_BUSY") {
			if fixErr := conn.handleDatabaseLock(); fixErr != nil {
				return fmt.Errorf("erro ao consertar banco: %v (original: %v)", fixErr, err)
			}
			uploaded, err = conn.client.Upload(context.Background(), fileBytes, whatsmeow.MediaDocument)
			if err != nil {
				return fmt.Errorf("erro persistente ao fazer upload: %v", err)
			}
		} else {
			return err
		}
	}

	docMsg := &waProto.Message{
		DocumentMessage: &waProto.DocumentMessage{
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			Mimetype:      proto.String(mimeType),
			Title:         proto.String(filename),
			FileName:      proto.String(filename),
			FileLength:    proto.Uint64(uint64(len(fileBytes))),
			FileSHA256:    uploaded.FileSHA256,
			FileEncSHA256: uploaded.FileEncSHA256,
		},
	}

	msg, err := conn.client.SendMessage(context.Background(), jid, docMsg)
	if err != nil {
		if strings.Contains(err.Error(), "untrusted identity") ||
			strings.Contains(err.Error(), "database is locked") ||
			strings.Contains(err.Error(), "SQLITE_BUSY") {
			if fixErr := conn.handleDatabaseLock(); fixErr != nil {
				return fmt.Errorf("erro ao consertar banco: %v (original: %v)", fixErr, err)
			}
			msg, err = conn.client.SendMessage(context.Background(), jid, docMsg)
			if err != nil {
				return fmt.Errorf("erro persistente ao enviar documento: %v", err)
			}
		} else {
			return err
		}
	}

	err = s.SaveMessage(sectorID, recipient, filename, "document", s3URL, filename, mimeType, msg.ID, true, userID, isAnonymous)
	if err != nil {
		utils.LogError("Error saving message: %v", err)
	}

	// Quando enviamos um documento, marcar mensagens anteriores como lidas
	go s.markPreviousMessagesAsRead(sectorID, recipient)

	// Mover o contato para o topo da lista
	if contact != nil {
		go s.contactRepository.UpdateContactOrder(sectorID, contact.ID)
	}

	return nil
}

func (s *WhatsAppService) SendTyping(recipient string, duration int) error {
	if !s.connected {
		return fmt.Errorf("whatsapp não está conectado")
	}

	if s.client == nil {
		return fmt.Errorf("cliente do whatsapp não inicializado")
	}

	// Adiciona código do país (Brasil - 55) se não estiver presente
	if len(recipient) == 11 || len(recipient) == 10 {
		recipient = "55" + recipient
	}

	jid, err := types.ParseJID(recipient + "@s.whatsapp.net")
	if err != nil {
		return fmt.Errorf("número de telefone inválido: %v", err)
	}

	// Se a duração não for especificada, usa 5 segundos como padrão
	if duration <= 0 {
		duration = 5
	}

	// Inicia o estado de digitando
	err = s.client.SendPresence(types.PresenceAvailable)
	if err != nil {
		if err.Error() == "can't send presence without PushName set" {
			return fmt.Errorf("não foi possível enviar status de digitação: dispositivo ainda não está totalmente configurado")
		}
		return fmt.Errorf("erro ao definir presença: %v", err)
	}

	err = s.client.SendChatPresence(jid, types.ChatPresenceComposing, types.ChatPresenceMediaText)
	if err != nil {
		return fmt.Errorf("erro ao enviar status de digitação: %v", err)
	}

	// Aguarda a duração especificada
	time.Sleep(time.Duration(duration) * time.Second)

	// Para o estado de digitando
	err = s.client.SendChatPresence(jid, types.ChatPresencePaused, types.ChatPresenceMediaText)
	if err != nil {
		return fmt.Errorf("erro ao limpar status de digitação: %v", err)
	}

	return nil
}

func (s *WhatsAppService) Logout() error {
	if s.client == nil {
		return fmt.Errorf("whatsapp client not initialized")
	}

	err := s.client.Logout()
	if err != nil {
		return fmt.Errorf("error during logout: %v", err)
	}

	s.connected = false
	s.client = nil

	// Notificar o manager sobre a desconexão
	if s.manager != nil {
		err = s.manager.SetDisconnected(s.sectorID)
		if err != nil {
			return fmt.Errorf("error updating disconnected status: %v", err)
		}
	}

	return nil
}

// Adicionar método para reconectar
func (s *WhatsAppService) Reconnect() error {
	utils.LogInfo("Tentando reconectar setor %d...", s.sectorID)

	if s.client == nil {
		return fmt.Errorf("cliente não inicializado")
	}

	// Se já está conectado, retornar imediatamente
	if s.client.IsConnected() {
		utils.LogInfo("WebSocket já está conectado, atualizando estado")
		s.connected = true
		return nil
	}

	// Criar canal para aguardar evento de conexão
	connectedChan := make(chan bool, 1)
	var once sync.Once

	// Adicionar handler temporário para capturar evento de conexão
	tempHandler := func(evt interface{}) {
		if _, ok := evt.(*events.Connected); ok {
			utils.LogDebug("Evento de conexão recebido")
			once.Do(func() {
				connectedChan <- true
			})
		}
	}
	handlerID := s.client.AddEventHandler(tempHandler)

	// Tentar conectar
	utils.LogDebug("Iniciando conexão WebSocket")
	err := s.client.Connect()
	if err != nil {
		s.client.RemoveEventHandler(handlerID)
		return fmt.Errorf("erro ao reconectar: %v", err)
	}

	// Aguardar até 15 segundos pela confirmação de conexão
	utils.LogDebug("Aguardando confirmação de conexão...")
	select {
	case <-connectedChan:
		utils.LogInfo("Reconectado com sucesso")
		s.connected = true
		s.client.RemoveEventHandler(handlerID)
		return nil
	case <-time.After(15 * time.Second):
		s.client.RemoveEventHandler(handlerID)
		// Se já está conectado no nível do WebSocket mas não recebeu o evento
		if s.client.IsConnected() {
			utils.LogInfo("WebSocket conectado mas evento não recebido, assumindo conectado")
			s.connected = true
			return nil
		}
		s.client.Disconnect()
		return fmt.Errorf("timeout aguardando reconexão")
	}
}

func (s *WhatsAppService) DeleteSession() error {
	dbPath := s.getDBPath()

	// Primeiro desconectar e fazer logout se necessário
	if s.client != nil {
		s.client.Disconnect()
		s.client = nil
	}

	// Remover o arquivo do banco de dados
	err := os.Remove(dbPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("erro ao remover arquivo de sessão: %v", err)
	}

	s.connected = false
	return nil
}

func (s *WhatsAppService) SaveSession() error {
	if s.client == nil {
		return fmt.Errorf("whatsapp client not initialized")
	}

	// Garantir que o cliente está conectado antes de salvar
	if !s.client.IsConnected() {
		return fmt.Errorf("cliente não está conectado")
	}

	// Verificar se a ID do dispositivo existe, o que indica uma sessão válida
	if s.client.Store.ID == nil {
		return fmt.Errorf("não há sessão válida para salvar")
	}

	dbPath := s.getDBPath()
	utils.LogInfo("Salvando sessão do WhatsApp para setor %d: %s", s.sectorID, dbPath)

	// Salvar o caminho do arquivo no banco
	return s.manager.SaveWhatsAppSession(s.sectorID, []byte(dbPath))
}

func (s *WhatsAppService) RestoreSession() error {
	fileRef, err := s.manager.GetWhatsAppSession(s.sectorID)
	if err != nil {
		return fmt.Errorf("erro ao buscar referência de sessão do banco: %v", err)
	}

	if fileRef == nil {
		return nil
	}

	// Verificar se o arquivo existe
	dbPath := string(fileRef)
	if _, err := os.Stat(dbPath); err != nil {
		if os.IsNotExist(err) {
			utils.LogWarning("Arquivo de sessão não encontrado em: %s", dbPath)
			return nil
		}
		return fmt.Errorf("erro ao verificar arquivo de sessão: %v", err)
	}

	utils.LogInfo("Usando arquivo de sessão existente: %s", dbPath)
	return nil
}

func (s *WhatsAppService) SaveMessage(sectorID int, contactJID string, content string, messageType string, url string, fileName string, mimeType string, whatsappMessageID string, isFromSystem bool, userID *int, isAnonymous bool) error {
	// Primeiro verificar se o contato já existe
	contact, err := s.contactRepository.GetByNumber(sectorID, contactJID)
	if err != nil {
		return fmt.Errorf("error checking if contact exists: %v", err)
	}

	// Se não existir, criar um novo
	if contact == nil {
		contact, err = s.contactRepository.CreateIfNotExists(sectorID, contactJID)
		if err != nil {
			return fmt.Errorf("error creating contact: %v", err)
		}
	}

	// Atualizar status de visualização baseado em quem enviou a mensagem
	if isFromSystem {
		// Se é mensagem enviada pelo sistema, marcar como visualizada
		err = s.contactRepository.SetViewed(sectorID, contactJID)
	} else {
		// Se é mensagem recebida, marcar como não visualizada
		err = s.contactRepository.SetUnviewed(sectorID, contactJID)
	}
	if err != nil {
		utils.LogError("Error updating viewed status: %v", err)
	}

	// Enviar uma única atualização do status de leitura após processar a mensagem
	go func() {
		// Pequeno atraso para garantir que todas as operações do banco sejam concluídas
		time.Sleep(500 * time.Millisecond)
		err := s.contactRepository.SendUnreadStatusUpdate(sectorID)
		if err != nil {
			utils.LogError("Error sending unread status update: %v", err)
		}

		s.sendContactsList(sectorID)
	}()

	// Criar e salvar a mensagem
	brasiliaLoc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		utils.LogError("Error loading Brasilia timezone: %v", err)
		brasiliaLoc = time.UTC
	}

	message := &models.Message{
		Conteudo:          content,
		Tipo:              messageType,
		URL:               url,
		NomeArquivo:       fileName,
		MimeType:          mimeType,
		IDSetor:           sectorID,
		ContatoID:         int64(contact.ID),
		DataEnvio:         time.Now().In(brasiliaLoc),
		Enviado:           isFromSystem,
		Lido:              false,
		WhatsAppMessageID: whatsappMessageID,
		IsOfficial:        false,
		CreatedAt:         time.Now().In(brasiliaLoc),
		UserID:            userID,
		IsAnonymous:       isAnonymous,
	}

	// Definindo o status da mensagem apenas para o WebSocket
	messageStatus := models.StatusReceived
	if isFromSystem {
		// Mensagens enviadas pelo sistema mostram duas barras azuis (lidas)
		messageStatus = models.StatusRead
	}

	err = s.messageRepository.Save(message)
	if err != nil {
		return fmt.Errorf("error saving message: %v", err)
	}

	// Enviar evento de mensagem via WebSocket
	utils.LogDebug("Enviando evento de mensagem via WebSocket para setor %d", sectorID)
	var urlPtr, fileNamePtr, mimeTypePtr *string
	if url != "" {
		urlPtr = &url
	}
	if fileName != "" {
		fileNamePtr = &fileName
	}
	if mimeType != "" {
		mimeTypePtr = &mimeType
	}

	wsnotify.SendMessageEvent(
		message.ID,
		int(message.ContatoID),
		message.IDSetor,
		message.Conteudo,
		message.Tipo,
		urlPtr,
		fileNamePtr,
		mimeTypePtr,
		message.DataEnvio,
		message.Enviado,
		message.Lido,
		messageStatus,
	)

	return nil
}

func (s *WhatsAppService) handleMessage(evt interface{}) {
	fmt.Printf("[DEBUG-WS] Evento de mensagem recebido: %+v\n", evt)
	if msg, ok := evt.(*events.Message); ok {
		// Log para debug de mensagens recebidas
		utils.LogDebug("Mensagem recebida: ID=%s, IsFromMe=%v, IsGroup=%v, Sender=%s",
			msg.Info.ID, msg.Info.IsFromMe,
			msg.Info.IsGroup, msg.Info.Sender.String())

		senderJID := msg.Info.Sender.String()
		normalizedJID := senderJID
		if parts := strings.Split(senderJID, ":"); len(parts) > 1 {
			normalizedJID = parts[0] + "@" + strings.Split(parts[1], "@")[1]
		}

		// Verificar se é um status pelo endereço do remetente
		if normalizedJID == "status@broadcast" ||
			strings.HasSuffix(normalizedJID, "@broadcast") ||
			strings.Contains(normalizedJID, "status") {
			return
		}

		// Verificar se é um story pela presença de campos específicos
		if msg.Message.GetEphemeralMessage() != nil ||
			msg.Message.GetDeviceSentMessage() != nil ||
			msg.Message.GetViewOnceMessage() != nil {
			utils.LogInfo("Ignorando possível story (tipo de mensagem especial): %+v", msg.Info.ID)
			return
		}

		// Verificar se é uma mensagem de status pela estrutura de dados
		messageStr := fmt.Sprintf("%+v", msg.Message)
		if strings.Contains(messageStr, "StatusMessage") ||
			strings.Contains(messageStr, "Story") ||
			strings.Contains(messageStr, "status") {
			return
		}

		sectorID := s.sectorID
		contact, err := s.contactRepository.GetByNumber(sectorID, normalizedJID)
		if err != nil {
			utils.LogError("Error checking contact: %v", err)
			return
		}

		// Criar contato se não existir
		if contact == nil {
			utils.LogInfo("Criando contato que não existe no banco: %s", normalizedJID)
			contact, err = s.contactRepository.CreateIfNotExists(sectorID, normalizedJID)
			if err != nil {
				utils.LogError("Error creating contact: %v", err)
				return
			}
		}

		// Atualizar foto do contato do WhatsApp se não houver avatar
		if contact.AvatarURL == "" || contact.AvatarURL == "null" {
			s.fetchContactInfo(sectorID, normalizedJID)
		}

		// Mover o contato para o topo da lista
		go s.contactRepository.UpdateContactOrder(sectorID, contact.ID)

		// Marcar como não visualizado ao receber mensagem
		err = s.contactRepository.SetUnviewed(sectorID, normalizedJID)
		if err != nil {
			utils.LogError("Error marking contact as unviewed: %v", err)
		}

		// Enviar uma única atualização do status de leitura após processar a mensagem
		go func() {
			// Pequeno atraso para garantir que todas as operações do banco sejam concluídas
			time.Sleep(500 * time.Millisecond)
			err := s.contactRepository.SendUnreadStatusUpdate(sectorID)
			if err != nil {
				utils.LogError("Error sending unread status update: %v", err)
			}
		}()

		var content string
		var messageType string
		var url string
		var fileName string
		var mimeType string

		switch {
		case msg.Message.GetConversation() != "":
			content = msg.Message.GetConversation()
			messageType = "text"

		case msg.Message.GetImageMessage() != nil:
			imgMsg := msg.Message.GetImageMessage()
			content = imgMsg.GetCaption()
			messageType = "image"
			mimeType = imgMsg.GetMimetype()

			if data, err := s.client.Download(imgMsg); err == nil {
				fileName = fmt.Sprintf("sector_%d/images/%s.%s", sectorID, msg.Info.ID, utils.GetExtensionFromMime(mimeType))
				if s3URL, err := s.s3Service.UploadBytes(data, fileName, mimeType); err == nil {
					url = s3URL
				} else {
					utils.LogError("Error uploading image to S3: %v", err)
				}
			}

		case msg.Message.GetAudioMessage() != nil:
			audioMsg := msg.Message.GetAudioMessage()
			messageType = "audio"
			mimeType = audioMsg.GetMimetype()

			if data, err := s.client.Download(audioMsg); err == nil {
				fileName = fmt.Sprintf("sector_%d/audios/%s.%s", sectorID, msg.Info.ID, utils.GetExtensionFromMime(mimeType))
				if s3URL, err := s.s3Service.UploadBytes(data, fileName, mimeType); err == nil {
					url = s3URL
				} else {
					utils.LogError("Error uploading audio to S3: %v", err)
				}
			}

		case msg.Message.GetDocumentMessage() != nil:
			docMsg := msg.Message.GetDocumentMessage()
			messageType = "document"
			fileName = docMsg.GetFileName()
			mimeType = docMsg.GetMimetype()

			if data, err := s.client.Download(docMsg); err == nil {
				s3FileName := fmt.Sprintf("sector_%d/documents/%s_%s", sectorID, msg.Info.ID, fileName)
				if s3URL, err := s.s3Service.UploadBytes(data, s3FileName, mimeType); err == nil {
					url = s3URL
				} else {
					utils.LogError("Error uploading document to S3: %v", err)
				}
			}

		case msg.Message.GetVideoMessage() != nil:
			vidMsg := msg.Message.GetVideoMessage()
			content = vidMsg.GetCaption()
			messageType = "video"
			mimeType = vidMsg.GetMimetype()

			if data, err := s.client.Download(vidMsg); err == nil {
				fileName = fmt.Sprintf("sector_%d/videos/%s.%s", sectorID, msg.Info.ID, utils.GetExtensionFromMime(mimeType))
				if s3URL, err := s.s3Service.UploadBytes(data, fileName, mimeType); err == nil {
					url = s3URL
				} else {
					utils.LogError("Error uploading video to S3: %v", err)
				}
			}

		case msg.Message.ListMessage != nil:
			messageType = "list"
			content = msg.Message.ListMessage.GetDescription()

		case msg.Message.ButtonsMessage != nil:
			messageType = "buttons"
			content = msg.Message.ButtonsMessage.GetContentText()

		case msg.Message.TemplateMessage != nil:
			messageType = "template"
			if msg.Message.TemplateMessage.HydratedTemplate != nil {
				content = msg.Message.TemplateMessage.HydratedTemplate.GetHydratedContentText()

				if imageMsg := msg.Message.TemplateMessage.HydratedTemplate.GetImageMessage(); imageMsg != nil {
					messageType = "image"
					mimeType = imageMsg.GetMimetype()
					content = imageMsg.GetCaption()

					if data, err := s.client.Download(imageMsg); err == nil {
						fileName = fmt.Sprintf("sector_%d/images/%s.%s", sectorID, msg.Info.ID, utils.GetExtensionFromMime(mimeType))
						if s3URL, err := s.s3Service.UploadBytes(data, fileName, mimeType); err == nil {
							url = s3URL
						}
					}
				}
			}

		case msg.Message.GetStickerMessage() != nil:
			stickerMsg := msg.Message.GetStickerMessage()
			messageType = "sticker"
			mimeType = stickerMsg.GetMimetype()

			if data, err := s.client.Download(stickerMsg); err == nil {
				fileName = fmt.Sprintf("sector_%d/stickers/%s.webp", sectorID, msg.Info.ID)
				if s3URL, err := s.s3Service.UploadBytes(data, fileName, mimeType); err == nil {
					url = s3URL
				}
			}

		case msg.Message.ExtendedTextMessage != nil:
			messageType = "text"
			content = msg.Message.ExtendedTextMessage.GetText()

		default:
			utils.LogDebug("Ignorando tipo de mensagem não mapeado: %+v", msg.Message)
			return
		}

		brasiliaLoc, err := time.LoadLocation("America/Sao_Paulo")
		if err != nil {
			utils.LogError("Error loading Brasilia timezone: %v", err)
			brasiliaLoc = time.UTC
		}

		message := &models.Message{
			Conteudo:          content,
			Tipo:              messageType,
			URL:               url,
			NomeArquivo:       fileName,
			MimeType:          mimeType,
			IDSetor:           sectorID,
			ContatoID:         int64(contact.ID),
			DataEnvio:         time.Now().In(brasiliaLoc),
			Enviado:           false,
			Lido:              false,
			WhatsAppMessageID: msg.Info.ID,
			IsOfficial:        false,
			CreatedAt:         time.Now().In(brasiliaLoc),
		}

		err = s.messageRepository.Save(message)
		if err != nil {
			utils.LogError("Error saving received message: %v", err)
		}

		// Enviar evento WebSocket para o front apenas para mensagens recebidas
		var mediaUrlPtr *string
		if url != "" {
			mediaUrlPtr = &url
		}
		var fileNamePtr *string
		if fileName != "" {
			fileNamePtr = &fileName
		}
		var mimeTypePtr *string
		if mimeType != "" {
			mimeTypePtr = &mimeType
		}
		isSent := false
		isRead := false

		// Status da mensagem para o front
		messageStatus := models.StatusReceived // Mensagens recebidas de clientes mostram duas barras

		wsnotify.SendMessageEvent(
			message.ID,
			int(message.ContatoID),
			message.IDSetor,
			message.Conteudo,
			message.Tipo,
			mediaUrlPtr,
			fileNamePtr,
			mimeTypePtr,
			message.DataEnvio,
			isSent,
			isRead,
			messageStatus,
		)

		// Enviar lista completa de contatos para atualizar a ordenação no frontend
		go s.sendContactsList(sectorID)
	}
}

// Novo método para enviar a lista completa de contatos via WebSocket
func (s *WhatsAppService) sendContactsList(sectorID int) {
	// Buscar todos os contatos do setor
	contacts, err := s.contactRepository.GetBySector(sectorID)
	if err != nil {
		utils.LogError("Erro ao buscar lista de contatos para enviar via WebSocket: %v", err)
		return
	}

	// Enviar evento com a lista completa de contatos
	wsnotify.SendContactsList(sectorID, contacts)
}

// Função para buscar e atualizar informações de contato
func (s *WhatsAppService) fetchContactInfo(sectorID int, jid string) {
	if s.client == nil || !s.client.IsConnected() {
		utils.LogError("Cliente não conectado ao tentar buscar informações de contato")
		return
	}

	normalizedJID := jid
	if parts := strings.Split(jid, ":"); len(parts) > 1 {
		normalizedJID = parts[0] + "@" + strings.Split(parts[1], "@")[1]
	}

	normalizedJID = strings.TrimSuffix(normalizedJID, "@s.whatsapp.net")
	if !strings.Contains(normalizedJID, "@") {
		normalizedJID = normalizedJID + "@s.whatsapp.net"
	}

	contact, err := s.contactRepository.GetByNumber(sectorID, normalizedJID)
	if err != nil {
		utils.LogError("Erro ao buscar contato no banco: %v", err)
		return
	}

	if contact == nil {
		utils.LogInfo("Criando contato que não existe no banco: %s", normalizedJID)
		contact, err = s.contactRepository.CreateIfNotExists(sectorID, normalizedJID)
		if err != nil || contact == nil {
			utils.LogError("Erro ao criar contato: %v", err)
			return
		}
	}

	phoneNumber := strings.TrimSuffix(normalizedJID, "@s.whatsapp.net")
	parsedJID := types.JID{
		User:   phoneNumber,
		Server: "s.whatsapp.net",
	}

	updated := false

	contactInfo, err := s.client.Store.Contacts.GetContact(parsedJID)
	if err == nil && contactInfo.PushName != "" &&
		(contact.Name == "" || contact.Name == normalizedJID ||
			strings.HasPrefix(contact.Name, "Contato ") ||
			contact.Name != contactInfo.PushName) {
		contact.Name = contactInfo.PushName
		updated = true
		utils.LogInfo("Nome do contato atualizado: %s", contactInfo.PushName)
	}

	if contact.AvatarURL == "" {
		profilePic, err := s.client.GetProfilePictureInfo(parsedJID, &whatsmeow.GetProfilePictureParams{})
		if err == nil && profilePic != nil && profilePic.URL != "" {
			resp, err := http.Get(profilePic.URL)
			if err == nil {
				defer resp.Body.Close()
				if picData, err := io.ReadAll(resp.Body); err == nil {
					s3FileName := fmt.Sprintf("sector_%d/avatars/%s.jpg", sectorID, phoneNumber)
					if s3URL, err := s.s3Service.UploadBytes(picData, s3FileName, "image/jpeg"); err == nil {
						contact.AvatarURL = s3URL
						updated = true
						utils.LogInfo("URL do avatar atualizada: %s", s3URL)
					} else {
						utils.LogError("Erro ao fazer upload do avatar para S3: %v", err)
					}
				}
			}
		} else if err != nil {
			utils.LogDebug("Não foi possível obter foto de perfil: %v", err)
		}
	}

	if updated {
		err = s.contactRepository.Update(contact)
		if err != nil {
			utils.LogError("Erro ao atualizar informações do contato: %v", err)
		} else {
			utils.LogInfo("Contato atualizado com sucesso: %s (%s)", contact.Name, normalizedJID)

			// Enviar uma única atualização de status WS após a atualização
			go func() {
				time.Sleep(500 * time.Millisecond)
				err := s.contactRepository.SendUnreadStatusUpdate(sectorID)
				if err != nil {
					utils.LogError("Erro ao enviar atualização de status após fetchContactInfo: %v", err)
				}
			}()

			// Enviar evento do contato que foi atualizado
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
	}
}

func (s *WhatsAppService) NewConnection(sectorID string) (*whatsmeow.Client, error) {
	deviceStore, err := sqlstore.New("sqlite", fmt.Sprintf("file:%s?_pragma=foreign_keys(1)", s.getDBPath()), nil)
	if err != nil {
		return nil, fmt.Errorf("error creating device store: %v", err)
	}
	clientLog := waLog.Stdout("Client", "DEBUG", true)
	client := whatsmeow.NewClient(deviceStore.NewDevice(), clientLog)
	client.AddEventHandler(s.handleMessage)

	err = client.Connect()
	if err != nil {
		return nil, fmt.Errorf("error connecting whatsapp: %v", err)
	}

	return client, nil
}
