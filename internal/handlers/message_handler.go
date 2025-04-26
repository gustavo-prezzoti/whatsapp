package handlers

import (
	"fmt"

	"github.com/Rhymen/go-whatsapp"
)

type MessageHandler struct {
	whatsapp.Handler
}

func NewMessageHandler() *MessageHandler {
	return &MessageHandler{}
}

func (h *MessageHandler) HandleTextMessage(message whatsapp.TextMessage) {
	fmt.Printf("Message from %s: %s\n", message.Info.RemoteJid, message.Text)
}

func (h *MessageHandler) HandleError(err error) {
	fmt.Printf("Error occurred: %v\n", err)
}
