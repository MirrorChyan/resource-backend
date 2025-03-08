package handler

import (
	"github.com/MirrorChyan/resource-backend/internal/handler/response"
	"github.com/MirrorChyan/resource-backend/internal/logic"
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
	r.Get("/storages/purge", func(ctx *fiber.Ctx) error {
		err := h.storageLogic.ClearOldStorages(ctx.UserContext())
		if err != nil {
			return err
		}
		return ctx.Status(fiber.StatusOK).JSON(response.Success(nil))
	})
}
