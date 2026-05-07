package models

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	appdb "Matchup/db"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

type Matchup struct {
	ID              uint          `db:"id" json:"id"`
	PublicID        string        `db:"public_id" json:"public_id"`
	// ShortID is the 8-char base62 identifier used in share URLs (see
	// migration 014). Nullable during rollout until the backfill binary
	// populates pre-existing rows; new matchups get one on Create.
	ShortID         *string       `db:"short_id" json:"short_id,omitempty"`
	Title           string        `db:"title" json:"title"`
	// Content lives in matchup_details (split out in migration 005). Transient on this struct;
	// loaded via LoadMatchupContent / saved via SaveMatchupContent.
	Content         string        `db:"-" json:"content"`
	Author          User          `db:"-" json:"-"`
	AuthorID        uint          `db:"author_id" json:"author_id"`
	Items           []MatchupItem `db:"-" json:"items"`
	Comments        []Comment     `db:"-" json:"comments"`
	CreatedAt       time.Time     `db:"created_at" json:"created_at"`
	UpdatedAt       time.Time     `db:"updated_at" json:"updated_at"`
	Status          string        `db:"status" json:"status"`
	EndMode         string        `db:"end_mode" json:"end_mode"`
	DurationSeconds int           `db:"duration_seconds" json:"duration_seconds"`
	BracketID       *uint         `db:"bracket_id" json:"bracket_id,omitempty"`
	Round           *int          `db:"round" json:"round,omitempty"`
	Seed            *int          `db:"seed" json:"seed,omitempty"`
	WinnerItemID    *uint         `db:"winner_item_id" json:"winner_item_id"`
	StartTime       *time.Time    `db:"start_time" json:"start_time,omitempty"`
	EndTime         *time.Time    `db:"end_time" json:"end_time,omitempty"`
	Visibility      string        `db:"visibility" json:"visibility"`
	ImagePath       string        `db:"image_path" json:"image_path"`
	Tags            pq.StringArray `db:"tags" json:"tags"`
	// Denormalized counters maintained by triggers (migration 006).
	LikesCount      int           `db:"likes_count" json:"likes_count"`
	CommentsCount   int           `db:"comments_count" json:"comments_count"`
}

type MatchupItem struct {
	ID        uint    `db:"id" json:"id"`
	PublicID  string  `db:"public_id" json:"public_id"`
	Matchup   Matchup `db:"-" json:"-"`
	MatchupID uint    `db:"matchup_id" json:"matchup_id"`
	Item      string  `db:"item" json:"item"`
	Votes     int     `db:"votes" json:"votes"`
	// Relative S3 path (mirrors matchups.image_path). Empty when the
	// item has no thumbnail. The proto mapper lifts this to a full URL
	// via db.ProcessMatchupItemImagePath. Migration 026 added the column.
	ImagePath string `db:"image_path" json:"image_path"`
}

// MatchupDetails holds the large `content` body, separated from `matchups`
// (vertical hot/cold split — migration 005). Read on demand for detail views;
// not loaded for list/feed paths so row fetches stay narrow.
type MatchupDetails struct {
	MatchupID uint   `db:"matchup_id" json:"matchup_id"`
	Content   string `db:"content" json:"content"`
}

