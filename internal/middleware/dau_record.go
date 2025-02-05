package middleware

import (
	"context"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

func NewDailyActiveUserRecorder(rdb *redis.Client) fiber.Handler {
	var (
		ch     = make(chan string, 1200)
		logger = zap.L()
	)
	go func() {
		var (
			buf    = make([]string, 0, 1000)
			ticker = time.NewTicker(time.Second * 7)
		)
		defer ticker.Stop()
		for {
			select {
			case ip := <-ch:
				buf = append(buf, ip)
			case <-ticker.C:
				if len(buf) > 0 {
					prefix := time.Now().Format(time.DateOnly)
					_, e := rdb.PFAdd(context.Background(), strings.Join([]string{
						"DAU",
						prefix,
					}, ":"), buf).Result()
					if e != nil {
						logger.Warn("Update DAU error", zap.Error(e))
					}
					buf = buf[:0]
				}
			}
		}
	}()

	return func(c *fiber.Ctx) error {
		ch <- c.IP()
		return c.Next()
	}
}
