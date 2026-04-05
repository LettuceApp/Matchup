package models

import (
	"context"
	"html"
	"strings"
	"time"

	appdb "Matchup/db"

	"github.com/jmoiron/sqlx"
)

type BracketComment struct {
	ID        uint      `db:"id" json:"id"`
	PublicID  string    `db:"public_id" json:"public_id"`
	UserID    uint      `db:"user_id" json:"user_id"`
	BracketID uint      `db:"bracket_id" json:"bracket_id"`
	Author    User      `db:"-" json:"author"`
	Body      string    `db:"body" json:"body"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
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

func (c *BracketComment) SaveComment(db sqlx.ExtContext) (*BracketComment, error) {
	c.PublicID = appdb.GeneratePublicID()

	query, args, err := appdb.Psql.Insert("bracket_comments").
		Columns("public_id", "user_id", "bracket_id", "body", "created_at", "updated_at").
		Values(c.PublicID, c.UserID, c.BracketID, c.Body, c.CreatedAt, c.UpdatedAt).
		Suffix("RETURNING *").
		ToSql()
	if err != nil {
		return nil, err
	}
	if err := sqlx.GetContext(context.Background(), db, c, query, args...); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *BracketComment) GetComments(db sqlx.ExtContext, bid uint) (*[]BracketComment, error) {
	comments := []BracketComment{}
	err := sqlx.SelectContext(context.Background(), db, &comments,
		"SELECT * FROM bracket_comments WHERE bracket_id = $1 ORDER BY created_at DESC", bid)
	if err != nil {
		return nil, err
	}
	// Load authors
	if len(comments) > 0 {
		userIDs := make([]uint, len(comments))
		for i, cm := range comments {
			userIDs[i] = cm.UserID
		}
		query, args, err := sqlx.In("SELECT * FROM users WHERE id IN (?)", userIDs)
		if err != nil {
			return nil, err
		}
		var users []User
		if err := sqlx.SelectContext(context.Background(), db, &users, db.Rebind(query), args...); err != nil {
			return nil, err
		}
		userMap := make(map[uint]User, len(users))
		for _, u := range users {
			u.ProcessAvatarPath()
			userMap[u.ID] = u
		}
		for i := range comments {
			comments[i].Author = userMap[comments[i].UserID]
		}
	}
	return &comments, nil
}

func (c *BracketComment) DeleteAComment(db sqlx.ExtContext) (int64, error) {
	result, err := db.ExecContext(context.Background(), "DELETE FROM bracket_comments WHERE id = $1", c.ID)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
