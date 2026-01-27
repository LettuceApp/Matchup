package models

import (
	"html"
	"strings"
	"time"

	"github.com/twinj/uuid"
	"gorm.io/gorm"
)

type Comment struct {
	ID        uint      `gorm:"primary_key;autoIncrement" json:"id"`
	PublicID  string    `gorm:"type:uuid;uniqueIndex;column:public_id" json:"public_id"`
	UserID    uint      `gorm:"not null" json:"user_id"`
	MatchupID uint      `gorm:"not null" json:"matchup_id"`
	Author    User      `gorm:"foreignKey:UserID" json:"author"`
	Body      string    `gorm:"text;not null;" json:"body"`
	CreatedAt time.Time `gorm:"default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt time.Time `gorm:"default:CURRENT_TIMESTAMP" json:"updated_at"`
}

func (c *Comment) BeforeCreate(tx *gorm.DB) (err error) {
	if strings.TrimSpace(c.PublicID) == "" {
		c.PublicID = uuid.NewV4().String()
	}
	return nil
}

func (c *Comment) Prepare() {
	c.ID = 0
	c.Body = html.EscapeString(strings.TrimSpace(c.Body))
	c.Author = User{}
	c.CreatedAt = time.Now()
	c.UpdatedAt = time.Now()
}

func (comment *Comment) Validate(action string) map[string]string {
	var errorMessages = make(map[string]string)

	if comment.Body == "" {
		errorMessages["Required_body"] = "Body is required"
	}
	if comment.UserID == 0 {
		errorMessages["Required_user"] = "User is required"
	}
	if comment.MatchupID == 0 {
		errorMessages["Required_matchup"] = "Matchup is required"
	}
	return errorMessages
}

func (c *Comment) SaveComment(db *gorm.DB) (*Comment, error) {
	err := db.Create(&c).Error
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Comment) GetComments(db *gorm.DB, mid uint) (*[]Comment, error) {
	comments := []Comment{}
	// Preload the comment author's information so the username is available
	err := db.Preload("Author").Where("matchup_id = ?", mid).
		Order("created_at desc").Find(&comments).Error
	if err != nil {
		return nil, err
	}
	return &comments, nil
}

func (c *Comment) UpdateAComment(db *gorm.DB) (*Comment, error) {
	err := db.Model(&Comment{}).Where("id = ?", c.ID).Updates(map[string]interface{}{"body": c.Body, "updated_at": time.Now()}).Error
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Comment) DeleteAComment(db *gorm.DB) (int64, error) {
	result := db.Where("id = ?", c.ID).Delete(&Comment{})
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}

// When a user is deleted, we also delete the comments that the user had
func (c *Comment) DeleteUserComments(db *gorm.DB, uid uint) (int64, error) {
	result := db.Where("user_id = ?", uid).Delete(&Comment{})
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}

// When a matchup is deleted, we also delete the comments that the matchup had
func (c *Comment) DeleteMatchupComments(db *gorm.DB, mid uint) (int64, error) {
	result := db.Where("matchup_id = ?", mid).Delete(&Comment{})
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}
