package models

import (
	"fmt"
	"html"
	"strings"
	"time"

	"gorm.io/gorm"
)

type Comment struct {
	ID        uint      `gorm:"primary_key;autoIncrement" json:"id"`
	UserID    uint      `gorm:"not null" json:"user_id"`
	MatchupID uint      `gorm:"not null" json:"matchup_id"`
	Body      string    `gorm:"text;not null;" json:"body"`
	User      User      `json:"user"`
	CreatedAt time.Time `gorm:"default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt time.Time `gorm:"default:CURRENT_TIMESTAMP" json:"updated_at"`
}

func (c *Comment) Prepare() {
	c.ID = 0
	c.Body = html.EscapeString(strings.TrimSpace(c.Body))
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
	if c.ID != 0 {
		err = db.Model(&User{}).Where("id = ?", c.UserID).Take(&c.User).Error
		if err != nil {
			return nil, err
		}
	}
	return c, nil
}

func (c *Comment) GetComments(db *gorm.DB, mid uint) (*[]Comment, error) {
	comments := []Comment{}
	err := db.Model(&Comment{}).Where("matchup_id = ?", mid).Order("created_at desc").Find(&comments).Error
	if err != nil {
		return nil, err
	}
	if len(comments) > 0 {
		for i := range comments {
			err := db.Model(&User{}).Where("id = ?", comments[i].UserID).Take(&comments[i].User).Error
			if err != nil {
				return nil, err
			}
		}
	}
	return &comments, nil
}

func (c *Comment) UpdateAComment(db *gorm.DB) (*Comment, error) {
	err := db.Model(&Comment{}).Where("id = ?", c.ID).Updates(map[string]interface{}{"body": c.Body, "updated_at": time.Now()}).Error
	if err != nil {
		return nil, err
	}

	fmt.Println("this is the comment body: ", c.Body)
	if c.ID != 0 {
		err = db.Model(&User{}).Where("id = ?", c.UserID).Take(&c.User).Error
		if err != nil {
			return nil, err
		}
	}
	return c, nil
}

func (c *Comment) DeleteAComment(db *gorm.DB) (int64, error) {
	result := db.Model(&Comment{}).Where("id = ?", c.ID).Delete(&Comment{})
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}

// When a user is deleted, we also delete the comments that the user had
func (c *Comment) DeleteUserComments(db *gorm.DB, uid uint) (int64, error) {
	result := db.Model(&Comment{}).Where("user_id = ?", uid).Delete(&Comment{})
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}

// When a matchup is deleted, we also delete the comments that the matchup had
func (c *Comment) DeleteMatchupComments(db *gorm.DB, mid uint) (int64, error) {
	result := db.Model(&Comment{}).Where("matchup_id = ?", mid).Delete(&Comment{})
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}
