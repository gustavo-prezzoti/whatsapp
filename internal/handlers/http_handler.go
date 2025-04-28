package handlers

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"whatsapp-bot/config"
	"whatsapp-bot/internal/models"
	"whatsapp-bot/internal/repositories"
	"whatsapp-bot/internal/services"
	"whatsapp-bot/internal/utils"
)

type HTTPHandler struct {
	connectionManager *services.ConnectionManager
	s3Service         *services.S3Service
	contactRepository *repositories.MySQLContactRepository
}

func NewHTTPHandler(manager *services.ConnectionManager) *HTTPHandler {
	s3Service, err := services.NewS3Service(config.NewConfig().S3Config)
	if err != nil {
		utils.LogError("Erro ao criar serviço S3: %v", err)
	}

	return &HTTPHandler{
		connectionManager: manager,
		s3Service:         s3Service,
		contactRepository: repositories.NewMySQLContactRepository(manager.GetDB()),
	}
}

// @Summary Upload a file
// @Description Upload a file to be sent via WhatsApp
// @Tags upload
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "File to upload"
// @Success 200 {object} models.APIResponse
// @Failure 400 {object} models.APIResponse
// @Router /upload [post]
func (h *HTTPHandler) HandleUpload(w http.ResponseWriter, r *http.Request) {
	if h.s3Service == nil {
		utils.LogError("Serviço S3 não está disponível em /upload")
		models.RespondWithJSON(w, http.StatusInternalServerError,
			models.NewErrorResponse("Serviço S3 não está disponível"))
		return
	}

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		utils.LogError("Arquivo muito grande em /upload: %v", err)
		models.RespondWithJSON(w, http.StatusBadRequest,
			models.NewErrorResponse("Arquivo muito grande. Limite de 10MB"))
		return
	}

	file, handler, err := r.FormFile("file")
	if err != nil {
		utils.LogError("Erro ao processar arquivo em /upload: %v", err)
		models.RespondWithJSON(w, http.StatusBadRequest,
			models.NewErrorResponse("Erro ao processar arquivo"))
		return
	}
	defer file.Close()

	fileUrl, err := h.s3Service.UploadFile(file, handler)
	if err != nil {
		utils.LogError("Erro ao fazer upload em /upload: %v", err)
		models.RespondWithJSON(w, http.StatusInternalServerError,
			models.NewErrorResponse(fmt.Sprintf("Erro ao fazer upload: %v", err)))
		return
	}

	response := map[string]string{
		"path": fileUrl,
	}
	models.RespondWithJSON(w, http.StatusOK,
		models.NewSuccessResponse("Arquivo enviado com sucesso", response))
}

// @Summary Send a text message
// @Description Send a text message to a WhatsApp contact
// @Tags messages
// @Accept json
// @Produce json
// @Param request body models.MessageRequest true "Message details"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Router /send-message [post]
func (h *HTTPHandler) SendMessage(w http.ResponseWriter, r *http.Request) {
	var req models.MessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.LogError("Erro ao decodificar requisição /send-message: %v", err)
		models.RespondWithJSON(w, http.StatusBadRequest, models.NewErrorResponse("Erro ao decodificar requisição: "+err.Error()))
		return
	}

	service, err := h.connectionManager.GetConnection(req.SectorID)
	if err != nil {
		utils.LogError("Erro ao obter conexão no /send-message: %v", err)
		models.RespondWithJSON(w, http.StatusBadRequest, models.NewErrorResponse(err.Error()))
		return
	}

	err = service.SendMessage(req.SectorID, req.Recipient, req.Message, req.UserID, req.IsAnonymous)
	if err != nil {
		utils.LogError("Erro ao enviar mensagem no /send-message: %v", err)
		models.RespondWithJSON(w, http.StatusInternalServerError, models.NewErrorResponse("Erro ao enviar mensagem: "+err.Error()))
		return
	}

	data := map[string]interface{}{
		"recipient": req.Recipient,
		"message":   req.Message,
	}
	models.RespondWithJSON(w, http.StatusOK, models.NewSuccessResponse("Mensagem enviada com sucesso", data))
}

