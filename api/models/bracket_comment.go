package models

import (
	"html"
	"strings"
	"time"

	"gorm.io/gorm"
)

type BracketComment struct {
	ID        uint      `gorm:"primary_key;autoIncrement" json:"id"`
	UserID    uint      `gorm:"not null" json:"user_id"`
	BracketID uint      `gorm:"not null" json:"bracket_id"`
	Author    User      `gorm:"foreignKey:UserID" json:"author"`
	Body      string    `gorm:"text;not null;" json:"body"`
	CreatedAt time.Time `gorm:"default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt time.Time `gorm:"default:CURRENT_TIMESTAMP" json:"updated_at"`
}

func (c *BracketComment) Prepare() {
	c.ID = 0
	c.Body = html.EscapeString(strings.TrimSpace(c.Body))
	c.Author = User{}
	c.CreatedAt = time.Now()
	c.UpdatedAt = time.Now()
}

func (comment *BracketComment) Validate(action string) map[string]string {
	errorMessages := make(map[string]string)

	if comment.Body == "" {
		errorMessages["Required_body"] = "Body is required"
	}
	if comment.UserID == 0 {
		errorMessages["Required_user"] = "User is required"
	}
	if comment.BracketID == 0 {
		errorMessages["Required_bracket"] = "Bracket is required"
	}
	return errorMessages
}

func (c *BracketComment) SaveComment(db *gorm.DB) (*BracketComment, error) {
	if err := db.Create(&c).Error; err != nil {
		return nil, err
	}
	return c, nil
}

func (c *BracketComment) GetComments(db *gorm.DB, bid uint) (*[]BracketComment, error) {
	comments := []BracketComment{}
	err := db.Preload("Author").Where("bracket_id = ?", bid).
		Order("created_at desc").Find(&comments).Error
	if err != nil {
		return nil, err
	}
	return &comments, nil
}

func (c *BracketComment) DeleteAComment(db *gorm.DB) (int64, error) {
	result := db.Where("id = ?", c.ID).Delete(&BracketComment{})
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}
