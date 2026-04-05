package controllers

import (
	"encoding/json"

	"Matchup/gen/admin/v1/adminv1connect"
	"Matchup/gen/auth/v1/authv1connect"
	"Matchup/gen/bracket/v1/bracketv1connect"
	"Matchup/gen/comment/v1/commentv1connect"
	"Matchup/gen/home/v1/homev1connect"
	"Matchup/gen/like/v1/likev1connect"
	"Matchup/gen/matchup/v1/matchupv1connect"
	"Matchup/gen/user/v1/userv1connect"
	"Matchup/middlewares"

	"connectrpc.com/connect"
	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// snakeJSONCodec is a ConnectRPC codec that serialises proto messages using
// snake_case field names (UseProtoNames: true) so the JSON matches the
// frontend's expectations.
type snakeJSONCodec struct{}

func (snakeJSONCodec) Name() string { return "json" }

func (snakeJSONCodec) Marshal(msg any) ([]byte, error) {
	if m, ok := msg.(proto.Message); ok {
		return protojson.MarshalOptions{UseProtoNames: true}.Marshal(m)
	}
	return json.Marshal(msg)
}

func (snakeJSONCodec) Unmarshal(data []byte, msg any) error {
	if m, ok := msg.(proto.Message); ok {
		return protojson.UnmarshalOptions{DiscardUnknown: true}.Unmarshal(data, m)
	}
	return json.Unmarshal(data, msg)
}

var snakeCodec = connect.WithCodec(snakeJSONCodec{})

func initializeConnectRoutes(r chi.Router, db *sqlx.DB) {
	authHandler := &AuthHandler{DB: db}
	userHandler := &UserHandler{DB: db}
	matchupHandler := &MatchupHandler{DB: db}
	matchupItemHandler := &MatchupItemHandler{DB: db}
	bracketHandler := &BracketHandler{DB: db}
	commentHandler := &CommentHandler{DB: db}
	likeHandler := &LikeHandler{DB: db}
	homeHandler := &HomeHandler{DB: db}
	adminHandler := &AdminHandler{DB: db}

	// ---------- Rate-limited auth routes ----------
	r.Group(func(r chi.Router) {
		r.Use(middlewares.LoginRateLimitMiddleware())

		path, h := authv1connect.NewAuthServiceHandler(authHandler, snakeCodec)
		r.Mount(path, h)
	})

	// ---------- Public routes (no auth required) ----------
	r.Group(func(r chi.Router) {
		r.Use(middlewares.SoftJWTMiddleware())

		path, h := homev1connect.NewHomeServiceHandler(homeHandler, snakeCodec)
		r.Mount(path, h)
	})

	// ---------- Mixed auth routes (auth optional — handlers check internally) ----------
	r.Group(func(r chi.Router) {
		r.Use(middlewares.SoftJWTMiddleware())

		path, h := matchupv1connect.NewMatchupServiceHandler(matchupHandler, snakeCodec)
		r.Mount(path, h)

		path, h = matchupv1connect.NewMatchupItemServiceHandler(matchupItemHandler, snakeCodec)
		r.Mount(path, h)

		path, h = bracketv1connect.NewBracketServiceHandler(bracketHandler, snakeCodec)
		r.Mount(path, h)

		path, h = commentv1connect.NewCommentServiceHandler(commentHandler, snakeCodec)
		r.Mount(path, h)

		path, h = likev1connect.NewLikeServiceHandler(likeHandler, snakeCodec)
		r.Mount(path, h)

		path, h = userv1connect.NewUserServiceHandler(userHandler, snakeCodec)
		r.Mount(path, h)
	})

	// ---------- Admin routes ----------
	r.Group(func(r chi.Router) {
		r.Use(middlewares.TokenAuthMiddleware(db))
		r.Use(middlewares.AdminOnlyMiddleware())

		path, h := adminv1connect.NewAdminServiceHandler(adminHandler, snakeCodec)
		r.Mount(path, h)
	})
}