// LoadMatchupContent fetches the content body for a single matchup.
// Returns "" without error when no row exists (e.g. matchup created before backfill).
func LoadMatchupContent(db sqlx.ExtContext, matchupID uint) (string, error) {
	var content string
	err := sqlx.GetContext(context.Background(), db, &content,
		"SELECT content FROM matchup_details WHERE matchup_id = $1", matchupID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return content, nil
}

// SaveMatchupContent upserts the content body for a matchup.
func SaveMatchupContent(db sqlx.ExtContext, matchupID uint, content string) error {
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO matchup_details (matchup_id, content) VALUES ($1, $2)
		 ON CONFLICT (matchup_id) DO UPDATE SET content = EXCLUDED.content`,
		matchupID, content)
	return err
}

// LoadMatchupContents batch-loads content for many matchups, keyed by matchup ID.
// Use this when an admin/edit view needs content for multiple matchups at once.
func LoadMatchupContents(db sqlx.ExtContext, matchupIDs []uint) (map[uint]string, error) {
	out := make(map[uint]string, len(matchupIDs))
	if len(matchupIDs) == 0 {
		return out, nil
	}
	query, args, err := sqlx.In("SELECT matchup_id, content FROM matchup_details WHERE matchup_id IN (?)", matchupIDs)
	if err != nil {
		return nil, err
	}
	var rows []MatchupDetails
	if err := sqlx.SelectContext(context.Background(), db, &rows, db.Rebind(query), args...); err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.MatchupID] = r.Content
	}
	return out, nil
}

func (m *Matchup) Prepare() {
	// React auto-escapes JSX text nodes at render time, so the
	// frontend renders user content safely without us pre-escaping
	// it here. Pre-escaping was breaking the round-trip — apostrophes
	// in titles surfaced as the literal `&#39;` in the UI.
	m.Title = strings.TrimSpace(m.Title)
	m.Content = strings.TrimSpace(m.Content)
	m.Author = User{}
	m.CreatedAt = time.Now()
	m.UpdatedAt = time.Now()

	if strings.TrimSpace(m.EndMode) == "" {
		m.EndMode = "manual"
	}
	if strings.TrimSpace(m.Visibility) == "" {
		m.Visibility = "public"
	}
}

func (m *Matchup) Validate() map[string]string {
	var err error
	var errorMessages = make(map[string]string)

	if m.Title == "" {
		err = errors.New("required title")
		errorMessages["Required_title"] = err.Error()
	}
	if m.AuthorID == 0 {
		err = errors.New("required author")
		errorMessages["Required_author"] = err.Error()
	}
	return errorMessages
}

func (m *Matchup) SaveMatchup(db sqlx.ExtContext) (*Matchup, error) {
	if strings.TrimSpace(m.PublicID) == "" {
		m.PublicID = appdb.GeneratePublicID()
	}

	if m.Tags == nil {
		m.Tags = pq.StringArray{}
	}
	query, args, err := appdb.Psql.Insert("matchups").
		Columns("public_id", "short_id", "title", "author_id", "status", "end_mode", "duration_seconds", "bracket_id", "round", "seed", "winner_item_id", "start_time", "end_time", "visibility", "image_path", "tags", "created_at", "updated_at").
		Values(m.PublicID, m.ShortID, m.Title, m.AuthorID, m.Status, m.EndMode, m.DurationSeconds, m.BracketID, m.Round, m.Seed, m.WinnerItemID, m.StartTime, m.EndTime, m.Visibility, m.ImagePath, m.Tags, m.CreatedAt, m.UpdatedAt).
		Suffix("RETURNING *").
		ToSql()
	if err != nil {
		return nil, err
	}
	// Capture content separately because GetContext won't repopulate the transient field.
	contentBody := m.Content
	if err := sqlx.GetContext(context.Background(), db, m, query, args...); err != nil {
		return nil, err
	}
	m.Content = contentBody
	if err := SaveMatchupContent(db, m.ID, contentBody); err != nil {
		return nil, err
	}

	// Save items
	for i, item := range m.Items {
		if item.ID == 0 {
			m.Items[i].MatchupID = m.ID
			if strings.TrimSpace(m.Items[i].PublicID) == "" {
				m.Items[i].PublicID = appdb.GeneratePublicID()
			}
			q, a, err := appdb.Psql.Insert("matchup_items").
				Columns("public_id", "matchup_id", "item", "votes", "image_path").
				Values(m.Items[i].PublicID, m.Items[i].MatchupID, m.Items[i].Item, m.Items[i].Votes, m.Items[i].ImagePath).
				Suffix("RETURNING *").
				ToSql()
			if err != nil {
				return nil, err
			}
			if err := sqlx.GetContext(context.Background(), db, &m.Items[i], q, a...); err != nil {
				return nil, err
			}
		}
	}

	// Load author
	if err := sqlx.GetContext(context.Background(), db, &m.Author, "SELECT * FROM users WHERE id = $1", m.AuthorID); err != nil {
		return nil, err
	}
	m.Author.ProcessAvatarPath()

	return m, nil
}

func (m *Matchup) FindAllMatchups(db sqlx.ExtContext) ([]Matchup, error) {
	matchups := []Matchup{}
	err := sqlx.SelectContext(context.Background(), db, &matchups, "SELECT * FROM matchups")
	if err != nil {
		return []Matchup{}, err
	}
	if err := LoadMatchupAssociations(db, matchups); err != nil {
		return []Matchup{}, err
	}
	return matchups, nil
}

func (m *Matchup) FindMatchupByID(db sqlx.ExtContext, id uint) (*Matchup, error) {
	err := sqlx.GetContext(context.Background(), db, m, "SELECT * FROM matchups WHERE id = $1", id)
	if err != nil {
		return nil, err
	}
	if err := sqlx.GetContext(context.Background(), db, &m.Author, "SELECT * FROM users WHERE id = $1", m.AuthorID); err != nil {
		return nil, err
	}
	m.Author.ProcessAvatarPath()
	return m, nil
}

func (m *Matchup) UpdateMatchup(db sqlx.ExtContext) (*Matchup, error) {
	_, err := db.ExecContext(context.Background(),
		"UPDATE matchups SET title = $1, updated_at = $2 WHERE id = $3",
		m.Title, time.Now(), m.ID)
	if err != nil {
		return &Matchup{}, err
	}
	if err := SaveMatchupContent(db, m.ID, m.Content); err != nil {
		return &Matchup{}, err
	}

	for i, item := range m.Items {
		item.MatchupID = m.ID
		_, err := db.ExecContext(context.Background(),
			"UPDATE matchup_items SET matchup_id = $1, item = $2, votes = $3 WHERE id = $4",
			item.MatchupID, item.Item, item.Votes, item.ID)
		if err != nil {
			return &Matchup{}, err
		}
		m.Items[i] = item
	}

	if m.ID != 0 {
		err = sqlx.GetContext(context.Background(), db, &m.Author, "SELECT * FROM users WHERE id = $1", m.AuthorID)
		if err != nil {
			return &Matchup{}, err
		}
		m.Author.ProcessAvatarPath()
	}
	return m, nil
}

func (m *Matchup) DeleteMatchup(db sqlx.ExtContext, id uint) (int64, error) {
	result, err := db.ExecContext(context.Background(), "DELETE FROM matchups WHERE id = $1", id)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (m *Matchup) FindUserMatchups(db sqlx.ExtContext, uid uint) (*[]Matchup, error) {
	var matchups []Matchup
	err := sqlx.SelectContext(context.Background(), db, &matchups,
		"SELECT * FROM matchups WHERE author_id = $1 ORDER BY created_at DESC LIMIT 100", uid)
	if err != nil {
		return nil, err
	}
	if err := LoadMatchupAssociations(db, matchups); err != nil {
		return nil, err
	}
	return &matchups, nil
}

func (m *Matchup) DeleteUserMatchups(db sqlx.ExtContext, uid uint) (int64, error) {
	result, err := db.ExecContext(context.Background(), "DELETE FROM matchups WHERE author_id = $1", uid)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (m *MatchupItem) IncrementVotes(db sqlx.ExtContext) error {
	_, err := db.ExecContext(context.Background(),
		"UPDATE matchup_items SET votes = votes + 1 WHERE id = $1", m.ID)
	return err
}

func (m *Matchup) GetComments(db sqlx.ExtContext) (*[]Comment, error) {
	comments := []Comment{}
	err := sqlx.SelectContext(context.Background(), db, &comments,
		"SELECT * FROM comments WHERE matchup_id = $1 ORDER BY created_at DESC", m.ID)
	if err != nil {
		return &[]Comment{}, err
	}
	return &comments, nil
}

func FindMatchupsByBracket(db sqlx.ExtContext, bracketID uint) ([]Matchup, error) {
	var matchups []Matchup
	err := sqlx.SelectContext(context.Background(), db, &matchups,
		"SELECT * FROM matchups WHERE bracket_id = $1 ORDER BY round ASC, seed ASC, created_at ASC", bracketID)
	if err != nil {
		return nil, err
	}
	if err := LoadMatchupAssociations(db, matchups); err != nil {
		return nil, err
	}
	return matchups, err
}

func (m *Matchup) WinnerItem() (*MatchupItem, error) {
	if m.WinnerItemID != nil {
		for _, item := range m.Items {
			if item.ID == *m.WinnerItemID {
				return &item, nil
			}
		}
		return nil, errors.New("manual winner not found")
	}

	if len(m.Items) != 2 {
		return nil, errors.New("matchup does not have exactly 2 items")
	}

	a := m.Items[0]
	b := m.Items[1]

	if a.Votes == b.Votes {
		return nil, errors.New("matchup is tied")
	}

	if a.Votes > b.Votes {
		return &a, nil
	}
	return &b, nil
}

func (m *Matchup) HasTie() bool {
	if len(m.Items) < 2 {
		return false
	}

	votes := m.Items[0].Votes
	for _, item := range m.Items[1:] {
		if item.Votes != votes {
			return false
		}
	}
	return true
}

func FindBracketRounds(db sqlx.ExtContext, bracketID uint) (map[int][]Matchup, error) {
	matchups, err := FindMatchupsByBracket(db, bracketID)
	if err != nil {
		return nil, err
	}

	rounds := make(map[int][]Matchup)

	for _, m := range matchups {
		if m.Round == nil {
			continue
		}
		round := *m.Round
		rounds[round] = append(rounds[round], m)
	}

	return rounds, nil
}

// LoadMatchupAssociations batch-loads Author and Items for a slice of matchups.
func LoadMatchupAssociations(db sqlx.ExtContext, matchups []Matchup) error {
	if len(matchups) == 0 {
		return nil
	}

	// Collect IDs
	matchupIDs := make([]uint, len(matchups))
	authorIDs := make([]uint, 0, len(matchups))
	authorIDSet := make(map[uint]bool)
	for i, m := range matchups {
		matchupIDs[i] = m.ID
		if !authorIDSet[m.AuthorID] {
			authorIDSet[m.AuthorID] = true
			authorIDs = append(authorIDs, m.AuthorID)
		}
	}

	// Load authors
	authorMap := make(map[uint]User)
	if len(authorIDs) > 0 {
		query, args, err := sqlx.In("SELECT * FROM users WHERE id IN (?)", authorIDs)
		if err != nil {
			return err
		}
		var authors []User
		if err := sqlx.SelectContext(context.Background(), db, &authors, db.Rebind(query), args...); err != nil {
			return err
		}
		for _, a := range authors {
			a.ProcessAvatarPath()
			authorMap[a.ID] = a
		}
	}

	// Load items
	itemMap := make(map[uint][]MatchupItem)
	if len(matchupIDs) > 0 {
		query, args, err := sqlx.In("SELECT * FROM matchup_items WHERE matchup_id IN (?)", matchupIDs)
		if err != nil {
			return err
		}
		var items []MatchupItem
		if err := sqlx.SelectContext(context.Background(), db, &items, db.Rebind(query), args...); err != nil {
			return err
		}
		for _, item := range items {
			itemMap[item.MatchupID] = append(itemMap[item.MatchupID], item)
		}
	}

	// Attach
	for i := range matchups {
		matchups[i].Author = authorMap[matchups[i].AuthorID]
		matchups[i].Items = itemMap[matchups[i].ID]
	}

	return nil
}
