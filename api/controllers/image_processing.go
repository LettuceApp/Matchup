package controllers

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"strings"
	"time"

	aws2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/disintegration/imaging"

	// Decoder side-effect imports for the formats we accept on upload.
	// imaging.Decode dispatches to the registered decoders.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
)

// imageVariant describes one of the resized output sizes uploaded to
// S3 alongside the original. The suffix is appended to the base S3 key
// (before the file extension) so the frontend can request a specific
// size by URL convention.
type imageVariant struct {
	// suffix is the bit appended to the base S3 key. Empty string means
	// "this is the full-size variant — keep the original key".
	suffix string
	// width is the target width in pixels. Height is computed to
	// preserve aspect ratio. Zero means "do not resize".
	width int
}

// imageVariants is the canonical resize ladder used everywhere.
//
// Three sizes is enough for the current frontend:
//   - thumb (100w)  — list cards, avatars
//   - medium (300w) — detail page hero, profile views
//   - full (no suffix) — full-resolution downloads
//
// Adding more sizes is harmless but increases per-upload S3 traffic, so
// keep this list short.
var imageVariants = []imageVariant{
	{suffix: "_thumb", width: 100},
	{suffix: "_medium", width: 300},
	{suffix: "", width: 0}, // full-size original
}

// resizeSem limits concurrent image resize operations per pod. Lanczos
// resampling is CPU-intensive (3 variants × decode + resize + encode);
// without a cap, a burst of concurrent uploads can starve other
// goroutines and pin the pod's CPU. Two concurrent resizes per pod is
// ample for normal traffic while preventing the DoS vector.
var resizeSem = make(chan struct{}, 2)

// resizeAndUpload decodes one image and uploads three variants
// (thumb / medium / full) to S3 under derived keys.
//
// keyBase must be the S3 key WITHOUT a file extension. The function
// adds an "_thumb" / "_medium" suffix and a ".jpg" extension to the
// base — every variant is JPEG-encoded so the frontend can request a
// stable URL pattern regardless of the upload's original format.
//
// Returns the keys of every variant uploaded, in the same order as
// imageVariants. Callers usually only persist the full-size key in the
// DB and reconstruct the others by suffix on read.
func resizeAndUpload(ctx context.Context, s3Client *s3.Client, bucket, keyBase string, src []byte) ([]string, error) {
	if s3Client == nil {
		return nil, fmt.Errorf("s3 client not initialized")
	}
	if bucket == "" {
		return nil, fmt.Errorf("s3 bucket not configured")
	}
	if len(src) == 0 {
		return nil, fmt.Errorf("empty source image")
	}

	// Acquire resize semaphore. If all slots are in use, wait up to
	// 10s before returning a transient error — the caller surfaces
	// this as a retriable connect.CodeResourceExhausted to the client.
	select {
	case resizeSem <- struct{}{}:
		defer func() { <-resizeSem }()
	case <-time.After(10 * time.Second):
		return nil, fmt.Errorf("image resize busy, try again")
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	img, _, err := image.Decode(bytes.NewReader(src))
	if err != nil {
		return nil, fmt.Errorf("decode source image: %w", err)
	}

	// Bound the entire upload sequence at 30s. Three sequential
	// PutObjects to the same region/bucket are normally well under
	// this; the cap exists to ensure a stuck S3 connection can never
	// permanently block a request goroutine.
	uploadCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	keys := make([]string, 0, len(imageVariants))
	for _, v := range imageVariants {
		var resized image.Image
		if v.width > 0 && img.Bounds().Dx() > v.width {
			resized = imaging.Resize(img, v.width, 0, imaging.Lanczos)
		} else {
			// No need to resize — either the variant is "full size" or
			// the original is already smaller than the target width.
			resized = img
		}

		var buf bytes.Buffer
		if err := imaging.Encode(&buf, resized, imaging.JPEG, imaging.JPEGQuality(85)); err != nil {
			return nil, fmt.Errorf("encode %s variant: %w", v.suffix, err)
		}

		key := buildVariantKey(keyBase, v.suffix)
		body := buf.Bytes()
		if _, err := s3Client.PutObject(uploadCtx, &s3.PutObjectInput{
			Bucket:        aws2.String(bucket),
			Key:           aws2.String(key),
			Body:          bytes.NewReader(body),
			ContentLength: aws2.Int64(int64(len(body))),
			ContentType:   aws2.String("image/jpeg"),
		}); err != nil {
			return nil, fmt.Errorf("upload %s variant: %w", key, err)
		}
		keys = append(keys, key)
	}
	return keys, nil
}

// buildVariantKey appends a size suffix to a base S3 key and forces a
// .jpg extension. The base key may already include an extension (left
// over from the upload's original filename); we strip it so we don't
// end up with "/foo.png_thumb.jpg".
func buildVariantKey(keyBase, suffix string) string {
	// Strip a single trailing extension if present. We are not trying
	// to be clever about double-extensions ("foo.tar.gz") — these keys
	// are generated server-side and only ever have one.
	if dot := strings.LastIndex(keyBase, "."); dot > strings.LastIndex(keyBase, "/") {
		keyBase = keyBase[:dot]
	}
	return keyBase + suffix + ".jpg"
}
