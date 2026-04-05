package controllers

import (
	"context"

	"github.com/jmoiron/sqlx"
)

type idPublicPair struct {
	ID       uint   `db:"id"`
	PublicID string `db:"public_id"`
}

func loadPublicIDMap(db sqlx.ExtContext, table string, ids []uint) map[uint]string {
	if len(ids) == 0 {
		return map[uint]string{}
	}

	query, args, err := sqlx.In("SELECT id, public_id FROM "+table+" WHERE id IN (?)", ids)
	if err != nil {
		return map[uint]string{}
	}

	var rows []idPublicPair
	if err := sqlx.SelectContext(context.Background(), db, &rows, db.Rebind(query), args...); err != nil {
		return map[uint]string{}
	}

	result := make(map[uint]string, len(rows))
	for _, row := range rows {
		result[row.ID] = row.PublicID
	}
	return result
}

func loadUserPublicIDMap(db sqlx.ExtContext, ids []uint) map[uint]string {
	return loadPublicIDMap(db, "users", ids)
}

func loadMatchupPublicIDMap(db sqlx.ExtContext, ids []uint) map[uint]string {
	return loadPublicIDMap(db, "matchups", ids)
}

func loadBracketPublicIDMap(db sqlx.ExtContext, ids []uint) map[uint]string {
	return loadPublicIDMap(db, "brackets", ids)
}

func loadMatchupItemPublicIDMap(db sqlx.ExtContext, ids []uint) map[uint]string {
	return loadPublicIDMap(db, "matchup_items", ids)
}
