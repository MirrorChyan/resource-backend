package handler

import (
	"context"

	"github.com/MirrorChyan/resource-backend/internal/logic"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

type ResourceHandler struct {
	logger        *zap.Logger
	resourceLogic *logic.ResourceLogic
}

func NewResourceHandler(logger *zap.Logger, resourceLogic *logic.ResourceLogic) *ResourceHandler {
	return &ResourceHandler{
		logger:        logger,
		resourceLogic: resourceLogic,
	}
}

func (h *ResourceHandler) Register(r fiber.Router) {
	r.Post("/resources", h.Create)
}

type CreateResourceRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (h *ResourceHandler) Create(c *fiber.Ctx) error {
	req := &CreateResourceRequest{}
	if err := c.BodyParser(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	param := logic.CreateResourceParam{
		Name:        req.Name,
		Description: req.Description,
	}

	ctx := context.Background()
	res, err := h.resourceLogic.Create(ctx, param)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(res)
}