// @Summary Send an image
// @Description Send an image with optional caption to a WhatsApp contact
// @Tags messages
// @Accept json
// @Produce json
// @Param request body models.MediaMessageRequest true "Image message details"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Router /send-image [post]
func (h *HTTPHandler) SendImage(w http.ResponseWriter, r *http.Request) {
	var req models.MediaMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.LogError("Erro ao decodificar requisição /send-image: %v", err)
		models.RespondWithJSON(w, http.StatusBadRequest, models.NewErrorResponse("Erro ao decodificar requisição: "+err.Error()))
		return
	}

	service, err := h.connectionManager.GetConnection(req.SectorID)
	if err != nil {
		utils.LogError("Erro ao obter conexão no /send-image: %v", err)
		models.RespondWithJSON(w, http.StatusBadRequest, models.NewErrorResponse(err.Error()))
		return
	}

	base64Data := req.Base64File
	if i := strings.Index(base64Data, ";base64,"); i > -1 {
		base64Data = base64Data[i+8:]
	}

	imageBytes, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		utils.LogError("Erro ao decodificar base64 em /send-image: %v", err)
		models.RespondWithJSON(w, http.StatusBadRequest, models.NewErrorResponse("Erro ao decodificar base64: "+err.Error()))
		return
	}

	err = service.SendImage(req.SectorID, req.Recipient, imageBytes, req.Caption, req.UserID, req.IsAnonymous)
	if err != nil {
		utils.LogError("Erro ao enviar imagem em /send-image: %v", err)
		models.RespondWithJSON(w, http.StatusInternalServerError, models.NewErrorResponse("Erro ao enviar imagem: "+err.Error()))
		return
	}

	data := map[string]interface{}{
		"recipient": req.Recipient,
		"fileName":  req.FileName,
		"mediaType": req.MediaType,
	}
	models.RespondWithJSON(w, http.StatusOK, models.NewSuccessResponse("Imagem enviada com sucesso", data))
}

// @Summary Send an audio file
// @Description Send an audio file to a WhatsApp contact
// @Tags messages
// @Accept json
// @Produce json
// @Param request body models.MediaMessageRequest true "Audio message details"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Router /send-audio [post]
func (h *HTTPHandler) SendAudio(w http.ResponseWriter, r *http.Request) {
	var req models.MediaMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.LogError("Erro ao decodificar requisição /send-audio: %v", err)
		models.RespondWithJSON(w, http.StatusBadRequest, models.NewErrorResponse("Erro ao decodificar requisição: "+err.Error()))
		return
	}

	service, err := h.connectionManager.GetConnection(req.SectorID)
	if err != nil {
		utils.LogError("Erro ao obter conexão no /send-audio: %v", err)
		models.RespondWithJSON(w, http.StatusBadRequest, models.NewErrorResponse(err.Error()))
		return
	}

	base64Data := req.Base64File
	if i := strings.Index(base64Data, ";base64,"); i > -1 {
		base64Data = base64Data[i+8:]
	}

	audioBytes, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		utils.LogError("Erro ao decodificar base64 em /send-audio: %v", err)
		models.RespondWithJSON(w, http.StatusBadRequest, models.NewErrorResponse("Erro ao decodificar base64: "+err.Error()))
		return
	}

	err = service.SendAudio(req.SectorID, req.Recipient, audioBytes, req.UserID, req.IsAnonymous)
	if err != nil {
		utils.LogError("Erro ao enviar áudio em /send-audio: %v", err)
		models.RespondWithJSON(w, http.StatusInternalServerError, models.NewErrorResponse("Erro ao enviar áudio: "+err.Error()))
		return
	}

	data := map[string]interface{}{
		"recipient": req.Recipient,
		"fileName":  req.FileName,
		"mediaType": req.MediaType,
	}
	models.RespondWithJSON(w, http.StatusOK, models.NewSuccessResponse("Áudio enviado com sucesso", data))
}

