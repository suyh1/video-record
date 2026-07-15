package tmdb

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSignedImageAcceptsOnlySupportedInputs(t *testing.T) {
	client := NewClient(ClientOptions{Token: "synthetic-token"})
	expires := time.Now().Add(time.Hour).Truncate(time.Second)

	for _, size := range []string{"w300", "w342", "w780", "w1280"} {
		t.Run(size, func(t *testing.T) {
			signature, err := client.SignImage(size, "/arrival.jpg", expires)
			require.NoError(t, err)
			require.Equal(t, testImageSignature("synthetic-token", size, "/arrival.jpg", expires), signature)
			require.True(t, client.VerifyImage(size, "/arrival.jpg", expires, signature))
		})
	}

	for _, test := range []struct {
		name string
		size string
		path string
	}{
		{name: "empty size", path: "/arrival.jpg"},
		{name: "unsupported size", size: "original", path: "/arrival.jpg"},
		{name: "empty path", size: "w1280"},
		{name: "missing leading slash", size: "w1280", path: "arrival.jpg"},
		{name: "empty filename", size: "w1280", path: "/.jpg"},
		{name: "nested path", size: "w1280", path: "/posters/arrival.jpg"},
		{name: "path traversal", size: "w1280", path: "/../arrival.jpg"},
		{name: "encoded traversal", size: "w1280", path: "/%2e%2e.jpg"},
		{name: "query material", size: "w1280", path: "/arrival.jpg?x=1"},
		{name: "uppercase extension", size: "w1280", path: "/arrival.JPG"},
		{name: "unsupported extension", size: "w1280", path: "/arrival.gif"},
		{name: "non ascii filename", size: "w1280", path: "/降临.jpg"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := client.SignImage(test.size, test.path, expires)
			require.ErrorIs(t, err, ErrInvalidImage)
			require.False(t, client.VerifyImage(test.size, test.path, expires, "anything"))
		})
	}
}

func TestSignedImageRejectsMissingTokenTamperingAndInvalidExpiry(t *testing.T) {
	expires := time.Now().Add(time.Hour).Truncate(time.Second)
	client := NewClient(ClientOptions{Token: "synthetic-token"})
	signature, err := client.SignImage("w1280", "/arrival.webp", expires)
	require.NoError(t, err)

	require.False(t, client.VerifyImage("w780", "/arrival.webp", expires, signature))
	require.False(t, client.VerifyImage("w1280", "/changed.webp", expires, signature))
	require.False(t, client.VerifyImage("w1280", "/arrival.webp", expires.Add(time.Second), signature))
	require.False(t, client.VerifyImage("w1280", "/arrival.webp", expires, "not-hex"))

	past := time.Now().Add(-time.Minute).Truncate(time.Second)
	pastSignature := testImageSignature("synthetic-token", "w1280", "/arrival.webp", past)
	require.False(t, client.VerifyImage("w1280", "/arrival.webp", past, pastSignature))

	_, err = client.SignImage("w1280", "/arrival.webp", time.Now().Add(maxImageSignatureTTL+time.Hour))
	require.ErrorIs(t, err, ErrInvalidImage)

	emptyTokenClient := NewClient(ClientOptions{})
	_, err = emptyTokenClient.SignImage("w1280", "/arrival.webp", expires)
	require.ErrorIs(t, err, ErrNotConfigured)
	require.False(t, emptyTokenClient.VerifyImage("w1280", "/arrival.webp", expires, signature))
}

func TestSignedImageUsesConfiguredClockForExpiryValidation(t *testing.T) {
	now := time.Date(2020, time.January, 2, 3, 4, 5, 0, time.UTC)
	client := NewClient(ClientOptions{
		Token: "synthetic-token",
		Now:   func() time.Time { return now },
	})
	expires := now.Add(maxImageSignatureTTL)

	signature, err := client.SignImage("w1280", "/historical.jpg", expires)

	require.NoError(t, err)
	require.True(t, client.VerifyImage("w1280", "/historical.jpg", expires, signature))
	now = expires.Add(time.Nanosecond)
	require.False(t, client.VerifyImage("w1280", "/historical.jpg", expires, signature))
}

