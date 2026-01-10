package models

import (
	"errors"
	"html"
	"strings"
	"time"

	"gorm.io/gorm"
)

type Bracket struct {
	ID          uint   `gorm:"primary_key;autoIncrement" json:"id"`
	Title       string `gorm:"size:255;not null" json:"title"`
	Description string `gorm:"text" json:"description"`

	Author   User `gorm:"constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
	AuthorID uint `gorm:"not null;index" json:"author_id"`

	Size         int    `gorm:"not null" json:"size"`
	Status       string `gorm:"size:20;not null;default:'draft'" json:"status"`
	CurrentRound int    `gorm:"default:1" json:"current_round"`

	CreatedAt time.Time `gorm:"default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt time.Time `gorm:"default:CURRENT_TIMESTAMP" json:"updated_at"`
}

//
// ===============================
// PREPARE & VALIDATE
// ===============================
//

func (b *Bracket) Prepare() {
	b.Title = html.EscapeString(strings.TrimSpace(b.Title))
	b.Description = html.EscapeString(strings.TrimSpace(b.Description))
	b.Author = User{}
	b.CreatedAt = time.Now()
	b.UpdatedAt = time.Now()

	if b.Status == "" {
		b.Status = "draft"
	}
	if b.CurrentRound == 0 {
		b.CurrentRound = 1
	}
}

func (b *Bracket) Validate() map[string]string {
	var err error
	errorsMap := make(map[string]string)

	if b.Title == "" {
		err = errors.New("required title")
		errorsMap["Required_title"] = err.Error()
	}
	if b.AuthorID == 0 {
		err = errors.New("required author")
		errorsMap["Required_author"] = err.Error()
	}
	if b.Status == "" {
		err = errors.New("required status")
		errorsMap["Required_status"] = err.Error()
	}

	return errorsMap
}

//
// ===============================
// DATABASE OPERATIONS
// ===============================
//

// SaveBracket creates a new bracket
func (b *Bracket) SaveBracket(db *gorm.DB) (*Bracket, error) {
	if err := db.Create(b).Error; err != nil {
		return nil, err
	}

	// Load author association
	if err := db.Model(b).Association("Author").Find(&b.Author); err != nil {
		return nil, err
	}

	return b, nil
}

// FindBracketByID retrieves a bracket by ID
func (b *Bracket) FindBracketByID(db *gorm.DB, id uint) (*Bracket, error) {
	err := db.Preload("Author").Where("id = ?", id).First(&b).Error
	if err != nil {
		return nil, err
	}
	return b, nil
}

// FindUserBrackets retrieves all brackets created by a user
func (b *Bracket) FindUserBrackets(db *gorm.DB, uid uint) (*[]Bracket, error) {
	var brackets []Bracket
	err := db.
		Preload("Author").
		Where("author_id = ?", uid).
		Order("created_at DESC").
		Find(&brackets).Error

	if err != nil {
		return nil, err
	}
	return &brackets, nil
}

// UpdateBracket updates editable bracket fields
func (b *Bracket) UpdateBracket(db *gorm.DB) (*Bracket, error) {
	b.UpdatedAt = time.Now()

	err := db.Model(&Bracket{}).
		Where("id = ?", b.ID).
		Updates(map[string]interface{}{
			"title":         b.Title,
			"description":   b.Description,
			"status":        b.Status,
			"current_round": b.CurrentRound,
			"updated_at":    b.UpdatedAt,
		}).Error

	if err != nil {
		return nil, err
	}

	// Reload author
	if err := db.Model(&User{}).Where("id = ?", b.AuthorID).Take(&b.Author).Error; err != nil {
		return nil, err
	}

	return b, nil
}

// DeleteBracket deletes a bracket by ID
func (b *Bracket) DeleteBracket(db *gorm.DB, id uint) (int64, error) {
	result := db.Delete(&Bracket{}, id)
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}
