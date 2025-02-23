package tasks

import (
	"github.com/MirrorChyan/resource-backend/internal/config"
	"github.com/hibiken/asynq"
	"log"
)

type TaskQueue struct {
	*asynq.Client
}

func NewTaskQueue() *TaskQueue {
	var (
		conf = config.GConfig
	)
	client := asynq.NewClient(asynq.RedisClientOpt{
		Addr: conf.Redis.Addr,
		DB:   conf.Redis.AsynqDB,
	})

	if err := client.Ping(); err != nil {
		log.Fatal(err)
	}
	return &TaskQueue{
		Client: client,
	}
}