func TestImageUsesConfiguredBaseURLWithoutBearerToken(t *testing.T) {
	contents := []byte("jpeg-image")
	type requestSnapshot struct {
		path          string
		rawQuery      string
		authorization string
		accept        string
	}
	requestReceived := make(chan requestSnapshot, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestReceived <- requestSnapshot{
			path:          r.URL.EscapedPath(),
			rawQuery:      r.URL.RawQuery,
			authorization: r.Header.Get("Authorization"),
			accept:        r.Header.Get("Accept"),
		}
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(contents)
	}))
	t.Cleanup(server.Close)
	client := NewClient(ClientOptions{
		ImageBaseURL: server.URL + "/t/p/",
		Token:        "must-not-be-sent",
	})

	asset, err := client.Image(context.Background(), "w1280", "/arrival.jpg")

	require.NoError(t, err)
	request := <-requestReceived
	require.Equal(t, "/t/p/w1280/arrival.jpg", request.path)
	require.Empty(t, request.rawQuery)
	require.Empty(t, request.authorization)
	require.Equal(t, "image/jpeg, image/png, image/webp", request.accept)
	require.Equal(t, "image/jpeg", asset.ContentType)
	require.Equal(t, contents, asset.Contents)
}

func TestImageBaseURLDefaultsToTMDB(t *testing.T) {
	client := NewClient(ClientOptions{Token: "synthetic-token"})
	require.Equal(t, "https://image.tmdb.org/t/p", client.imageBaseURL)
}

func TestImageAcceptsSupportedContentTypes(t *testing.T) {
	for _, contentType := range []string{"image/jpeg", "image/png", "image/webp"} {
		t.Run(contentType, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", contentType)
				_, _ = w.Write([]byte("image"))
			}))
			t.Cleanup(server.Close)
			client := NewClient(ClientOptions{ImageBaseURL: server.URL, Token: "synthetic-token"})

			asset, err := client.Image(context.Background(), "w342", "/poster.png")

			require.NoError(t, err)
			require.Equal(t, contentType, asset.ContentType)
			require.Equal(t, []byte("image"), asset.Contents)
		})
	}
}

func TestImageRejectsInvalidInputsWithoutRequestingUpstream(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests++
	}))
	t.Cleanup(server.Close)
	client := NewClient(ClientOptions{ImageBaseURL: server.URL, Token: "synthetic-token"})

	for _, test := range []struct {
		size string
		path string
	}{
		{size: "original", path: "/arrival.jpg"},
		{size: "w1280", path: "arrival.jpg"},
		{size: "w1280", path: "/nested/arrival.jpg"},
		{size: "w1280", path: "/../arrival.jpg"},
		{size: "w1280", path: "/arrival.bmp"},
	} {
		_, err := client.Image(context.Background(), test.size, test.path)
		require.ErrorIs(t, err, ErrInvalidImage)
	}
	require.Zero(t, requests)
}

func TestImageMapsUpstreamFailuresToStableErrors(t *testing.T) {
	for _, test := range []struct {
		name       string
		statusCode int
		headers    map[string]string
		expected   error
	}{
		{name: "not found", statusCode: http.StatusNotFound, expected: ErrUpstreamUnavailable},
		{name: "unauthorized", statusCode: http.StatusUnauthorized, expected: ErrUnauthorized},
		{
			name:       "rate limited",
			statusCode: http.StatusTooManyRequests,
			headers:    map[string]string{"Retry-After": "120"},
			expected:   ErrRateLimited,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				for name, value := range test.headers {
					w.Header().Set(name, value)
				}
				w.WriteHeader(test.statusCode)
				_, _ = w.Write([]byte("must not leak"))
			}))
			t.Cleanup(server.Close)
			client := NewClient(ClientOptions{ImageBaseURL: server.URL, Token: "synthetic-token"})

			_, err := client.Image(context.Background(), "w300", "/profile.jpeg")

			require.ErrorIs(t, err, test.expected)
			require.NotContains(t, err.Error(), "must not leak")
			if test.expected == ErrRateLimited {
				var clientError *ClientError
				require.ErrorAs(t, err, &clientError)
				require.Equal(t, 2*time.Minute, clientError.RetryAfter)
			}
		})
	}
}

func TestImageRejectsRedirectsWithoutContactingTheirTarget(t *testing.T) {
	redirectTargetContacted := make(chan struct{}, 1)
	redirectTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		redirectTargetContacted <- struct{}{}
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("redirected image"))
	}))
	t.Cleanup(redirectTarget.Close)
	imageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, redirectTarget.URL+"/outside.jpg", http.StatusFound)
	}))
	t.Cleanup(imageServer.Close)
	sharedHTTPClient := &http.Client{}
	client := NewClient(ClientOptions{
		ImageBaseURL: imageServer.URL,
		Token:        "synthetic-token",
		HTTPClient:   sharedHTTPClient,
	})

	_, err := client.Image(context.Background(), "w1280", "/arrival.jpg")

	require.ErrorIs(t, err, ErrUpstreamUnavailable)
	select {
	case <-redirectTargetContacted:
		t.Fatal("image client followed a redirect outside ImageBaseURL")
	default:
	}
	require.Nil(t, sharedHTTPClient.CheckRedirect, "the caller-provided HTTP client must not be mutated")
}

