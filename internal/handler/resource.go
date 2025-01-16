package handler

import (
	"regexp"
	"sync"

	"github.com/MirrorChyan/resource-backend/internal/handler/response"
	"github.com/MirrorChyan/resource-backend/internal/logic"
	. "github.com/MirrorChyan/resource-backend/internal/model"
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

var validID = sync.Pool{
	New: func() interface{} {
		return regexp.MustCompile("^[a-zA-Z0-9_-]+$")
	},
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

	switch idLength := len(req.ID); {
	case idLength == 0:
		resp := response.BusinessError("id is required")
		return c.Status(fiber.StatusBadRequest).JSON(resp)
	case idLength < 3:
		resp := response.BusinessError("id must be at least 3 characters long")
		return c.Status(fiber.StatusBadRequest).JSON(resp)
	case idLength > 64:
		resp := response.BusinessError("id must be at most 64 characters long")
		return c.Status(fiber.StatusBadRequest).JSON(resp)
	}

	var validator = validID.Get().(*regexp.Regexp)
	defer validID.Put(validator)

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

	param := CreateResourceParam{
		ID:          req.ID,
		Name:        req.Name,
		Description: req.Description,
	}

	res, err := h.resourceLogic.Create(ctx, param)
	if err != nil {
		h.logger.Error("failed to create resource",
			zap.Error(err),
		)
		resp := response.UnexpectedError()
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
