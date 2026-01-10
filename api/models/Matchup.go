package models

import (
	"errors"
	"html"
	"strings"
	"time"

	"gorm.io/gorm"
)

type Matchup struct {
	ID              uint          `gorm:"primary_key;autoIncrement" json:"id"`
	Title           string        `gorm:"size:255;not null" json:"title"`
	Content         string        `gorm:"text;not null;" json:"content"`
	Author          User          `gorm:"constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
	AuthorID        uint          `gorm:"not null" json:"author_id"`
	Items           []MatchupItem `gorm:"foreignKey:MatchupID;constraint:OnDelete:CASCADE" json:"items"`
	Comments        []Comment     `gorm:"foreignKey:MatchupID" json:"comments"`
	CreatedAt       time.Time     `gorm:"default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt       time.Time     `gorm:"default:CURRENT_TIMESTAMP" json:"updated_at"`
	Status          string        `gorm:"size:20;not null;default:'published'" json:"status"`
	EndMode         string        `gorm:"size:20;not null;default:'timer'" json:"end_mode"`
	DurationSeconds int           `gorm:"not null;default:0" json:"duration_seconds"`
	BracketID       *uint         `gorm:"index" json:"bracket_id,omitempty"`
	Round           *int          `json:"round,omitempty"`
	Seed            *int          `json:"seed,omitempty"`
	WinnerItemID    *uint         `json:"winner_item_id"`
	StartTime       *time.Time    `json:"start_time,omitempty"`
	EndTime         *time.Time    `json:"end_time,omitempty"`
}

type MatchupItem struct {
	ID        uint    `gorm:"primary_key;autoIncrement" json:"id"`
	Matchup   Matchup `gorm:"constraint:OnDelete:CASCADE" json:"-"`
	MatchupID uint    `gorm:"not null;index" json:"matchup_id"`
	Item      string  `json:"item"`
	Votes     int     `json:"votes"`
}

func (m *Matchup) Prepare() {
	m.Title = html.EscapeString(strings.TrimSpace(m.Title))
	m.Content = html.EscapeString(strings.TrimSpace(m.Content))
	m.Author = User{}
	m.CreatedAt = time.Now()
	m.UpdatedAt = time.Now()

	if strings.TrimSpace(m.EndMode) == "" {
		m.EndMode = "timer"
	}
}

func (m *Matchup) Validate() map[string]string {
	var err error
	var errorMessages = make(map[string]string)

	if m.Title == "" {
		err = errors.New("required title")
		errorMessages["Required_title"] = err.Error()
	}
	if m.Content == "" {
		err = errors.New("required content")
		errorMessages["Required_content"] = err.Error()
	}
	if m.AuthorID == 0 {
		err = errors.New("required author")
		errorMessages["Required_author"] = err.Error()
	}
	return errorMessages
}

func (m *Matchup) SaveMatchup(db *gorm.DB) (*Matchup, error) {
	// Create the Matchup record
	if err := db.Create(m).Error; err != nil {
		return nil, err
	}

	// Save the MatchupItems
	for i, item := range m.Items {
		// If the item's ID is empty, set the MatchupID and create the item
		if item.ID == 0 {
			m.Items[i].MatchupID = m.ID // Set the MatchupID to the ID of the Matchup instance
			if err := db.Create(&m.Items[i]).Error; err != nil {
				return nil, err
			}
		}
	}

	// Load the author association after creating the matchup.
	if err := db.Model(m).Association("Author").Find(&m.Author); err != nil {
		return nil, err
	}

	// Load the items association after creating the matchup.
	if err := db.Model(m).Association("Items").Find(&m.Items); err != nil {
		return nil, err
	}

	return m, nil
}
func (m *Matchup) FindAllMatchups(db *gorm.DB) ([]Matchup, error) {
	var err error
	matchups := []Matchup{}
	err = db.Preload("Author").Preload("Items").Find(&matchups).Error
	if err != nil {
		return []Matchup{}, err
	}
	return matchups, nil
}

