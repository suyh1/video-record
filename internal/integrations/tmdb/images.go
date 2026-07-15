package tmdb

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"time"
)

const (
	maxImageResponseBytes = 12 << 20
	maxImageSignatureTTL  = 24 * time.Hour
)

var (
	ErrInvalidImage = errors.New("invalid tmdb image")

	allowedImageSizes = map[string]struct{}{
		"w300":  {},
		"w342":  {},
		"w780":  {},
		"w1280": {},
	}
	validImagePath = regexp.MustCompile(`^/[A-Za-z0-9_-]+\.(?:jpg|jpeg|png|webp)$`)
)

type ImageAsset struct {
	ContentType string
	Contents    []byte
}

func (client *Client) SignImage(size, path string, expires time.Time) (string, error) {
	if client.token == "" {
		return "", &ClientError{Kind: ErrNotConfigured}
	}
	now := time.Now()
	if !validImageRequest(size, path) || !expires.After(now) || expires.After(now.Add(maxImageSignatureTTL)) {
		return "", &ClientError{Kind: ErrInvalidImage}
	}
	return client.imageSignature(size, path, expires), nil
}

func (client *Client) VerifyImage(size, path string, expires time.Time, signature string) bool {
	now := time.Now()
	if client.token == "" || !validImageRequest(size, path) || !expires.After(now) ||
		expires.After(now.Add(maxImageSignatureTTL)) {
		return false
	}
	provided, err := hex.DecodeString(signature)
	if err != nil || len(provided) != sha256.Size {
		return false
	}
	expected, err := hex.DecodeString(client.imageSignature(size, path, expires))
	return err == nil && hmac.Equal(provided, expected)
}

func (client *Client) Image(ctx context.Context, size, path string) (ImageAsset, error) {
	if !validImageRequest(size, path) {
		return ImageAsset{}, &ClientError{Kind: ErrInvalidImage}
	}
	endpoint, err := url.Parse(client.imageBaseURL + "/" + size + path)
	if err != nil {
		return ImageAsset{}, &ClientError{Kind: ErrUpstreamUnavailable}
	}
	requestCtx, cancel := context.WithTimeout(ctx, client.timeout)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return ImageAsset{}, &ClientError{Kind: ErrUpstreamUnavailable}
	}
	request.Header.Set("Accept", "image/jpeg, image/png, image/webp")

	response, err := client.httpClient.Do(request)
	if err != nil {
		return ImageAsset{}, client.imageRequestError(ctx, requestCtx, 0)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode == http.StatusTooManyRequests {
		retryAfter := parseRetryAfter(response.Header.Get("Retry-After"), time.Now())
		client.logFailure(ctx, "rate_limited", response.StatusCode)
		return ImageAsset{}, &ClientError{Kind: ErrRateLimited, RetryAfter: retryAfter}
	}
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		client.logFailure(ctx, "unauthorized", response.StatusCode)
		return ImageAsset{}, &ClientError{Kind: ErrUnauthorized}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		client.logFailure(ctx, "unavailable", response.StatusCode)
		return ImageAsset{}, &ClientError{Kind: ErrUpstreamUnavailable}
	}
	contentType := response.Header.Get("Content-Type")
	if !allowedImageContentType(contentType) || response.ContentLength > maxImageResponseBytes {
		client.logFailure(ctx, "invalid_response", response.StatusCode)
		return ImageAsset{}, &ClientError{Kind: ErrUpstreamUnavailable}
	}
	contents, err := io.ReadAll(io.LimitReader(response.Body, maxImageResponseBytes+1))
	if err != nil {
		return ImageAsset{}, client.imageRequestError(ctx, requestCtx, response.StatusCode)
	}
	if len(contents) > maxImageResponseBytes {
		client.logFailure(ctx, "invalid_response", response.StatusCode)
		return ImageAsset{}, &ClientError{Kind: ErrUpstreamUnavailable}
	}
	return ImageAsset{ContentType: contentType, Contents: contents}, nil
}

func (client *Client) imageSignature(size, path string, expires time.Time) string {
	key := sha256.Sum256([]byte(client.token))
	mac := hmac.New(sha256.New, key[:])
	_, _ = fmt.Fprintf(mac, "%s\n%s\n%d", size, path, expires.Unix())
	return hex.EncodeToString(mac.Sum(nil))
}

func (client *Client) imageRequestError(ctx, requestCtx context.Context, status int) error {
	if errors.Is(requestCtx.Err(), context.DeadlineExceeded) {
		client.logFailure(ctx, "timeout", status)
		return &ClientError{Kind: ErrUpstreamTimeout}
	}
	client.logFailure(ctx, "unavailable", status)
	return &ClientError{Kind: ErrUpstreamUnavailable}
}

func validImageRequest(size, path string) bool {
	_, sizeAllowed := allowedImageSizes[size]
	return sizeAllowed && validImagePath.MatchString(path)
}

func allowedImageContentType(contentType string) bool {
	switch contentType {
	case "image/jpeg", "image/png", "image/webp":
		return true
	default:
		return false
	}
}
