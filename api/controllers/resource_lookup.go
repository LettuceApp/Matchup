package controllers

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"

	"Matchup/models"

	"github.com/jmoiron/sqlx"
)

var errInvalidIdentifier = errors.New("invalid identifier")

func resolveMatchupByIdentifier(db sqlx.ExtContext, identifier string) (*models.Matchup, error) {
	trimmed := strings.TrimSpace(identifier)
	if trimmed == "" {
		return nil, errInvalidIdentifier
	}
	var matchup models.Matchup
	err := sqlx.GetContext(context.Background(), db, &matchup, "SELECT * FROM matchups WHERE public_id::text = $1", trimmed)
	if err == nil {
		return &matchup, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if numericID, err := strconv.ParseUint(trimmed, 10, 32); err == nil {
		if err := sqlx.GetContext(context.Background(), db, &matchup, "SELECT * FROM matchups WHERE id = $1", uint(numericID)); err != nil {
			return nil, err
		}
		return &matchup, nil
	}
	return nil, sql.ErrNoRows
}

func resolveBracketByIdentifier(db sqlx.ExtContext, identifier string) (*models.Bracket, error) {
	trimmed := strings.TrimSpace(identifier)
	if trimmed == "" {
		return nil, errInvalidIdentifier
	}
	var bracket models.Bracket
	err := sqlx.GetContext(context.Background(), db, &bracket, "SELECT * FROM brackets WHERE public_id::text = $1", trimmed)
	if err == nil {
		return &bracket, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if numericID, err := strconv.ParseUint(trimmed, 10, 32); err == nil {
		if err := sqlx.GetContext(context.Background(), db, &bracket, "SELECT * FROM brackets WHERE id = $1", uint(numericID)); err != nil {
			return nil, err
		}
		return &bracket, nil
	}
	return nil, sql.ErrNoRows
}

func resolveMatchupItemByIdentifier(db sqlx.ExtContext, identifier string) (*models.MatchupItem, error) {
	trimmed := strings.TrimSpace(identifier)
	if trimmed == "" {
		return nil, errInvalidIdentifier
	}
	var item models.MatchupItem
	err := sqlx.GetContext(context.Background(), db, &item, "SELECT * FROM matchup_items WHERE public_id::text = $1", trimmed)
	if err == nil {
		return &item, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if numericID, err := strconv.ParseUint(trimmed, 10, 32); err == nil {
		if err := sqlx.GetContext(context.Background(), db, &item, "SELECT * FROM matchup_items WHERE id = $1", uint(numericID)); err != nil {
			return nil, err
		}
		return &item, nil
	}
	return nil, sql.ErrNoRows
}

func resolveCommentByIdentifier(db sqlx.ExtContext, identifier string) (*models.Comment, error) {
	trimmed := strings.TrimSpace(identifier)
	if trimmed == "" {
		return nil, errInvalidIdentifier
	}
	var comment models.Comment
	err := sqlx.GetContext(context.Background(), db, &comment, "SELECT * FROM comments WHERE public_id::text = $1", trimmed)
	if err == nil {
		return &comment, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if numericID, err := strconv.ParseUint(trimmed, 10, 32); err == nil {
		if err := sqlx.GetContext(context.Background(), db, &comment, "SELECT * FROM comments WHERE id = $1", uint(numericID)); err != nil {
			return nil, err
		}
		return &comment, nil
	}
	return nil, sql.ErrNoRows
}

func resolveBracketCommentByIdentifier(db sqlx.ExtContext, identifier string) (*models.BracketComment, error) {
	trimmed := strings.TrimSpace(identifier)
	if trimmed == "" {
		return nil, errInvalidIdentifier
	}
	var comment models.BracketComment
	err := sqlx.GetContext(context.Background(), db, &comment, "SELECT * FROM bracket_comments WHERE public_id::text = $1", trimmed)
	if err == nil {
		return &comment, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if numericID, err := strconv.ParseUint(trimmed, 10, 32); err == nil {
		if err := sqlx.GetContext(context.Background(), db, &comment, "SELECT * FROM bracket_comments WHERE id = $1", uint(numericID)); err != nil {
			return nil, err
		}
		return &comment, nil
	}
	return nil, sql.ErrNoRows
}
