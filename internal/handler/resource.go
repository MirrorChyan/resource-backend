package handler

import (
	"github.com/MirrorChyan/resource-backend/internal/handler/response"
	"github.com/MirrorChyan/resource-backend/internal/logic"
	"github.com/MirrorChyan/resource-backend/internal/logic/misc"
	. "github.com/MirrorChyan/resource-backend/internal/model"
	"github.com/MirrorChyan/resource-backend/internal/model/types"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
	"regexp"
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
	req := &CreateResourceRequest{}
	if err := c.BodyParser(req); err != nil {
		h.logger.Error("failed to parse request body",
			zap.Error(err),
		)
		resp := response.BusinessError("invalid param")
		return c.Status(fiber.StatusBadRequest).JSON(resp)
	}

	switch l := len(req.ID); {
	case l == 0:
		resp := response.BusinessError("id is required")
		return c.Status(fiber.StatusBadRequest).JSON(resp)
	case l < 3:
		resp := response.BusinessError("id must be at least 3 characters long")
		return c.Status(fiber.StatusBadRequest).JSON(resp)
	case l > 64:
		resp := response.BusinessError("id must be at most 64 characters long")
		return c.Status(fiber.StatusBadRequest).JSON(resp)
	}

	var validator = misc.ValidID.Get().(*regexp.Regexp)
	defer misc.ValidID.Put(validator)

	if !validator.MatchString(req.ID) {
		resp := response.BusinessError("id must be alphanumeric, underscore, or hyphen")
		return c.Status(fiber.StatusBadRequest).JSON(resp)
	}

	if req.Name == "" {
		resp := response.BusinessError("name is required")
		return c.Status(fiber.StatusBadRequest).JSON(resp)
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
	} else if t != types.UpdateIncremental.String() && t != types.UpdateFull.String() {
		resp := response.BusinessError("update type only be incremental or full")
		return c.Status(fiber.StatusBadRequest).JSON(resp)
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
