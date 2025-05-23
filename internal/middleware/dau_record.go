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
		ch     = make(chan string, 1000)
		logger = zap.L()
	)
	go func() {
		for {
			select {
			case ip := <-ch:
				if ip == "" {
					continue
				}
				ip = strings.Split(ip, ",")[0]
				prefix := time.Now().Format(time.DateOnly)
				_, e := rdb.PFAdd(context.Background(), strings.Join([]string{
					"dau",
					prefix,
				}, ":"), ip).Result()
				if e != nil {
					logger.Warn("Update DAU error", zap.Error(e))
				}
			}
		}
	}()

	return func(c *fiber.Ctx) error {
		ch <- c.IP()
		return c.Next()
	}
}
