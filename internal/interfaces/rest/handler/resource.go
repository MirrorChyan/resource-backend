package handler

import (
	"github.com/MirrorChyan/resource-backend/internal/logic"
	. "github.com/MirrorChyan/resource-backend/internal/model"
	"github.com/MirrorChyan/resource-backend/internal/model/types"
	"github.com/MirrorChyan/resource-backend/internal/pkg/restserver/response"
	"github.com/MirrorChyan/resource-backend/internal/pkg/sortorder"
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

	// for admin
	admin := r.Group("/admin")
	admin.Get("/resources", h.List)
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

func (h *ResourceHandler) List(c *fiber.Ctx) error {

	var req ListResourceRequest
	if err := validator.ValidateQuery(c, &req); err != nil {
		return err
	}

	order := sortorder.Parse(req.Sort)

	if req.Limit == 0 {
		req.Limit = 20
	}

	result, err := h.resourceLogic.List(c.UserContext(), &ListResourceParam{
		Offset: req.Offset,
		Limit:  req.Limit,
		Order:  order,
	})
	if err != nil {
		return err
	}

	list := make([]*ResourceResponseItem, 0, len(result.List))
	for _, item := range result.List {
		list = append(list, &ResourceResponseItem{
			ID:          item.ID,
			Name:        item.Name,
			Description: item.Description,
			CreatedAt:   item.CreatedAt,
		})
	}

	resp := response.Success(ListResourceResponseData{
		List:    list,
		Offset:  req.Offset,
		Limit:   req.Limit,
		Total:   result.Total,
		HasMore: result.HasMore,
	})
	return c.JSON(resp)
}
