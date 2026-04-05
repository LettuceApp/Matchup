package models

import (
	"context"
	"html"
	"strings"
	"time"

	appdb "Matchup/db"

	"github.com/jmoiron/sqlx"
)

type ResetPassword struct {
	ID        uint       `db:"id" json:"id"`
	CreatedAt time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt time.Time  `db:"updated_at" json:"updated_at"`
	DeletedAt *time.Time `db:"deleted_at" json:"deleted_at,omitempty"`
	Email     string     `db:"email" json:"email"`
	Token     string     `db:"token" json:"token"`
}

func (resetPassword *ResetPassword) Prepare() {
	resetPassword.Token = html.EscapeString(strings.TrimSpace(resetPassword.Token))
	resetPassword.Email = html.EscapeString(strings.TrimSpace(resetPassword.Email))
}

func (resetPassword *ResetPassword) SaveDatails(db sqlx.ExtContext) (*ResetPassword, error) {
	resetPassword.CreatedAt = time.Now()
	resetPassword.UpdatedAt = time.Now()

	query, args, err := appdb.Psql.Insert("reset_passwords").
		Columns("email", "token", "created_at", "updated_at").
		Values(resetPassword.Email, resetPassword.Token, resetPassword.CreatedAt, resetPassword.UpdatedAt).
		Suffix("RETURNING *").
		ToSql()
	if err != nil {
		return &ResetPassword{}, err
	}
	if err := sqlx.GetContext(context.Background(), db, resetPassword, query, args...); err != nil {
		return &ResetPassword{}, err
	}
	return resetPassword, nil
}

func (resetPassword *ResetPassword) DeleteDetails(db sqlx.ExtContext) (int64, error) {
	result, err := db.ExecContext(context.Background(), "DELETE FROM reset_passwords WHERE id = $1", resetPassword.ID)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
