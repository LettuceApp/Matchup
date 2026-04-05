package models

import (
	"context"
	"html"
	"strings"
	"time"

	appdb "Matchup/db"

	"github.com/jmoiron/sqlx"
)

type Comment struct {
	ID        uint      `db:"id" json:"id"`
	PublicID  string    `db:"public_id" json:"public_id"`
	UserID    uint      `db:"user_id" json:"user_id"`
	MatchupID uint      `db:"matchup_id" json:"matchup_id"`
	Author    User      `db:"-" json:"author"`
	Body      string    `db:"body" json:"body"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
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

func (c *Comment) SaveComment(db sqlx.ExtContext) (*Comment, error) {
	c.PublicID = appdb.GeneratePublicID()

	query, args, err := appdb.Psql.Insert("comments").
		Columns("public_id", "user_id", "matchup_id", "body", "created_at", "updated_at").
		Values(c.PublicID, c.UserID, c.MatchupID, c.Body, c.CreatedAt, c.UpdatedAt).
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

func (c *Comment) GetComments(db sqlx.ExtContext, mid uint) (*[]Comment, error) {
	comments := []Comment{}
	err := sqlx.SelectContext(context.Background(), db, &comments,
		"SELECT * FROM comments WHERE matchup_id = $1 ORDER BY created_at DESC", mid)
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

func (c *Comment) UpdateAComment(db sqlx.ExtContext) (*Comment, error) {
	_, err := db.ExecContext(context.Background(),
		"UPDATE comments SET body = $1, updated_at = $2 WHERE id = $3",
		c.Body, time.Now(), c.ID)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Comment) DeleteAComment(db sqlx.ExtContext) (int64, error) {
	result, err := db.ExecContext(context.Background(), "DELETE FROM comments WHERE id = $1", c.ID)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (c *Comment) DeleteUserComments(db sqlx.ExtContext, uid uint) (int64, error) {
	result, err := db.ExecContext(context.Background(), "DELETE FROM comments WHERE user_id = $1", uid)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (c *Comment) DeleteMatchupComments(db sqlx.ExtContext, mid uint) (int64, error) {
	result, err := db.ExecContext(context.Background(), "DELETE FROM comments WHERE matchup_id = $1", mid)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
