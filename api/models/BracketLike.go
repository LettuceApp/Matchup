package models

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"
)

type BracketLike struct {
	ID        uint      `db:"id" json:"id"`
	PublicID  string    `db:"public_id" json:"public_id"`
	UserID    uint      `db:"user_id" json:"user_id"`
	BracketID uint      `db:"bracket_id" json:"bracket_id"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

func (l *BracketLike) DeleteUserBracketLikes(db sqlx.ExtContext, uid uint) (int64, error) {
	result, err := db.ExecContext(context.Background(), "DELETE FROM bracket_likes WHERE user_id = $1", uid)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (l *BracketLike) DeleteBracketLikes(db sqlx.ExtContext, bid uint) (int64, error) {
	result, err := db.ExecContext(context.Background(), "DELETE FROM bracket_likes WHERE bracket_id = $1", bid)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
