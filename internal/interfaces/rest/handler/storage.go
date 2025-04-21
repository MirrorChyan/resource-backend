package handler

import (
	"github.com/MirrorChyan/resource-backend/internal/logic"
	"github.com/MirrorChyan/resource-backend/internal/pkg/restserver/response"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

type StorageHandler struct {
	logger       *zap.Logger
	storageLogic *logic.StorageLogic
}

func NewStorageHandler(
	logger *zap.Logger,
	storageLogic *logic.StorageLogic,
) *StorageHandler {
	return &StorageHandler{
		logger:       logger,
		storageLogic: storageLogic,
	}
}

func (h *StorageHandler) Register(r fiber.Router) {
	r.Get("/storages/purge", h.Purge)
}

func (h *StorageHandler) Purge(ctx *fiber.Ctx) error {

	err := h.storageLogic.ClearOldStorages(ctx.UserContext())

	if err != nil {
		h.logger.Error("failed to clear old storages",
			zap.Error(err),
		)
		resp := response.UnexpectedError()
		return ctx.Status(fiber.StatusInternalServerError).JSON(resp)
	}

	resp := response.Success(nil)
	return ctx.Status(fiber.StatusOK).JSON(resp)
}