// @Summary Send a document
// @Description Send a document file to a WhatsApp contact
// @Tags messages
// @Accept json
// @Produce json
// @Param request body models.MediaMessageRequest true "Document message details"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Router /send-document [post]
func (h *HTTPHandler) SendDocument(w http.ResponseWriter, r *http.Request) {
	var req models.MediaMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.LogError("Erro ao decodificar requisição /send-document: %v", err)
		models.RespondWithJSON(w, http.StatusBadRequest, models.NewErrorResponse("Erro ao decodificar requisição: "+err.Error()))
		return
	}

	service, err := h.connectionManager.GetConnection(req.SectorID)
	if err != nil {
		utils.LogError("Erro ao obter conexão no /send-document: %v", err)
		models.RespondWithJSON(w, http.StatusBadRequest, models.NewErrorResponse(err.Error()))
		return
	}

	base64Data := req.Base64File
	if i := strings.Index(base64Data, ";base64,"); i > -1 {
		base64Data = base64Data[i+8:]
	}

	fileBytes, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		utils.LogError("Erro ao decodificar base64 em /send-document: %v", err)
		models.RespondWithJSON(w, http.StatusBadRequest, models.NewErrorResponse("Erro ao decodificar base64: "+err.Error()))
		return
	}

	utils.LogInfo("Enviando documento: %s (%d bytes)", req.FileName, len(fileBytes))

	err = service.SendDocument(req.SectorID, req.Recipient, fileBytes, req.FileName, req.UserID, req.IsAnonymous)
	if err != nil {
		utils.LogError("Erro ao enviar documento em /send-document: %v", err)
		models.RespondWithJSON(w, http.StatusInternalServerError, models.NewErrorResponse("Erro ao enviar documento: "+err.Error()))
		return
	}

	data := map[string]interface{}{
		"recipient": req.Recipient,
		"fileName":  req.FileName,
		"mediaType": req.MediaType,
		"size":      len(fileBytes),
	}
	models.RespondWithJSON(w, http.StatusOK, models.NewSuccessResponse("Documento enviado com sucesso", data))
}

// @Summary Send typing indication
// @Description Send a typing indication to a WhatsApp contact with specified duration
// @Tags messages
// @Accept json
// @Produce json
// @Param request body models.TypingRequest true "Typing indication details with duration in seconds"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Router /send-typing [post]
func (h *HTTPHandler) SendTyping(w http.ResponseWriter, r *http.Request) {
	var req models.TypingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.LogError("Erro ao decodificar requisição /send-typing: %v", err)
		models.RespondWithJSON(w, http.StatusBadRequest, models.NewErrorResponse("Erro ao decodificar requisição: "+err.Error()))
		return
	}

	service, err := h.connectionManager.GetConnection(req.SectorID)
	if err != nil {
		utils.LogError("Erro ao obter conexão no /send-typing: %v", err)
		models.RespondWithJSON(w, http.StatusBadRequest, models.NewErrorResponse(err.Error()))
		return
	}

	err = service.SendTyping(req.Recipient, req.Duration)
	if err != nil {
		utils.LogError("Erro ao enviar status de digitação em /send-typing: %v", err)
		models.RespondWithJSON(w, http.StatusInternalServerError, models.NewErrorResponse("Erro ao enviar status de digitação: "+err.Error()))
		return
	}

	data := map[string]interface{}{
		"recipient": req.Recipient,
		"duration":  req.Duration,
	}
	models.RespondWithJSON(w, http.StatusOK, models.NewSuccessResponse("Status de digitação enviado com sucesso", data))
}

