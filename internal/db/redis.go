package db

import (
	"context"
	"github.com/MirrorChyan/resource-backend/internal/config"
	"github.com/go-redsync/redsync/v4"
	"github.com/go-redsync/redsync/v4/redis/goredis/v9"
	"github.com/pkg/errors"
	"github.com/redis/go-redis/v9"
)

func NewRedis() *redis.Client {
	var (
		conf = config.GConfig
	)
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

func NewRedSync(rdb *redis.Client) *redsync.Redsync {
	pool := goredis.NewPool(rdb)
	return redsync.New(pool)
}
