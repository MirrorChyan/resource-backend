package repo

import (
	"context"
	"encoding/json"
	"time"

	"github.com/MirrorChyan/resource-backend/internal/model"
	"github.com/redis/go-redis/v9"
)

type TempDownloadInfo struct {
	rdb *redis.Client
}

func NewTempDownloadInfo(rdb *redis.Client) *TempDownloadInfo {
	return &TempDownloadInfo{
		rdb: rdb,
	}
}

func (r *TempDownloadInfo) GetDelTempDownloadInfo(ctx context.Context, key string) (*model.TempDownloadInfo, error) {
	val, err := r.rdb.Get(ctx, key).Result()
	if err != nil {
		return nil, err
	}

	info := &model.TempDownloadInfo{}
	err = json.Unmarshal([]byte(val), info)
	if err != nil {
		return nil, err
	}

	return info, nil
}

func (r *TempDownloadInfo) SetTempDownloadInfo(ctx context.Context, key string, info *model.TempDownloadInfo, expiration time.Duration) error {
	buf, err := json.Marshal(info)
	if err != nil {
		return err
	}

	return r.rdb.Set(ctx, key, buf, expiration).Err()
}
