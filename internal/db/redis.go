package db

import (
	"context"

	"github.com/MirrorChyan/resource-backend/internal/config"
	"github.com/pkg/errors"
	"github.com/redis/go-redis/v9"
)

func NewRedis(conf *config.Config) *redis.Client {
	rdb := redis.NewClient(&redis.Options{
		Addr:     conf.Redis.Addr,
		DB:       conf.Redis.DB,
		Username: conf.Redis.Username,
		Password: conf.Redis.Password,
	})
	_, err := rdb.Ping(context.Background()).Result()
	if err != nil {
		panic(errors.WithMessage(err, "failed to ping redis"))
	}

	return rdb
}
