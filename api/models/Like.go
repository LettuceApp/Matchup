package models

import (
	"context"
	"time"

	appdb "Matchup/db"

	"github.com/jmoiron/sqlx"
)

type Like struct {
	ID        uint      `db:"id" json:"id"`
	PublicID  string    `db:"public_id" json:"public_id"`
	UserID    uint      `db:"user_id" json:"user_id"`
	MatchupID uint      `db:"matchup_id" json:"matchup_id"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

func (like *Like) SaveLike(db sqlx.ExtContext) (*Like, error) {
	like.PublicID = appdb.GeneratePublicID()
	like.CreatedAt = time.Now()
	like.UpdatedAt = time.Now()

	query, args, err := appdb.Psql.Insert("likes").
		Columns("public_id", "user_id", "matchup_id", "created_at", "updated_at").
		Values(like.PublicID, like.UserID, like.MatchupID, like.CreatedAt, like.UpdatedAt).
		Suffix("RETURNING *").
		ToSql()
	if err != nil {
		return nil, err
	}
	if err := sqlx.GetContext(context.Background(), db, like, query, args...); err != nil {
		return nil, err
	}
	return like, nil
}

func (l *Like) DeleteLike(db sqlx.ExtContext) (*Like, error) {
	// First fetch the like
	err := sqlx.GetContext(context.Background(), db, l, "SELECT * FROM likes WHERE id = $1", l.ID)
	if err != nil {
		return &Like{}, err
	}
	deletedLike := *l
	_, err = db.ExecContext(context.Background(), "DELETE FROM likes WHERE id = $1", l.ID)
	if err != nil {
		return &Like{}, err
	}
	return &deletedLike, nil
}

func (l *Like) GetLikesInfo(db sqlx.ExtContext, mid uint) (*[]Like, error) {
	likes := []Like{}
	err := sqlx.SelectContext(context.Background(), db, &likes, "SELECT * FROM likes WHERE matchup_id = $1", mid)
	if err != nil {
		return &[]Like{}, err
	}
	return &likes, err
}

func (l *Like) DeleteUserLikes(db sqlx.ExtContext, uid uint) (int64, error) {
	result, err := db.ExecContext(context.Background(), "DELETE FROM likes WHERE user_id = $1", uid)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (l *Like) DeleteMatchupLikes(db sqlx.ExtContext, mid uint) (int64, error) {
	result, err := db.ExecContext(context.Background(), "DELETE FROM likes WHERE matchup_id = $1", mid)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
