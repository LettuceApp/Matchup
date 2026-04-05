package models

import (
	"context"
	"database/sql"
	"errors"
	"html"
	"log"
	"strings"
	"time"

	appdb "Matchup/db"
	"Matchup/security"

	"github.com/badoux/checkmail"
	"github.com/jmoiron/sqlx"
)

type User struct {
	ID             uint      `db:"id" json:"id"`
	PublicID       string    `db:"public_id" json:"public_id"`
	Username       string    `db:"username" json:"username"`
	Email          string    `db:"email" json:"email"`
	Password       string    `db:"password" json:"password"`
	AvatarPath     string    `db:"avatar_path" json:"avatar_path"`
	Bio            string    `db:"bio" json:"bio"`
	IsAdmin        bool      `db:"is_admin" json:"is_admin"`
	IsPrivate      bool      `db:"is_private" json:"is_private"`
	FollowersCount int64     `db:"followers_count" json:"followers_count"`
	FollowingCount int64     `db:"following_count" json:"following_count"`
	CreatedAt      time.Time `db:"created_at" json:"created_at"`
	UpdatedAt      time.Time `db:"updated_at" json:"updated_at"`
}

const SeedAdminEmail = "cordelljenkins1914@gmail.com"

func (u *User) HashPassword() error {
	hashedPassword, err := security.Hash(u.Password)
	if err != nil {
		return err
	}
	u.Password = string(hashedPassword)
	return nil
}

// ProcessAvatarPath converts a relative path to S3 URL after query.
func (u *User) ProcessAvatarPath() {
	u.AvatarPath = appdb.ProcessAvatarPath(u.AvatarPath)
}

func (u *User) Prepare() {
	u.Username = html.EscapeString(strings.ToLower(strings.TrimSpace(u.Username)))
	u.Email = html.EscapeString(strings.ToLower(strings.TrimSpace(u.Email)))

	// Derive admin flag on the server side
	if u.Email == strings.ToLower(SeedAdminEmail) {
		u.IsAdmin = true
	} else {
		// For newly created users, force non-admin unless they match the seed email
		if u.ID == 0 {
			u.IsAdmin = false
		}
	}

	u.CreatedAt = time.Now()
	u.UpdatedAt = time.Now()
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

func (u *User) SaveUser(db sqlx.ExtContext) (*User, error) {
	if strings.TrimSpace(u.PublicID) == "" {
		u.PublicID = appdb.GeneratePublicID()
	}
	if err := u.HashPassword(); err != nil {
		return nil, err
	}

	query, args, err := appdb.Psql.Insert("users").
		Columns("public_id", "username", "email", "password", "avatar_path", "is_admin", "is_private", "followers_count", "following_count", "created_at", "updated_at").
		Values(u.PublicID, u.Username, u.Email, u.Password, u.AvatarPath, u.IsAdmin, u.IsPrivate, u.FollowersCount, u.FollowingCount, u.CreatedAt, u.UpdatedAt).
		Suffix("RETURNING *").
		ToSql()
	if err != nil {
		return nil, err
	}
	if err := sqlx.GetContext(context.Background(), db, u, query, args...); err != nil {
		return nil, err
	}
	u.ProcessAvatarPath()
	return u, nil
}

func (u *User) FindAllUsers(db sqlx.ExtContext) (*[]User, error) {
	var users []User
	query, args, err := appdb.Psql.Select("*").From("users").Limit(100).ToSql()
	if err != nil {
		return nil, err
	}
	if err := sqlx.SelectContext(context.Background(), db, &users, query, args...); err != nil {
		return nil, err
	}
	for i := range users {
		users[i].ProcessAvatarPath()
	}
	return &users, nil
}

func (u *User) FindUserByID(db sqlx.ExtContext, uid uint) (*User, error) {
	var user User
	err := sqlx.GetContext(context.Background(), db, &user, "SELECT * FROM users WHERE id = $1", uid)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("User not found")
		}
		return nil, err
	}
	user.ProcessAvatarPath()
	return &user, nil
}

func (u *User) UpdateAUser(db sqlx.ExtContext, uid uint) (*User, error) {
	if u.Password != "" {
		err := u.HashPassword()
		if err != nil {
			log.Fatal(err)
		}
	}

	query, args, err := appdb.Psql.Update("users").
		Set("password", u.Password).
		Set("email", u.Email).
		Set("bio", u.Bio).
		Set("updated_at", time.Now()).
		Where("id = ?", uid).
		ToSql()
	if err != nil {
		return nil, err
	}
	if _, err := db.ExecContext(context.Background(), query, args...); err != nil {
		return nil, err
	}

	// Reload updated user
	err = sqlx.GetContext(context.Background(), db, u, "SELECT * FROM users WHERE id = $1", uid)
	if err != nil {
		return nil, err
	}
	u.ProcessAvatarPath()
	return u, nil
}

func (u *User) UpdateAUserAvatar(db sqlx.ExtContext, uid uint) (*User, error) {
	query, args, err := appdb.Psql.Update("users").
		Set("avatar_path", u.AvatarPath).
		Set("updated_at", time.Now()).
		Where("id = ?", uid).
		ToSql()
	if err != nil {
		return nil, err
	}
	if _, err := db.ExecContext(context.Background(), query, args...); err != nil {
		return nil, err
	}

	err = sqlx.GetContext(context.Background(), db, u, "SELECT * FROM users WHERE id = $1", uid)
	if err != nil {
		return nil, err
	}
	u.ProcessAvatarPath()
	return u, nil
}

func (u *User) DeleteAUser(db sqlx.ExtContext, uid uint) (int64, error) {
	result, err := db.ExecContext(context.Background(), "DELETE FROM users WHERE id = $1", uid)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (u *User) UpdatePassword(db sqlx.ExtContext) error {
	err := u.HashPassword()
	if err != nil {
		log.Fatal(err)
	}

	query, args, err := appdb.Psql.Update("users").
		Set("password", u.Password).
		Set("updated_at", time.Now()).
		Where("email = ?", u.Email).
		ToSql()
	if err != nil {
		return err
	}
	_, err = db.ExecContext(context.Background(), query, args...)
	return err
}
