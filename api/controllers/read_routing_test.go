package controllers

import (
	"context"
	"database/sql"
	"net/http"
	"testing"

	httpctx "Matchup/utils/httpctx"

	"github.com/jmoiron/sqlx"
)

func TestDbForRead(t *testing.T) {
	primary := sqlx.NewDb(new(sql.DB), "postgres")
	replica := sqlx.NewDb(new(sql.DB), "postgres")

	t.Run("normal context returns replica", func(t *testing.T) {
		ctx := context.Background()
		got := dbForRead(ctx, primary, replica)
		if got != replica {
			t.Errorf("dbForRead with normal context returned primary, want replica")
		}
	})

	t.Run("read-primary context returns primary", func(t *testing.T) {
		ctx := httpctx.WithReadPrimary(context.Background())
		got := dbForRead(ctx, primary, replica)
		if got != primary {
			t.Errorf("dbForRead with read-primary context returned replica, want primary")
		}
	})
}

func TestSetReadPrimaryCookie(t *testing.T) {
	t.Run("sets correct Set-Cookie header", func(t *testing.T) {
		h := make(http.Header)
		setReadPrimaryCookie(h)
		got := h.Get("Set-Cookie")
		want := "_rwp=1; Max-Age=5; Path=/; HttpOnly; SameSite=Strict"
		if got != want {
			t.Errorf("setReadPrimaryCookie header = %q, want %q", got, want)
		}
	})
}
