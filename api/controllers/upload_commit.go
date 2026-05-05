package controllers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"

	"connectrpc.com/connect"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// CommitUploadedObject validates a client-supplied S3 key + downloads
// the uploaded bytes so the caller can feed them into the image
// resize pipeline. Centralises the shared logic between UpdateAvatar
// and CreateMatchup's matchup-cover path so there's one place to
// evolve it when we add the next upload kind.
//
// Returns the raw bytes on success. Callers are responsible for:
//   - running resizeAndUpload (or whatever processing the kind needs)
//   - calling DeleteUploadedObject after a successful commit to reap
//     the temp upload (failure paths leave the blob for the 24h S3
//     lifecycle rule to sweep up)
//
// Errors are pre-wrapped with connect.NewError so the caller can just
// return them directly.
func CommitUploadedObject(
	ctx context.Context,
	s3Client *s3.Client,
	bucket, key string,
	kind UploadKind,
	userID uint,
) ([]byte, error) {
	// Validate client inputs BEFORE server-config checks so a user with
	// a bad key gets a clear "your key is wrong" error, not a
	// misleading "server is broken" one. Ordering matters for DX even
	// when prod always has S3 configured.
	if key == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("upload_key is required"))
	}
	// Cross-user forge check — the key carries its own owner in the
	// path, so we never have to ask the DB.
	if err := validateKeyOwnership(key, kind, userID); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	if s3Client == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("upload service unavailable"))
	}
	if bucket == "" {
		return nil, connect.NewError(connect.CodeInternal, errors.New("server configuration error"))
	}

	spec, found := uploadKinds[kind]
	if !found {
		// Defensive: the UploadKind type is closed, so this is unreachable
		// with the current constants. Still, better to return a clean
		// error than panic if a future kind forgets to register a spec.
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("unregistered kind %q", kind))
	}

	// HeadObject — confirm the blob exists + check size / content-type
	// before we download the whole thing. S3 charges for GET bandwidth;
	// rejecting oversized blobs here saves money + catches clients who
	// tried to bypass the client-side size check.
	head, err := s3Client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("uploaded object not found; did the PUT succeed?"))
	}
	if head.ContentLength != nil && *head.ContentLength > spec.MaxBytes {
		return nil, connect.NewError(
			connect.CodeInvalidArgument,
			fmt.Errorf("uploaded object too large (%d bytes, max %d)", *head.ContentLength, spec.MaxBytes),
		)
	}
	if head.ContentType != nil && !spec.AllowedTypes[*head.ContentType] {
		// A mismatch here means the client PUT with a different
		// Content-Type than what they told PresignUpload — S3 should
		// have refused it in the first place, but defense in depth.
		return nil, connect.NewError(
			connect.CodeInvalidArgument,
			fmt.Errorf("uploaded object has disallowed content_type %q", *head.ContentType),
		)
	}

	// Download. No streaming pipe — resizeAndUpload works on a byte
	// slice already, and the size cap we just checked bounds the
	// memory footprint.
	obj, err := s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("fetch uploaded object: %w", err))
	}
	defer obj.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(obj.Body, spec.MaxBytes+1))
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("read uploaded object: %w", err))
	}
	// Belt-and-suspenders: LimitReader caps at MaxBytes+1 so we can
	// detect a lying HeadObject. In practice this only trips if S3
	// somehow reports a smaller ContentLength than the real body.
	if int64(len(raw)) > spec.MaxBytes {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("uploaded object exceeded size cap mid-download"))
	}
	return raw, nil
}

// DeleteUploadedObject reaps the temp upload after the caller has
// successfully processed it. Best-effort — failures are logged but
// not surfaced to the user (the S3 lifecycle rule will mop up any
// orphans after 24h anyway).
func DeleteUploadedObject(ctx context.Context, s3Client *s3.Client, bucket, key string) {
	if s3Client == nil || bucket == "" || key == "" {
		return
	}
	if _, err := s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}); err != nil {
		log.Printf("upload: prune temp object %s: %v", key, err)
	}
}
