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

func resolveUserByIdentifier(db sqlx.ExtContext, identifier string) (*models.User, error) {
	trimmed := strings.TrimSpace(identifier)
	if trimmed == "" {
		return nil, sql.ErrNoRows
	}

	var user models.User
	if isUUIDLike(trimmed) {
		err := sqlx.GetContext(context.Background(), db, &user, "SELECT * FROM users WHERE public_id::text = $1", trimmed)
		if err == nil {
			user.ProcessAvatarPath()
			return &user, nil
		} else if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}

	err := sqlx.GetContext(context.Background(), db, &user, "SELECT * FROM users WHERE lower(username) = lower($1)", trimmed)
	if err == nil {
		user.ProcessAvatarPath()
		return &user, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	if numericID, err := strconv.ParseUint(trimmed, 10, 32); err == nil {
		if err := sqlx.GetContext(context.Background(), db, &user, "SELECT * FROM users WHERE id = $1", uint(numericID)); err == nil {
			user.ProcessAvatarPath()
			return &user, nil
		}
	}

	err = sqlx.GetContext(context.Background(), db, &user, "SELECT * FROM users WHERE public_id::text = $1", trimmed)
	if err != nil {
		return nil, err
	}

	user.ProcessAvatarPath()
	return &user, nil
}
