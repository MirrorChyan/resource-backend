package handler

import (
	"github.com/MirrorChyan/resource-backend/internal/handler/response"
	"github.com/MirrorChyan/resource-backend/internal/logic"
	. "github.com/MirrorChyan/resource-backend/internal/model"
	"github.com/MirrorChyan/resource-backend/internal/model/types"
	"github.com/MirrorChyan/resource-backend/internal/pkg/validator"
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
	// For Developer
	r.Post("/resources", h.Create)
}

func (h *ResourceHandler) Create(c *fiber.Ctx) error {

	var req CreateResourceRequest
	if err := validator.ValidateBody(c, &req); err != nil {
		return err
	}

	ctx := c.UserContext()

	exists, err := h.resourceLogic.Exists(ctx, req.ID)
	if err != nil {
		h.logger.Error("failed to check resource exists",
			zap.Error(err),
		)
		resp := response.UnexpectedError()
		return c.Status(fiber.StatusInternalServerError).JSON(resp)
	}
	if exists {
		resp := response.BusinessError("resource id already exists")
		return c.Status(fiber.StatusBadRequest).JSON(resp)
	}

	var t = req.UpdateType
	if t == "" {
		t = types.UpdateIncremental.String()
	}

	param := CreateResourceParam{
		ID:          req.ID,
		Name:        req.Name,
		Description: req.Description,
		UpdateType:  t,
	}

	res, err := h.resourceLogic.Create(ctx, param)
	if err != nil {
		h.logger.Error("failed to create resource",
			zap.Error(err),
		)
		resp := response.UnexpectedError(err.Error())
		return c.Status(fiber.StatusInternalServerError).JSON(resp)
	}

	resp := response.Success(CreateResourceResponseData{
		ID:          res.ID,
		Name:        res.Name,
		Description: res.Description,
	})
	return c.Status(fiber.StatusOK).JSON(resp)
}