// @Summary Get QR Code
// @Description Get QR code as PNG image for WhatsApp login
// @Tags authentication
// @Produce json
// @Param sector_id query int true "ID do setor para gerar o QR code" minimum(1)
// @Success 200 {object} models.APIResponse "QR code em base64"
// @Failure 400 {object} models.APIResponse "Erro de requisição inválida"
// @Failure 404 {object} models.APIResponse "QR code não encontrado"
// @Router /qrcode [get]
func (h *HTTPHandler) GetQRCode(w http.ResponseWriter, r *http.Request) {
	sectorID := r.URL.Query().Get("sector_id")
	if sectorID == "" {
		utils.LogError("sector_id não informado em /qrcode")
		models.RespondWithJSON(w, http.StatusBadRequest, models.NewErrorResponse("Por favor, informe o ID do setor para gerar o QR Code"))
		return
	}

	var id int
	if _, err := fmt.Sscanf(sectorID, "%d", &id); err != nil {
		utils.LogError("ID do setor inválido em /qrcode: %v", err)
		models.RespondWithJSON(w, http.StatusBadRequest, models.NewErrorResponse("O ID do setor deve ser um número válido"))
		return
	}

	status, err := h.connectionManager.GetConnectionStatus(id)
	if err != nil {
		utils.LogError("Erro ao obter status da conexão em /qrcode: %v", err)
		models.RespondWithJSON(w, http.StatusBadRequest, models.NewErrorResponse("Não foi possível verificar o status do setor. Por favor, tente novamente."))
		return
	}

	if status == "connected" {
		data := map[string]interface{}{
			"status":  "connected",
			"message": "O WhatsApp já está conectado e pronto para uso neste setor!",
		}
		models.RespondWithJSON(w, http.StatusOK, models.NewSuccessResponse("WhatsApp conectado com sucesso", data))
		return
	}

	_, err = h.connectionManager.GetConnection(id)
	if err != nil {
		utils.LogError("Erro ao estabelecer conexão em /qrcode: %v", err)
		models.RespondWithJSON(w, http.StatusBadRequest, models.NewErrorResponse("Não foi possível estabelecer uma conexão com o WhatsApp. Por favor, tente novamente em alguns instantes."))
		return
	}

	qrCode, err := h.connectionManager.GetQRCode(id)
	if err != nil {
		utils.LogError("Erro ao obter QR Code em /qrcode: %v", err)
		models.RespondWithJSON(w, http.StatusNotFound, models.NewErrorResponse("O QR Code ainda não está disponível. Por favor, aguarde alguns segundos e tente novamente."))
		return
	}

	instructions := []string{
		"Para conectar seu WhatsApp, siga os passos abaixo:",
		"1. Abra o WhatsApp no seu celular",
		"2. Toque em Menu (três pontos) ou Configurações",
		"3. Selecione 'Aparelhos conectados'",
		"4. Toque em 'Conectar um aparelho'",
		"5. Aponte a câmera do seu celular para este QR Code",
		"",
		"Importante:",
		"• Mantenha seu celular conectado à internet",
		"• O QR Code expira em poucos minutos",
		"• Se expirar, atualize a página para gerar um novo",
		"• Você pode usar o mesmo WhatsApp em até 4 aparelhos",
	}

	data := map[string]interface{}{
		"qrcode":       qrCode,
		"instructions": strings.Join(instructions, "\n"),
	}

	models.RespondWithJSON(w, http.StatusOK, models.NewSuccessResponse("QR Code gerado com sucesso", data))
}

