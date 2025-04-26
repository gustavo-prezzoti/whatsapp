package models

import (
	"encoding/json"
	"net/http"
	"time"
)

type APIResponse struct {
	Status    string      `json:"status"`
	Message   string      `json:"message"`
	Data      interface{} `json:"data,omitempty"`
	Timestamp time.Time   `json:"-"`
}

func (r *APIResponse) MarshalJSON() ([]byte, error) {
	type Alias APIResponse
	return json.Marshal(&struct {
		*Alias
		Timestamp string `json:"timestamp"`
	}{
		Alias:     (*Alias)(r),
		Timestamp: r.Timestamp.Format("2006-01-02T15:04:05Z07:00"),
	})
}

func NewSuccessResponse(message string, data interface{}) *APIResponse {
	return &APIResponse{
		Status:    "success",
		Message:   message,
		Data:      data,
		Timestamp: time.Now().UTC(),
	}
}

func NewErrorResponse(message string) *APIResponse {
	return &APIResponse{
		Status:    "error",
		Message:   message,
		Timestamp: time.Now().UTC(),
	}
}

func NewWaitingResponse(message string) *APIResponse {
	return &APIResponse{
		Status:    "waiting",
		Message:   message,
		Timestamp: time.Now().UTC(),
	}
}

func RespondWithJSON(w http.ResponseWriter, statusCode int, response *APIResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(response)
}
