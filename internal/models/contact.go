package models

import "time"

type Contact struct {
	ID            int       `json:"id"`
	Name          string    `json:"name"`
	Number        string    `json:"number"`
	AvatarURL     string    `json:"avatar_url"`
	SectorID      int       `json:"sector_id"`
	TagID         int       `json:"tag_id"`
	IsActive      bool      `json:"is_active"`
	Email         string    `json:"email"`
	Notes         string    `json:"notes"`
	AIActive      bool      `json:"ai_active"`
	AssignedTo    int       `json:"assigned_to"`
	Priority      string    `json:"priority"`
	ContactStatus string    `json:"contact_status"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	IsOfficial    bool      `json:"is_official"`
	IsViewed      bool      `json:"is_viewed"`
}

type ContactRepository interface {
	Save(contact *Contact) error
	GetByNumber(sectorID int, number string) (*Contact, error)
	GetBySector(sectorID int) ([]*Contact, error)
	Update(contact *Contact) error
	CreateIfNotExists(sectorID int, number string) (*Contact, error)
	SetViewed(sectorID int, number string) error
	SetUnviewed(sectorID int, number string) error
	GetViewedStatus(sectorID int) (map[int]bool, error)
}
