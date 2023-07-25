package models

import (
	"errors"
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Comment struct {
	ID        uuid.UUID `gorm:"primary_key;type:uuid;default:gen_random_uuid()" json:"id"`
	UserID    uuid.UUID `gorm:"not null" json:"user_id"`
	MatchupID uuid.UUID `gorm:"not null" json:"matchup_id"`
	Body      string    `gorm:"text;not null;" json:"body"`
	User      User      `json:"user"`
	CreatedAt time.Time `gorm:"default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt time.Time `gorm:"default:CURRENT_TIMESTAMP" json:"updated_at"`
}

func (c *Comment) Prepare() {
	c.ID = uuid.Nil
	c.Body = html.EscapeString(strings.TrimSpace(c.Body))
	c.User = User{}
	c.CreatedAt = time.Now()
	c.UpdatedAt = time.Now()
}

func (c *Comment) BeforeCreate(tx *gorm.DB) error {
	c.ID = uuid.Nil
	c.Body = html.EscapeString(strings.TrimSpace(c.Body))
	c.User = User{}
	c.CreatedAt = time.Now()
	c.UpdatedAt = time.Now()
	return nil
}

func (c *Comment) Validate(action string) map[string]string {
	var errorMessages = make(map[string]string)
	var err error

	switch strings.ToLower(action) {
	case "update":
		if c.Body == "" {
			err = errors.New("required comment")
			errorMessages["required_body"] = err.Error()
		}
	default:
		if c.Body == "" {
			err = errors.New("required comment")
			errorMessages["required_body"] = err.Error()
		}
	}
	return errorMessages
}

func (c *Comment) SaveComment(db *gorm.DB) (*Comment, error) {
	err := db.Debug().Create(&c).Error
	if err != nil {
		return nil, err
	}
	if c.ID != uuid.Nil {
		err = db.Debug().Model(&User{}).Where("id = ?", c.UserID).Take(&c.User).Error
		if err != nil {
			return nil, err
		}
	}
	return c, nil
}

func (c *Comment) GetComments(db *gorm.DB, mid uuid.UUID) (*[]Comment, error) {

	comments := []Comment{}
	err := db.Debug().Model(&Comment{}).Where("matchup_id = ?", mid).Order("created_at desc").Find(&comments).Error
	if err != nil {
		return &[]Comment{}, err
	}
	if len(comments) > 0 {
		for i := range comments {
			err := db.Debug().Model(&User{}).Where("id = ?", comments[i].UserID).Take(&comments[i].User).Error
			if err != nil {
				return &[]Comment{}, err
			}
		}
	}
	return &comments, err
}

func (c *Comment) UpdateAComment(db *gorm.DB) (*Comment, error) {

	var err error

	err = db.Debug().Model(&Comment{}).Where("id = ?", c.ID).Updates(map[string]interface{}{"body": c.Body, "updated_at": time.Now()}).Error
	if err != nil {
		return &Comment{}, err
	}

	fmt.Println("this is the comment body: ", c.Body)
	if c.ID != uuid.Nil {
		err = db.Debug().Model(&User{}).Where("id = ?", c.UserID).Take(&c.User).Error
		if err != nil {
			return &Comment{}, err
		}
	}
	return c, nil
}

func (c *Comment) DeleteAComment(db *gorm.DB) (int64, error) {

	db = db.Model(&Comment{}).Where("id = ?", c.ID).Take(&Comment{}).Delete(&Comment{})

	if db.Error != nil {
		return 0, db.Error
	}
	return db.RowsAffected, nil
}

// When a user is deleted, we also delete the comments that the user had
func (c *Comment) DeleteUserComments(db *gorm.DB, uid uuid.UUID) (int64, error) {
	comments := []Comment{}
	db = db.Model(&Comment{}).Where("user_id = ?", uid).Find(&comments).Delete(&comments)
	if db.Error != nil {
		return 0, db.Error
	}
	return db.RowsAffected, nil
}

// When a matchup is deleted, we also delete the comments that the matchup had
func (c *Comment) DeleteMatchupComments(db *gorm.DB, mid uuid.UUID) (int64, error) {
	comments := []Comment{}
	db = db.Model(&Comment{}).Where("matchup_id = ?", mid).Find(&comments).Delete(&comments)
	if db.Error != nil {
		return 0, db.Error
	}
	return db.RowsAffected, nil
}
