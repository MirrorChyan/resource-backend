package handler

import (
	"context"

	"github.com/MirrorChyan/resource-backend/internal/handler/response"
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

type CreateResourceResponseData struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (h *ResourceHandler) Create(c *fiber.Ctx) error {
	req := &CreateResourceRequest{}
	if err := c.BodyParser(req); err != nil {
		h.logger.Error("failed to parse request body",
			zap.Error(err),
		)
		resp := response.BusinessError("failed to parse request body")
		return c.Status(fiber.StatusBadRequest).JSON(resp)
	}
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
		resp := response.UnexpectedError("internal server error")
		return c.Status(fiber.StatusInternalServerError).JSON(resp)
	}

	data := CreateResourceResponseData{
		ID:          res.ID,
		Name:        res.Name,
		Description: res.Description,
	}
	resp := response.Success(data)
	return c.Status(fiber.StatusOK).JSON(resp)
}
