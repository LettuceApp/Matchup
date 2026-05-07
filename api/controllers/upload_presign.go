package controllers

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	uploadv1 "Matchup/gen/upload/v1"
	"Matchup/gen/upload/v1/uploadv1connect"
	httpctx "Matchup/utils/httpctx"

	"connectrpc.com/connect"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// UploadHandler brokers direct-to-S3 client uploads. See
// proto/upload/v1/upload.proto for the three-step client flow. The
// handler's only responsibility is PresignUpload — the commit paths
// (UpdateAvatar, CreateMatchup) live in their own handlers and call
// CommitUploadedObject to validate + fetch the blob.
type UploadHandler struct {
	DB       *sqlx.DB
	S3Client *s3.Client
}

var _ uploadv1connect.UploadServiceHandler = (*UploadHandler)(nil)

// UploadKind identifies a class of upload. Kept as a distinct type so
// callers of CommitUploadedObject can't accidentally pass a free-form
// string + skip the registry validation.
type UploadKind string

const (
	UploadKindAvatar       UploadKind = "avatar"
	UploadKindMatchupCover UploadKind = "matchup_cover"
	// Item thumbnails — added migration 026. Smaller cap than matchup
	// covers (items are tile-sized in the contender card layout, so a
	// 2 MB cap is plenty for a sharp upload that resizes well).
	UploadKindMatchupItem UploadKind = "matchup_item"
)

// uploadKindSpec is the per-kind policy: what's allowed + how large.
// Kept private — callers should reference kinds through the typed
// constants above, and size/content-type enforcement lives here.
type uploadKindSpec struct {
	MaxBytes     int64
	AllowedTypes map[string]bool
}

// uploadKinds is the single source of truth for per-kind upload rules.
// Adding a new upload class is one line here + a new UploadKind*
// constant + a new caller of CommitUploadedObject.
var uploadKinds = map[UploadKind]uploadKindSpec{
	UploadKindAvatar: {
		MaxBytes: 500_000,
		AllowedTypes: map[string]bool{
			"image/jpeg": true,
			"image/png":  true,
			"image/webp": true,
			"image/gif":  true,
		},
	},
	UploadKindMatchupCover: {
		MaxBytes: 5_000_000,
		AllowedTypes: map[string]bool{
			"image/jpeg": true,
			"image/png":  true,
			"image/webp": true,
			"image/gif":  true,
		},
	},
	UploadKindMatchupItem: {
		MaxBytes: 2_000_000,
		AllowedTypes: map[string]bool{
			"image/jpeg": true,
			"image/png":  true,
			"image/webp": true,
			"image/gif":  true,
		},
	},
}

// presignedURLTTL bounds how long a signed PUT URL is valid. 5 minutes
// is long enough for slow mobile networks + short enough that a leaked
// URL has bounded blast radius. S3 refuses the PUT past this window.
const presignedURLTTL = 5 * time.Minute

// keyPattern matches the shape our keys take:
//
//	uploads/{kind}/{userID}/{uuid}.bin
//
// The anchors + kind/userID placeholders are filled in per-call in
// validateKeyOwnership so the compiled regex is cheap + reusable.
var keyPattern = regexp.MustCompile(`^uploads/([a-z_]+)/(\d+)/([0-9a-fA-F-]+)\.bin$`)

// PresignUpload generates an S3 PUT-scoped signed URL for the caller.
// The URL is valid for presignedURLTTL seconds and binds Content-Type
// to the value the client declared — a stricter PUT than what the
// client would have signed themselves.
func (h *UploadHandler) PresignUpload(ctx context.Context, req *connect.Request[uploadv1.PresignUploadRequest]) (*connect.Response[uploadv1.PresignUploadResponse], error) {
	uid, ok := httpctx.CurrentUserID(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthorized"))
	}

	kind := UploadKind(req.Msg.GetKind())
	spec, found := uploadKinds[kind]
	if !found {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("unsupported kind: %q", req.Msg.GetKind()))
	}

	contentType := req.Msg.GetContentType()
	if !spec.AllowedTypes[contentType] {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("unsupported content_type: %q", contentType))
	}

	bucket := bucketFromEnv()
	if bucket == "" {
		return nil, connect.NewError(connect.CodeInternal, errors.New("server configuration error"))
	}
	if h.S3Client == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("upload service unavailable"))
	}

	// Embed kind + user_id directly in the key so the commit handler
	// can reject cross-user keys without a DB round-trip.
	key := fmt.Sprintf("uploads/%s/%d/%s.bin", kind, uid, uuid.New().String())

	presigner := s3.NewPresignClient(h.S3Client)
	signed, err := presigner.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(key),
		ContentType: aws.String(contentType),
	}, s3.WithPresignExpires(presignedURLTTL))
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("presign: %w", err))
	}

	return connect.NewResponse(&uploadv1.PresignUploadResponse{
		UploadUrl:        signed.URL,
		Key:              key,
		MaxBytes:         spec.MaxBytes,
		ExpiresInSeconds: int64(presignedURLTTL.Seconds()),
	}), nil
}

// validateKeyOwnership checks a client-supplied S3 key against the
// well-known uploads/{kind}/{uid}/ scheme. Returns (kind, nil) when
// it matches; typed error otherwise.
//
// Kept exported-private so both commit handlers + tests can call it
// without reaching into the regex.
func validateKeyOwnership(key string, expectedKind UploadKind, expectedUID uint) error {
	m := keyPattern.FindStringSubmatch(key)
	if m == nil {
		return errors.New("upload_key has invalid shape")
	}
	gotKind := UploadKind(m[1])
	if gotKind != expectedKind {
		return fmt.Errorf("upload_key kind mismatch: got %q, want %q", gotKind, expectedKind)
	}
	gotUID, err := strconv.ParseUint(m[2], 10, 64)
	if err != nil {
		return errors.New("upload_key has malformed user_id")
	}
	if uint(gotUID) != expectedUID {
		return errors.New("upload_key does not belong to caller")
	}
	return nil
}

// bucketFromEnv pulls the S3 bucket name from env, stripping any
// trailing "/prefix" segment. Matches the idiom already used in the
// existing UpdateAvatar + CreateMatchup handlers.
func bucketFromEnv() string {
	raw := os.Getenv("S3_BUCKET")
	return strings.SplitN(raw, "/", 2)[0]
}
