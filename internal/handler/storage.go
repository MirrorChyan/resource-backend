package handler

import (
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
	r.Post("/resources/:rid/versions/latest/clear", func(ctx *fiber.Ctx) error {
		return ctx.SendString("TODO")
	})
}