func (m *Matchup) FindMatchupByID(db *gorm.DB, id uint) (*Matchup, error) {
	err := db.Preload("Author").Where("id = ?", id).First(&m).Error
	if err != nil {
		return nil, err
	}
	return m, nil
}

func (m *Matchup) UpdateMatchup(db *gorm.DB) (*Matchup, error) {
	var err error
	err = db.Model(&Matchup{}).Where("id = ?", m.ID).Updates(Matchup{Title: m.Title, Content: m.Content, UpdatedAt: time.Now()}).Error
	if err != nil {
		return &Matchup{}, err
	}
	for i, item := range m.Items {
		item.MatchupID = m.ID // Set the matchup_id
		err := db.Model(&MatchupItem{}).Where("id = ?", item.ID).Save(&item).Error
		if err != nil {
			return &Matchup{}, err
		}
		m.Items[i] = item
	}

	if m.ID != 0 {
		err = db.Model(&User{}).Where("id = ?", m.AuthorID).Take(&m.Author).Error
		if err != nil {
			return &Matchup{}, err
		}
	}
	return m, nil
}

func (m *Matchup) DeleteMatchup(db *gorm.DB, id uint) (int64, error) {
	result := db.Delete(&Matchup{}, id)
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}
func (m *Matchup) FindUserMatchups(db *gorm.DB, uid uint) (*[]Matchup, error) {
	var matchups []Matchup
	result := db.Preload("Author").Preload("Items").Where("author_id = ?", uid).Limit(100).Order("created_at desc").Find(&matchups)
	if result.Error != nil {
		return nil, result.Error
	}
	return &matchups, nil
}

// When a user is deleted, we also delete the matchups that the user had
func (m *Matchup) DeleteUserMatchups(db *gorm.DB, uid uint) (int64, error) {
	result := db.Where("author_id = ?", uid).Delete(&Matchup{})
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}

func (m *MatchupItem) IncrementVotes(db *gorm.DB) error {
	// Increment votes directly and return error if any
	return db.Model(&MatchupItem{}).Where("id = ?", m.ID).UpdateColumn("votes", gorm.Expr("votes + ?", 1)).Error
}

func (m *Matchup) GetComments(db *gorm.DB) (*[]Comment, error) {
	comments := []Comment{}
	err := db.Debug().Model(&Comment{}).Where("matchup_id = ?", m.ID).Order("created_at desc").Find(&comments).Error
	if err != nil {
		return &[]Comment{}, err
	}
	return &comments, nil
}

func FindMatchupsByBracket(db *gorm.DB, bracketID uint) ([]Matchup, error) {
	var matchups []Matchup
	err := db.
		Where("bracket_id = ?", bracketID).
		Order("round ASC, seed ASC, created_at ASC").
		Preload("Items").
		Find(&matchups).Error

	return matchups, err
}

func (m *Matchup) WinnerItem() (*MatchupItem, error) {
	if m.WinnerItemID != nil {
		for _, item := range m.Items {
			if item.ID == *m.WinnerItemID {
				return &item, nil
			}
		}
		return nil, errors.New("manual winner not found")
	}

	if len(m.Items) != 2 {
		return nil, errors.New("matchup does not have exactly 2 items")
	}

	a := m.Items[0]
	b := m.Items[1]

	if a.Votes == b.Votes {
		return nil, errors.New("matchup is tied")
	}

	if a.Votes > b.Votes {
		return &a, nil
	}
	return &b, nil
}

func (m *Matchup) HasTie() bool {
	if len(m.Items) < 2 {
		return false
	}

	votes := m.Items[0].Votes
	for _, item := range m.Items[1:] {
		if item.Votes != votes {
			return false
		}
	}
	return true
}

func FindBracketRounds(db *gorm.DB, bracketID uint) (map[int][]Matchup, error) {
	matchups, err := FindMatchupsByBracket(db, bracketID)
	if err != nil {
		return nil, err
	}

	rounds := make(map[int][]Matchup)

	for _, m := range matchups {
		if m.Round == nil {
			continue
		}
		round := *m.Round
		rounds[round] = append(rounds[round], m)
	}

	return rounds, nil
}
