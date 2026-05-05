package controllers

import (
	"encoding/json"

	"Matchup/gen/activity/v1/activityv1connect"
	"Matchup/gen/report/v1/reportv1connect"
	"Matchup/gen/upload/v1/uploadv1connect"
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
	"github.com/aws/aws-sdk-go-v2/service/s3"
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

func initializeConnectRoutes(r chi.Router, db, readDB *sqlx.DB, s3Client *s3.Client) {
	authHandler := &AuthHandler{DB: db}
	userHandler := &UserHandler{DB: db, ReadDB: readDB, S3Client: s3Client}
	matchupHandler := &MatchupHandler{DB: db, ReadDB: readDB, S3Client: s3Client}
	matchupItemHandler := &MatchupItemHandler{DB: db, ReadDB: readDB}
	bracketHandler := &BracketHandler{DB: db, ReadDB: readDB}
	commentHandler := &CommentHandler{DB: db, ReadDB: readDB}
	likeHandler := &LikeHandler{DB: db, ReadDB: readDB}
	homeHandler := &HomeHandler{DB: db, ReadDB: readDB}
	activityHandler := &ActivityHandler{DB: db, ReadDB: readDB}
	uploadHandler := &UploadHandler{DB: db, S3Client: s3Client}
	reportHandler := &ReportHandler{DB: db}
	adminHandler := &AdminHandler{DB: db, ReadDB: readDB}

	// ---------- Rate-limited auth routes ----------
	// AuthService mounts as one handler; SoftJWT is harmless for Login
	// / Forgot / Reset / Refresh (they don't require auth) and lets
	// Logout + LogoutAll read the viewer's uid to scope the revoke.
	// The handlers enforce "uid required" internally for those two.
	r.Group(func(r chi.Router) {
		r.Use(middlewares.LoginRateLimitMiddleware())
		r.Use(middlewares.SoftJWTMiddleware())

		path, h := authv1connect.NewAuthServiceHandler(authHandler, snakeCodec)
		r.Mount(path, h)
	})

	// ---------- Public routes (no auth required) ----------
	r.Group(func(r chi.Router) {
		r.Use(middlewares.SoftJWTMiddleware())

		path, h := homev1connect.NewHomeServiceHandler(homeHandler, snakeCodec)
		r.Mount(path, h)

		// One-click unsubscribe for weekly email digests. Auth comes
		// from the HMAC-signed token in the URL, not the session —
		// users don't need to be logged in to opt out from their
		// inbox client.
		r.Get("/email/unsubscribe/{token}", EmailUnsubscribeHandler(db))
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

		r.Get("/brackets/{bracketID}/events", bracketHandler.BracketEventsSSE)

		path, h = commentv1connect.NewCommentServiceHandler(commentHandler, snakeCodec)
		r.Mount(path, h)

		path, h = likev1connect.NewLikeServiceHandler(likeHandler, snakeCodec)
		r.Mount(path, h)

		path, h = userv1connect.NewUserServiceHandler(userHandler, snakeCodec)
		r.Mount(path, h)

		path, h = activityv1connect.NewActivityServiceHandler(activityHandler, snakeCodec)
		r.Mount(path, h)

		// UploadService brokers presigned S3 PUT URLs so clients upload
		// blobs directly to S3 instead of base64-ing them through the API.
		// Auth is enforced inside the handler (the presign includes the
		// caller's user_id in the key, which the commit RPCs then cross-
		// check).
		path, h = uploadv1connect.NewUploadServiceHandler(uploadHandler, snakeCodec)
		r.Mount(path, h)

		// ReportService — user-submitted content reports. Rate-limited
		// per-user inside the handler so a disgruntled account can't
		// flood the moderation queue.
		path, h = reportv1connect.NewReportServiceHandler(reportHandler, snakeCodec)
		r.Mount(path, h)
	})

	// ---------- Auth-required SSE routes ----------
	// Activity events stream is per-user and must be auth-required (we
	// derive the Redis channel from the session uid). TokenAuthMiddleware
	// rejects anonymous callers with 401 before the handler runs.
	r.Group(func(r chi.Router) {
		r.Use(middlewares.TokenAuthMiddleware(db))
		r.Get("/users/me/activity/events", activityHandler.ActivityEventsSSE)
	})

	// ---------- Admin routes ----------
	r.Group(func(r chi.Router) {
		r.Use(middlewares.TokenAuthMiddleware(db))
		r.Use(middlewares.AdminOnlyMiddleware())

		path, h := adminv1connect.NewAdminServiceHandler(adminHandler, snakeCodec)
		r.Mount(path, h)
	})
}
