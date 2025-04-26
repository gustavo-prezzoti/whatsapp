package models

type WebSocketEvent struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}
