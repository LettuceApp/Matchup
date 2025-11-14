package models

import (
	"errors"
	"html"
	"log"
	"os"
	"strings"
	"time"

	"Matchup/api/security"

	"github.com/badoux/checkmail"
	"gorm.io/gorm"
)

type User struct {
	ID         uint      `gorm:"primary_key;autoIncrement" json:"id"`
	Username   string    `gorm:"size:255;not null;unique" json:"username"`
	Email      string    `gorm:"size:100;not null;unique" json:"email"`
	Password   string    `gorm:"size:100;not null;" json:"password"`
	AvatarPath string    `gorm:"size:255;null;" json:"avatar_path"`
	CreatedAt  time.Time `gorm:"default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt  time.Time `gorm:"default:CURRENT_TIMESTAMP" json:"updated_at"`
}

func (u *User) HashPassword() error {
	hashedPassword, err := security.Hash(u.Password)
	if err != nil {
		return err
	}
	u.Password = string(hashedPassword)
	return nil
}

func (u *User) BeforeSave(tx *gorm.DB) (err error) {
	return u.HashPassword()
}

func (u *User) Prepare() {
	u.Username = html.EscapeString(strings.TrimSpace(u.Username))
	u.Email = html.EscapeString(strings.TrimSpace(u.Email))
	u.CreatedAt = time.Now()
	u.UpdatedAt = time.Now()
}

func (u *User) AfterFind(tx *gorm.DB) (err error) {
	if u.AvatarPath == "" || strings.HasPrefix(u.AvatarPath, "http") {
		return nil
	}
	bucket := os.Getenv("S3_BUCKET")      // bucket name only
	region := os.Getenv("AWS_REGION_ENV") // e.g., us-east-2
	if region == "" {
		region = "us-east-2"
	}
	// If you saved only the filename, add your prefix folder if needed
	key := u.AvatarPath
	if !strings.HasPrefix(key, "UserProfilePics/") {
		key = "UserProfilePics/" + key
	}
	u.AvatarPath = "https://" + bucket + ".s3." + region + ".amazonaws.com/" + key
	return nil
}

func (u *User) Validate(action string) map[string]string {
	var errorMessages = make(map[string]string)

	switch strings.ToLower(action) {
	case "update":
		if u.Email == "" {
			errorMessages["Required_email"] = "Required Email"
		}
		if u.Email != "" {
			if err := checkmail.ValidateFormat(u.Email); err != nil {
				errorMessages["Invalid_email"] = "Invalid Email"
			}
		}

	case "login":
		if u.Password == "" {
			errorMessages["Required_password"] = "Required Password"
		}
		if u.Email == "" {
			errorMessages["Required_email"] = "Required Email"
		}
		if u.Email != "" {
			if err := checkmail.ValidateFormat(u.Email); err != nil {
				errorMessages["Invalid_email"] = "Invalid Email"
			}
		}
	case "forgotpassword":
		if u.Email == "" {
			errorMessages["Required_email"] = "Required Email"
		}
		if u.Email != "" {
			if err := checkmail.ValidateFormat(u.Email); err != nil {
				errorMessages["Invalid_email"] = "Invalid Email"
			}
		}
	default:
		if u.Username == "" {
			errorMessages["Required_username"] = "Required Username"
		}
		if u.Password == "" {
			errorMessages["Required_password"] = "Required Password"
		}
		if len(u.Password) < 6 {
			errorMessages["Invalid_password"] = "Password should be at least 6 characters"
		}
		if u.Email == "" {
			errorMessages["Required_email"] = "Required Email"
		}
		if u.Email != "" {
			if err := checkmail.ValidateFormat(u.Email); err != nil {
				errorMessages["Invalid_email"] = "Invalid Email"
			}
		}
	}
	return errorMessages
}

func (u *User) SaveUser(db *gorm.DB) (*User, error) {
	err := db.Create(&u).Error
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (u *User) FindAllUsers(db *gorm.DB) (*[]User, error) {
	var users []User
	err := db.Limit(100).Find(&users).Error
	if err != nil {
		return nil, err
	}
	return &users, nil
}

func (u *User) FindUserByID(db *gorm.DB, uid uint) (*User, error) {
	var user User
	err := db.Where("id = ?", uid).Take(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("User not found")
		}
		return nil, err
	}
	return &user, nil
}

func (u *User) UpdateAUser(db *gorm.DB, uid uint) (*User, error) {
	if u.Password != "" {
		// Hash the password
		err := u.HashPassword()
		if err != nil {
			log.Fatal(err)
		}
	}

	err := db.Model(&User{}).Where("id = ?", uid).Updates(map[string]interface{}{
		"password":   u.Password,
		"email":      u.Email,
		"updated_at": time.Now(),
	}).Error
	if err != nil {
		return nil, err
	}

	// Display the updated user
	err = db.Where("id = ?", uid).Take(&u).Error
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (u *User) UpdateAUserAvatar(db *gorm.DB, uid uint) (*User, error) {
	err := db.Model(&User{}).Where("id = ?", uid).Updates(map[string]interface{}{
		"avatar_path": u.AvatarPath,
		"updated_at":  time.Now(),
	}).Error
	if err != nil {
		return nil, err
	}

	// Display the updated user
	err = db.Where("id = ?", uid).Take(&u).Error
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (u *User) DeleteAUser(db *gorm.DB, uid uint) (int64, error) {
	result := db.Where("id = ?", uid).Delete(&User{})
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}

func (u *User) UpdatePassword(db *gorm.DB) error {
	// Hash the password
	err := u.HashPassword()
	if err != nil {
		log.Fatal(err)
	}

	err = db.Model(&User{}).Where("email = ?", u.Email).Updates(map[string]interface{}{
		"password":   u.Password,
		"updated_at": time.Now(),
	}).Error
	if err != nil {
		return err
	}
	return nil
}