// @Summary Get QR Code Base64
// @Description Get QR code as base64 string for WhatsApp login
// @Tags authentication
// @Produce json
// @Param sector_id query int true "ID do setor para gerar o QR code" minimum(1)
// @Success 200 {object} models.APIResponse "QR code em base64 e status"
// @Failure 400 {object} models.APIResponse "Erro de requisição inválida"
// @Failure 404 {object} models.APIResponse "QR code não encontrado"
// @Router /qrcode-base64 [get]
func (h *HTTPHandler) GetQRCodeBase64(w http.ResponseWriter, r *http.Request) {
	sectorID := r.URL.Query().Get("sector_id")
	if sectorID == "" {
		utils.LogError("sector_id não informado em /qrcode-base64")
		models.RespondWithJSON(w, http.StatusBadRequest, models.NewErrorResponse("Por favor, informe o ID do setor para gerar o QR Code"))
		return
	}

	var id int
	if _, err := fmt.Sscanf(sectorID, "%d", &id); err != nil {
		utils.LogError("ID do setor inválido em /qrcode-base64: %v", err)
		models.RespondWithJSON(w, http.StatusBadRequest, models.NewErrorResponse("O ID do setor deve ser um número válido"))
		return
	}

	status, err := h.connectionManager.GetConnectionStatus(id)
	if err != nil {
		utils.LogError("Erro ao obter status da conexão em /qrcode-base64: %v", err)
		models.RespondWithJSON(w, http.StatusBadRequest, models.NewErrorResponse("Não foi possível verificar o status do setor. Por favor, tente novamente."))
		return
	}

	if status == "connected" {
		data := map[string]interface{}{
			"status":  "connected",
			"message": "O WhatsApp já está conectado e pronto para uso neste setor!",
		}
		models.RespondWithJSON(w, http.StatusOK, models.NewSuccessResponse("WhatsApp conectado com sucesso", data))
		return
	}

	_, err = h.connectionManager.GetConnection(id)
	if err != nil {
		utils.LogError("Erro ao estabelecer conexão em /qrcode-base64: %v", err)
		models.RespondWithJSON(w, http.StatusBadRequest, models.NewErrorResponse("Não foi possível estabelecer uma conexão com o WhatsApp. Por favor, tente novamente em alguns instantes."))
		return
	}

	qrCode, err := h.connectionManager.GetQRCode(id)
	if err != nil {
		utils.LogError("Erro ao obter QR Code em /qrcode-base64: %v", err)
		models.RespondWithJSON(w, http.StatusNotFound, models.NewErrorResponse("O QR Code ainda não está disponível. Por favor, aguarde alguns segundos e tente novamente."))
		return
	}

	instructions := []string{
		"Para conectar seu WhatsApp, siga os passos abaixo:",
		"1. Abra o WhatsApp no seu celular",
		"2. Toque em Menu (três pontos) ou Configurações",
		"3. Selecione 'Aparelhos conectados'",
		"4. Toque em 'Conectar um aparelho'",
		"5. Aponte a câmera do seu celular para este QR Code",
		"",
		"Importante:",
		"• Mantenha seu celular conectado à internet",
		"• O QR Code expira em poucos minutos",
		"• Se expirar, atualize a página para gerar um novo",
		"• Você pode usar o mesmo WhatsApp em até 4 aparelhos",
	}

	data := map[string]interface{}{
		"qrcode":       qrCode,
		"instructions": strings.Join(instructions, "\n"),
	}

	models.RespondWithJSON(w, http.StatusOK, models.NewSuccessResponse("QR Code gerado com sucesso", data))
}

// @Summary Check Connection Status
// @Description Check if WhatsApp is connected
// @Tags authentication
// @Produce json
// @Param sector_id query int true "ID do setor para verificar o status" minimum(1)
// @Success 200 {object} map[string]interface{} "Status da conexão"
// @Failure 400 {object} map[string]string "Erro de requisição inválida"
// @Router /status [get]
func (h *HTTPHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	sectorID := r.URL.Query().Get("sector_id")
	if sectorID == "" {
		http.Error(w, "Por favor, informe o ID do setor para verificar o status", http.StatusBadRequest)
		return
	}

	var id int
	if _, err := fmt.Sscanf(sectorID, "%d", &id); err != nil {
		http.Error(w, "O ID do setor deve ser um número válido", http.StatusBadRequest)
		return
	}

	status, err := h.connectionManager.GetConnectionStatus(id)
	if err != nil {
		http.Error(w, "Não foi possível verificar o status do setor. Por favor, tente novamente.", http.StatusInternalServerError)
		return
	}

	var message string
	var connected bool

	switch status {
	case "connected":
		connected = true
		message = "O WhatsApp está conectado e pronto para enviar mensagens!"
	case "connecting":
		message = "O QR Code está pronto para ser escaneado."
	case "disconnected":
		message = "O WhatsApp está desconectado."
	case "not_found":
		message = "Nenhuma conexão encontrada para este setor. Por favor, inicie uma nova conexão."
	default:
		message = "Status desconhecido. Por favor, tente reconectar."
	}

	response := map[string]interface{}{
		"status":    status,
		"connected": connected,
		"message":   message,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Helper function to download file from URL if needed
func (h *HTTPHandler) getFileBytes(path string) ([]byte, string, error) {
	// Check if path is a URL
	if utils.IsURL(path) {
		// Download file from URL
		resp, err := http.Get(path)
		if err != nil {
			return nil, "", fmt.Errorf("erro ao baixar arquivo: %v", err)
		}
		defer resp.Body.Close()

		// Read response body
		fileBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, "", fmt.Errorf("erro ao ler resposta: %v", err)
		}

		// Extract filename from URL
		fileName := filepath.Base(path)
		return fileBytes, fileName, nil
	}

	// If path is a local file path
	fileBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("erro ao ler arquivo: %v", err)
	}

	fileName := filepath.Base(path)
	return fileBytes, fileName, nil
}

