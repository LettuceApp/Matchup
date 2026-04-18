package models

import (
	"context"
	"errors"
	"html"
	"strings"
	"time"

	appdb "Matchup/db"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

type Bracket struct {
	ID          uint   `db:"id" json:"id"`
	PublicID    string `db:"public_id" json:"public_id"`
	Title       string `db:"title" json:"title"`
	Description string `db:"description" json:"description"`

	Author   User `db:"-" json:"-"`
	AuthorID uint `db:"author_id" json:"author_id"`

	Size         int    `db:"size" json:"size"`
	Status       string `db:"status" json:"status"`
	CurrentRound int    `db:"current_round" json:"current_round"`
	Visibility   string `db:"visibility" json:"visibility"`

	AdvanceMode          string     `db:"advance_mode" json:"advance_mode"`
	RoundDurationSeconds int        `db:"round_duration_seconds" json:"round_duration_seconds"`
	RoundStartedAt       *time.Time `db:"round_started_at" json:"round_started_at,omitempty"`
	RoundEndsAt          *time.Time `db:"round_ends_at" json:"round_ends_at,omitempty"`
	CompletedAt          *time.Time `db:"completed_at" json:"completed_at,omitempty"`

	ChampionMatchupID *uint `db:"champion_matchup_id" json:"champion_matchup_id,omitempty"`
	ChampionItemID    *uint `db:"champion_item_id" json:"champion_item_id,omitempty"`

	Tags pq.StringArray `db:"tags" json:"tags"`

	// Denormalized counters maintained by triggers (migration 006).
	LikesCount    int `db:"likes_count" json:"likes_count"`
	CommentsCount int `db:"comments_count" json:"comments_count"`

	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

func (b *Bracket) Prepare() {
	b.Title = html.EscapeString(strings.TrimSpace(b.Title))
	b.Description = html.EscapeString(strings.TrimSpace(b.Description))
	b.Author = User{}
	b.CreatedAt = time.Now()
	b.UpdatedAt = time.Now()

	if b.Status == "" {
		b.Status = "draft"
	}
	if b.CurrentRound == 0 {
		b.CurrentRound = 1
	}

	if strings.TrimSpace(b.AdvanceMode) == "" {
		b.AdvanceMode = "manual"
	}
	if b.RoundDurationSeconds < 0 {
		b.RoundDurationSeconds = 0
	}
	if strings.TrimSpace(b.Visibility) == "" {
		b.Visibility = "public"
	}
}

func (b *Bracket) Validate() map[string]string {
	var err error
	errorsMap := make(map[string]string)

	if b.Title == "" {
		err = errors.New("required title")
		errorsMap["Required_title"] = err.Error()
	}
	if b.AuthorID == 0 {
		err = errors.New("required author")
		errorsMap["Required_author"] = err.Error()
	}
	if b.Status == "" {
		err = errors.New("required status")
		errorsMap["Required_status"] = err.Error()
	}

	mode := strings.TrimSpace(b.AdvanceMode)
	if mode == "" {
		mode = "manual"
	}
	if mode != "manual" && mode != "timer" {
		errorsMap["advance_mode"] = "advance_mode must be 'manual' or 'timer'"
	}
	if mode == "timer" {
		if b.RoundDurationSeconds < 60 || b.RoundDurationSeconds > 86400 {
			errorsMap["round_duration_seconds"] = "round_duration_seconds must be between 60 and 86400 (up to 24 hours)"
		}
	}

	return errorsMap
}

func (b *Bracket) SaveBracket(db sqlx.ExtContext) (*Bracket, error) {
	if strings.TrimSpace(b.PublicID) == "" {
		b.PublicID = appdb.GeneratePublicID()
	}

	if b.Tags == nil {
		b.Tags = pq.StringArray{}
	}
	query, args, err := appdb.Psql.Insert("brackets").
		Columns("public_id", "title", "description", "author_id", "size", "status", "current_round", "visibility", "advance_mode", "round_duration_seconds", "round_started_at", "round_ends_at", "completed_at", "champion_matchup_id", "champion_item_id", "tags", "created_at", "updated_at").
		Values(b.PublicID, b.Title, b.Description, b.AuthorID, b.Size, b.Status, b.CurrentRound, b.Visibility, b.AdvanceMode, b.RoundDurationSeconds, b.RoundStartedAt, b.RoundEndsAt, b.CompletedAt, b.ChampionMatchupID, b.ChampionItemID, b.Tags, b.CreatedAt, b.UpdatedAt).
		Suffix("RETURNING *").
		ToSql()
	if err != nil {
		return nil, err
	}
	if err := sqlx.GetContext(context.Background(), db, b, query, args...); err != nil {
		return nil, err
	}

	// Load author
	if err := sqlx.GetContext(context.Background(), db, &b.Author, "SELECT * FROM users WHERE id = $1", b.AuthorID); err != nil {
		return nil, err
	}
	b.Author.ProcessAvatarPath()

	return b, nil
}

func (b *Bracket) FindBracketByID(db sqlx.ExtContext, id uint) (*Bracket, error) {
	err := sqlx.GetContext(context.Background(), db, b, "SELECT * FROM brackets WHERE id = $1", id)
	if err != nil {
		return nil, err
	}
	if err := sqlx.GetContext(context.Background(), db, &b.Author, "SELECT * FROM users WHERE id = $1", b.AuthorID); err != nil {
		return nil, err
	}
	b.Author.ProcessAvatarPath()
	return b, nil
}

func (b *Bracket) FindUserBrackets(db sqlx.ExtContext, uid uint) (*[]Bracket, error) {
	var brackets []Bracket
	err := sqlx.SelectContext(context.Background(), db, &brackets,
		"SELECT * FROM brackets WHERE author_id = $1 ORDER BY created_at DESC", uid)
	if err != nil {
		return nil, err
	}
	// Load authors
	if len(brackets) > 0 {
		var author User
		if err := sqlx.GetContext(context.Background(), db, &author, "SELECT * FROM users WHERE id = $1", uid); err != nil {
			return nil, err
		}
		author.ProcessAvatarPath()
		for i := range brackets {
			brackets[i].Author = author
		}
	}
	return &brackets, nil
}

func (b *Bracket) UpdateBracket(db sqlx.ExtContext) (*Bracket, error) {
	b.UpdatedAt = time.Now()

	query, args, err := appdb.Psql.Update("brackets").
		Set("title", b.Title).
		Set("description", b.Description).
		Set("status", b.Status).
		Set("current_round", b.CurrentRound).
		Set("advance_mode", b.AdvanceMode).
		Set("round_duration_seconds", b.RoundDurationSeconds).
		Set("round_started_at", b.RoundStartedAt).
		Set("round_ends_at", b.RoundEndsAt).
		Set("champion_matchup_id", b.ChampionMatchupID).
		Set("champion_item_id", b.ChampionItemID).
		Set("completed_at", b.CompletedAt).
		Set("updated_at", b.UpdatedAt).
		Where("id = ?", b.ID).
		ToSql()
	if err != nil {
		return nil, err
	}
	if _, err := db.ExecContext(context.Background(), query, args...); err != nil {
		return nil, err
	}

	// Reload author
	if err := sqlx.GetContext(context.Background(), db, &b.Author, "SELECT * FROM users WHERE id = $1", b.AuthorID); err != nil {
		return nil, err
	}
	b.Author.ProcessAvatarPath()

	return b, nil
}

func (b *Bracket) DeleteBracket(db sqlx.ExtContext, id uint) (int64, error) {
	result, err := db.ExecContext(context.Background(), "DELETE FROM brackets WHERE id = $1", id)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
