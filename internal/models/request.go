package models

type ViewedRequest struct {
	SectorID  int `json:"sector_id"`
	ContactID int `json:"contact_id,omitempty"`
}
