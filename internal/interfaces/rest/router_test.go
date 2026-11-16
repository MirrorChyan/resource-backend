package rest

import (
	"io"
	"net"
	"net/http"
	"testing"

	"github.com/MirrorChyan/resource-backend/internal/interfaces/rest/handler"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"
)

// TestRouterConfig pins the behavior that fiber v3 changed by default and NewRouter restores.
func TestRouterConfig(t *testing.T) {
	app := NewRouter()
	app.Get("/ip", func(c fiber.Ctx) error { return c.SendString(c.IP()) })
	handler.NewMetricsHandler().Register(app)

	// a real listener, as the trusted proxy check needs a loopback peer
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	go func() { _ = app.Listener(ln, fiber.ListenConfig{DisableStartupMessage: true}) }()
	t.Cleanup(func() { _ = app.Shutdown() })
	base := "http://" + ln.Addr().String()

	get := func(t *testing.T, req *http.Request) (int, string) {
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		return resp.StatusCode, string(body)
	}

	t.Run("client IP is the raw X-Forwarded-For value", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, base+"/ip", nil)
		require.NoError(t, err)
		req.Header.Set(fiber.HeaderXForwardedFor, "203.0.113.7, 198.51.100.2")
		status, body := get(t, req)
		require.Equal(t, http.StatusOK, status)
		require.Equal(t, "203.0.113.7, 198.51.100.2", body)
	})

	t.Run("a GET route does not answer HEAD", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodHead, base+"/ip", nil)
		require.NoError(t, err)
		status, _ := get(t, req)
		require.Equal(t, http.StatusMethodNotAllowed, status)
	})

	t.Run("metrics are served by the net/http handler", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, base+"/metrics", nil)
		require.NoError(t, err)
		status, body := get(t, req)
		require.Equal(t, http.StatusOK, status)
		require.Contains(t, body, "go_goroutines")
	})
}
