package models

type MessageRequest struct {
	SectorID    int    `json:"sector_id" example:"1" swagger:"required" description:"ID do setor"`
	Recipient   string `json:"recipient" example:"5511999999999" swagger:"required" description:"Número do telefone no formato DDDNúmero"`
	Message     string `json:"message" example:"Olá, como vai?" swagger:"required" description:"Texto da mensagem"`
	UserID      *int   `json:"userId"`
	IsAnonymous bool   `json:"isAnonymous"`
}

type ImageMessageRequest struct {
	SectorID  int    `json:"sector_id"`
	Recipient string `json:"recipient"`
	ImagePath string `json:"image_path"`
	Caption   string `json:"caption"`
}

type AudioMessageRequest struct {
	SectorID  int    `json:"sector_id"`
	Recipient string `json:"recipient"`
	AudioPath string `json:"audio_path"`
}

type DocumentMessageRequest struct {
	SectorID  int    `json:"sector_id"`
	Recipient string `json:"recipient"`
	FilePath  string `json:"file_path"`
}

type TypingRequest struct {
	SectorID  int    `json:"sector_id" example:"1" swagger:"required" description:"ID do setor"`
	Recipient string `json:"recipient" example:"5511999999999" swagger:"required" description:"Número do telefone no formato DDDNúmero"`
	Duration  int    `json:"duration" example:"5" default:"5" description:"Duração em segundos da indicação de digitação"`
}

type MediaMessageRequest struct {
	Base64File  string `json:"base64File"`
	MediaType   string `json:"mediaType"`
	FileName    string `json:"fileName"`
	Caption     string `json:"caption"`
	Recipient   string `json:"recipient"`
	ContactID   int    `json:"contactId"`
	SectorID    int    `json:"sectorId"`
	UserID      *int   `json:"userId"`
	IsAnonymous bool   `json:"isAnonymous"`
}
