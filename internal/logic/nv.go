package logic

import (
	"context"
	"strconv"
	"strings"

	"github.com/MirrorChyan/resource-backend/internal/ent"
	"github.com/MirrorChyan/resource-backend/internal/logic/misc"
	. "github.com/MirrorChyan/resource-backend/internal/model"
	"github.com/MirrorChyan/resource-backend/internal/model/types"
	"github.com/bytedance/sonic"
	"github.com/hibiken/asynq"
	"go.uber.org/zap"
)

func (l *VersionLogic) doProcessUpdateRequest(ctx context.Context, param UpdateRequestParam) (*UpdateInfoTuple, error) {
	var (
		cg = l.GetCacheGroup()

		resourceId         = param.ResourceId
		currentVersionName = param.CurrentVersionName
		currentVersionId   int
		targetInfo         = param.TargetVersionInfo
		isFull             = currentVersionName == ""
		full               = &UpdateInfoTuple{
			PackageHash: targetInfo.PackageHash.String,
			PackagePath: targetInfo.PackagePath.String,
			UpdateType:  types.UpdateFull.String(),
		}
	)

	if isFull || targetInfo.ResourceUpdateType == types.UpdateFull {
		return full, nil
	}

	var ck = cg.GetCacheKey(resourceId, currentVersionName)
	vid, err := cg.VersionNameIdCache.ComputeIfAbsent(ck, func() (int, error) {
		v, err := l.versionRepo.GetVersionByName(ctx, resourceId, currentVersionName)
		if err != nil {
			return 0, err
		}
		return v.ID, nil
	})
	switch {
	case err == nil:
		currentVersionId = *vid
	case !ent.IsNotFound(err):
		return nil, err
	default:
		return full, nil
	}

	incremental, err := l.getIncrementalInfoOrEmpty(ctx,
		targetInfo.VersionId,
		currentVersionId,
		targetInfo.OS,
		targetInfo.Arch,
	)
	if err != nil {
		l.logger.Error("failed to get incremental update info",
			zap.String("resource id", resourceId),
			zap.Int("target version id", targetInfo.VersionId),
			zap.Int("current version id", currentVersionId),
			zap.Error(err),
		)
		return nil, err
	}
	if incremental.Storage != nil {
		s := incremental.Storage
		return &UpdateInfoTuple{
			PackageHash: s.PackageHashSha256,
			PackagePath: s.PackagePath,
			UpdateType:  types.UpdateIncremental.String(),
		}, nil
	}

	var (
		targetVersion  = strconv.Itoa(targetInfo.VersionId)
		currentVersion = strconv.Itoa(currentVersionId)
		key            = strings.Join([]string{misc.GenerateTagKey, resourceId, targetVersion, currentVersion}, ":")
	)

	l.logger.Info("incremental fallback to full update",
		zap.String("resourceId", resourceId),
		zap.Int("currentVersionId", currentVersionId),
		zap.Int("targetVersionId", targetInfo.VersionId),
	)

	result := l.rdb.SetNX(ctx, key, 1, 0)
	if err := result.Err(); err != nil {
		return nil, err
	}
	if !result.Val() {
		return full, nil
	}

	rollback := func() {
		l.rdb.Del(ctx, key)
	}

	payload, err := sonic.Marshal(PatchTaskPayload{
		ResourceId:       resourceId,
		CurrentVersionId: currentVersionId,
		TargetVersionId:  targetInfo.VersionId,
		OS:               targetInfo.OS,
		Arch:             targetInfo.Arch,
	})

	if err != nil {
		rollback()
		return nil, err
	}

	task := asynq.NewTask(misc.DiffTask, payload, asynq.MaxRetry(5))
	submitted, err := l.taskQueue.Enqueue(task)
	if err != nil {
		rollback()
		return nil, err
	}
	l.logger.Info("submit generate incremental update package task success",
		zap.String("resource id", resourceId),
		zap.String("target version", targetVersion),
		zap.String("current version", currentVersion),
		zap.String("task id", submitted.ID),
	)

	return full, nil
}

func (l *VersionLogic) GetUpdateInfo(ctx context.Context, param UpdateRequestParam) (*UpdateInfo, error) {
	result, err := l.doProcessUpdateRequest(ctx, param)
	if err != nil {
		return nil, err
	}
	return &UpdateInfo{
		RelPath:    l.cleanTwiceStoragePath(result.PackagePath),
		SHA256:     result.PackageHash,
		UpdateType: result.UpdateType,
	}, nil
}
