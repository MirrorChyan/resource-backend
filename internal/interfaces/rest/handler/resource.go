package handler

import (
	"github.com/MirrorChyan/resource-backend/internal/logic"
	. "github.com/MirrorChyan/resource-backend/internal/model"
	"github.com/MirrorChyan/resource-backend/internal/model/types"
	"github.com/MirrorChyan/resource-backend/internal/pkg/restserver/response"
	"github.com/MirrorChyan/resource-backend/internal/pkg/validator"
	"github.com/gofiber/fiber/v2"
)

type ResourceHandler struct {
	resourceLogic *logic.ResourceLogic
}

func NewResourceHandler(resourceLogic *logic.ResourceLogic) *ResourceHandler {
	return &ResourceHandler{
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

	var t = req.UpdateType
	if t == "" {
		t = types.UpdateIncremental.String()
	}

	res, err := h.resourceLogic.Create(c.UserContext(), CreateResourceParam{
		ID:          req.ID,
		Name:        req.Name,
		Description: req.Description,
		UpdateType:  t,
	})

	if err != nil {
		return err
	}

	resp := response.Success(CreateResourceResponseData{
		ID:          res.ID,
		Name:        res.Name,
		Description: res.Description,
	})
	return c.JSON(resp)
}