func TestImageRejectsWrongContentTypeAndOversizedResponses(t *testing.T) {
	t.Run("wrong content type", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<html>not an image</html>"))
		}))
		t.Cleanup(server.Close)
		client := NewClient(ClientOptions{ImageBaseURL: server.URL, Token: "synthetic-token"})

		_, err := client.Image(context.Background(), "w780", "/still.webp")

		require.ErrorIs(t, err, ErrUpstreamUnavailable)
		require.NotContains(t, err.Error(), "not an image")
	})

	t.Run("content type parameters are not accepted", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "image/jpeg; charset=utf-8")
			_, _ = w.Write([]byte("image"))
		}))
		t.Cleanup(server.Close)
		client := NewClient(ClientOptions{ImageBaseURL: server.URL, Token: "synthetic-token"})

		_, err := client.Image(context.Background(), "w780", "/still.jpg")

		require.ErrorIs(t, err, ErrUpstreamUnavailable)
	})

	t.Run("oversized response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "image/webp")
			w.Header().Set("Content-Length", strconv.Itoa(maxImageResponseBytes+1))
			_, _ = w.Write(bytes.Repeat([]byte{'x'}, maxImageResponseBytes+1))
		}))
		t.Cleanup(server.Close)
		client := NewClient(ClientOptions{ImageBaseURL: server.URL, Token: "synthetic-token"})

		_, err := client.Image(context.Background(), "w1280", "/large.webp")

		require.ErrorIs(t, err, ErrUpstreamUnavailable)
	})

	t.Run("oversized chunked response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "image/webp")
			w.WriteHeader(http.StatusOK)
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			_, _ = w.Write(bytes.Repeat([]byte{'x'}, maxImageResponseBytes+1))
		}))
		t.Cleanup(server.Close)
		contentLength := make(chan int64, 1)
		httpClient := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			response, err := http.DefaultTransport.RoundTrip(request)
			if response != nil {
				contentLength <- response.ContentLength
			}
			return response, err
		})}
		client := NewClient(ClientOptions{
			ImageBaseURL: server.URL,
			Token:        "synthetic-token",
			HTTPClient:   httpClient,
		})

		_, err := client.Image(context.Background(), "w1280", "/large.webp")

		require.Equal(t, int64(-1), <-contentLength)
		require.ErrorIs(t, err, ErrUpstreamUnavailable)
	})
}

func TestImageMapsTimeoutAndCallerCancellation(t *testing.T) {
	t.Run("transport deadline exceeded", func(t *testing.T) {
		httpClient := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, fmt.Errorf("transport failed: %w", context.DeadlineExceeded)
		})}
		client := NewClient(ClientOptions{
			ImageBaseURL: "https://images.example.test/t/p",
			Token:        "synthetic-token",
			HTTPClient:   httpClient,
			Timeout:      time.Hour,
		})

		_, err := client.Image(context.Background(), "w1280", "/slow.jpg")

		require.ErrorIs(t, err, ErrUpstreamTimeout)
	})

	t.Run("client timeout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			time.Sleep(100 * time.Millisecond)
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("late image"))
		}))
		t.Cleanup(server.Close)
		client := NewClient(ClientOptions{
			ImageBaseURL: server.URL,
			Token:        "synthetic-token",
			Timeout:      20 * time.Millisecond,
		})

		_, err := client.Image(context.Background(), "w1280", "/slow.jpg")

		require.ErrorIs(t, err, ErrUpstreamTimeout)
	})

	t.Run("caller cancellation stops the upstream read", func(t *testing.T) {
		started := make(chan struct{})
		upstreamCanceled := make(chan struct{})
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "image/jpeg")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("partial"))
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			close(started)
			<-r.Context().Done()
			close(upstreamCanceled)
		}))
		t.Cleanup(server.Close)
		client := NewClient(ClientOptions{ImageBaseURL: server.URL, Token: "synthetic-token"})
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		result := make(chan error, 1)
		go func() {
			_, err := client.Image(ctx, "w1280", "/cancel.jpg")
			result <- err
		}()

		<-started
		cancel()

		require.ErrorIs(t, <-result, ErrUpstreamUnavailable)
		select {
		case <-upstreamCanceled:
		case <-time.After(time.Second):
			t.Fatal("upstream request was not canceled")
		}
	})
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func testImageSignature(token, size, path string, expires time.Time) string {
	key := sha256.Sum256([]byte(token))
	mac := hmac.New(sha256.New, key[:])
	_, _ = fmt.Fprintf(mac, "%s\n%s\n%d", size, path, expires.Unix())
	return hex.EncodeToString(mac.Sum(nil))
}
