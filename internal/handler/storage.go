package handler

import (
	"context"

	"github.com/MirrorChyan/resource-backend/internal/ent"
	"github.com/MirrorChyan/resource-backend/internal/ent/version"
	"github.com/MirrorChyan/resource-backend/internal/handler/response"
	"github.com/MirrorChyan/resource-backend/internal/logic"
	"github.com/MirrorChyan/resource-backend/internal/logic/misc"
	"github.com/MirrorChyan/resource-backend/internal/model"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

type StorageHandler struct {
	logger        *zap.Logger
	resourceLogic *logic.ResourceLogic
	versionLogic  *logic.VersionLogic
	storageLogic  *logic.StorageLogic
}

func NewStorageHandler(
	logger *zap.Logger,
	resourceLogic *logic.ResourceLogic,
	versionLogic *logic.VersionLogic,
	storageLogic *logic.StorageLogic,
) *StorageHandler {
	return &StorageHandler{
		logger:        logger,
		resourceLogic: resourceLogic,
		versionLogic:  versionLogic,
		storageLogic:  storageLogic,
	}
}

func (h *StorageHandler) Register(r fiber.Router) {
	r.Post("/resources/:rid/versions/latest/clear", h.Clear)
}

func (h *StorageHandler) Clear(c *fiber.Ctx) error {
	resID := c.Params(misc.ResourceKey)

	ctx := c.UserContext()

	exist, err := h.resourceLogic.Exists(ctx, resID)
	if err != nil {
		resp := response.UnexpectedError()
		return c.Status(fiber.StatusInternalServerError).JSON(resp)
	}
	if !exist {
		resp := response.BusinessError("resource not found")
		return c.Status(fiber.StatusBadRequest).JSON(resp)
	}

	req := &model.ClearOldStorageRequest{}
	if err := c.QueryParser(req); err != nil {
		resp := response.BusinessError("invalid param")
		return c.Status(fiber.StatusBadRequest).JSON(resp)
	}

	channel, ok := misc.ChannelMap[req.Channel]
	if !ok {
		resp := response.BusinessError("invalid channel")
		return c.Status(fiber.StatusBadRequest).JSON(resp)
	}

	var getLatestFunc func(context.Context, string) (*ent.Version, error)

	switch channel {
	case "alpha":
		getLatestFunc = h.versionLogic.GetLatestAlphaVersion
	case "beta":
		getLatestFunc = h.versionLogic.GetLatestBetaVersion
	default:
		getLatestFunc = h.versionLogic.GetLatestStableVersion
	}

	latest, err := getLatestFunc(ctx, resID)
	if err != nil {
		resp := response.UnexpectedError()
		return c.Status(fiber.StatusInternalServerError).JSON(resp)
	}

	go func() {
		h.logger.Info("start clear old storages",
			zap.String("resource id", resID),
			zap.String("channel", channel),
		)
		err := h.storageLogic.ClearOldStorages(context.Background(), resID, version.Channel(channel), latest.ID)
		if err != nil {
			h.logger.Error("clear old storages failed",
				zap.Error(err),
			)
			return
		}
		h.logger.Info("complete clear old storages",
			zap.String("resource id", resID),
			zap.String("channel", channel),
		)
	}()

	resp := response.Success(nil)
	return c.Status(fiber.StatusOK).JSON(resp)
}