// @Summary Mark Contact as Viewed
// @Description Mark a contact's messages as viewed
// @Tags contacts
// @Accept json
// @Produce json
// @Param request body models.ViewedRequest true "Viewed request details"
// @Success 200 {object} models.APIResponse
// @Failure 400 {object} models.APIResponse
// @Router /mark-viewed [post]
func (h *HTTPHandler) MarkContactViewed(w http.ResponseWriter, r *http.Request) {
	var req models.ViewedRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.LogError("Erro ao decodificar requisição /mark-viewed: %v", err)
		models.RespondWithJSON(w, http.StatusBadRequest, models.NewErrorResponse("Erro ao decodificar requisição: "+err.Error()))
		return
	}

	if req.SectorID == 0 || req.ContactID == 0 {
		utils.LogError("SectorID e ContactID obrigatórios em /mark-viewed")
		models.RespondWithJSON(w, http.StatusBadRequest, models.NewErrorResponse("SectorID e ContactID são obrigatórios"))
		return
	}

	err := h.contactRepository.SetViewedByID(req.SectorID, req.ContactID)
	if err != nil {
		utils.LogError("Erro ao marcar contato como visualizado em /mark-viewed: %v", err)
		models.RespondWithJSON(w, http.StatusInternalServerError, models.NewErrorResponse("Erro ao marcar contato como visualizado: "+err.Error()))
		return
	}

	data := map[string]interface{}{
		"sector_id":  req.SectorID,
		"contact_id": req.ContactID,
	}
	models.RespondWithJSON(w, http.StatusOK, models.NewSuccessResponse("Contato marcado como visualizado com sucesso", data))
}

// @Summary Get Contact Viewed Status
// @Description Check if a contact's messages have been viewed
// @Tags contacts
// @Accept json
// @Produce json
// @Param request body models.ViewedRequest true "Viewed request details"
// @Success 200 {object} models.APIResponse
// @Failure 400 {object} models.APIResponse
// @Router /check-viewed [post]
func (h *HTTPHandler) CheckContactViewed(w http.ResponseWriter, r *http.Request) {
	var req models.ViewedRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.LogError("Erro ao decodificar requisição /check-viewed: %v", err)
		models.RespondWithJSON(w, http.StatusBadRequest, models.NewErrorResponse("Erro ao decodificar requisição: "+err.Error()))
		return
	}

	if req.SectorID == 0 {
		utils.LogError("SectorID obrigatório em /check-viewed")
		models.RespondWithJSON(w, http.StatusBadRequest, models.NewErrorResponse("SectorID é obrigatório"))
		return
	}

	isViewed, err := h.contactRepository.GetViewedStatus(req.SectorID)
	if err != nil {
		utils.LogError("Erro ao verificar status de visualização em /check-viewed: %v", err)
		models.RespondWithJSON(w, http.StatusInternalServerError, models.NewErrorResponse("Erro ao verificar status de visualização: "+err.Error()))
		return
	}

	data := map[string]interface{}{
		"sector_id":  req.SectorID,
		"contact_id": req.ContactID,
		"is_viewed":  isViewed,
	}
	models.RespondWithJSON(w, http.StatusOK, models.NewSuccessResponse("Status de visualização verificado com sucesso", data))
}
