package handler

import (
	"github.com/MirrorChyan/resource-backend/internal/ent"
	"github.com/MirrorChyan/resource-backend/internal/logic"
	. "github.com/MirrorChyan/resource-backend/internal/logic/misc"
	. "github.com/MirrorChyan/resource-backend/internal/model"
	"github.com/MirrorChyan/resource-backend/internal/pkg/errs"
	"github.com/MirrorChyan/resource-backend/internal/pkg/restserver/response"
	"github.com/MirrorChyan/resource-backend/internal/pkg/validator"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

// AdminHandler serves read-only resource/version queries for the admin console.
//
// For Admin (gateway-restricted): these endpoints are NOT authenticated in-process,
// consistent with the existing "For Developer" endpoints. Access control is expected
// at the APISIX gateway, which can match the shared `/admin` prefix to enforce
// internal-network / token admission.
type AdminHandler struct {
	logger        *zap.Logger
	resourceLogic *logic.ResourceLogic
	versionLogic  *logic.VersionLogic
}

func NewAdminHandler(
	logger *zap.Logger,
	resourceLogic *logic.ResourceLogic,
	versionLogic *logic.VersionLogic,
) *AdminHandler {
	return &AdminHandler{
		logger:        logger,
		resourceLogic: resourceLogic,
		versionLogic:  versionLogic,
	}
}

const (
	defaultPage     = 1
	defaultPageSize = 20
	maxPageSize     = 100
)

func (h *AdminHandler) Register(r fiber.Router) {
	g := r.Group("/admin/resources")
	g.Get("/", h.ListResources)
	g.Get("/:rid", h.GetResource)
	g.Get("/:rid/versions", h.ListVersions)
}

func normalizePage(page, size int) (int, int) {
	if page <= 0 {
		page = defaultPage
	}
	if size <= 0 {
		size = defaultPageSize
	}
	if size > maxPageSize {
		size = maxPageSize
	}
	return page, size
}

func (h *AdminHandler) ListResources(c *fiber.Ctx) error {
	var req ListResourcesRequest
	if err := validator.ValidateQuery(c, &req); err != nil {
		return err
	}

	page, size := normalizePage(req.Page, req.PageSize)
	items, total, err := h.resourceLogic.ListResources(c.UserContext(), (page-1)*size, size, req.ID, req.Name)
	if err != nil {
		return err
	}

	list := make([]ResourceItem, len(items))
	for i, it := range items {
		list[i] = toResourceItem(it)
	}
	return c.JSON(response.Success(&PageData{List: list, Total: total, Page: page, PageSize: size}))
}

func (h *AdminHandler) GetResource(c *fiber.Ctx) error {
	var (
		ctx = c.UserContext()
		rid = c.Params(ResourceKey)
	)

	res, err := h.resourceLogic.GetByID(ctx, rid)
	if err != nil {
		return err
	}
	count, err := h.resourceLogic.CountVersions(ctx, rid)
	if err != nil {
		return err
	}

	return c.JSON(response.Success(&ResourceDetailData{
		ResourceItem: toResourceItem(res),
		VersionCount: count,
	}))
}

func (h *AdminHandler) ListVersions(c *fiber.Ctx) error {
	var req ListVersionsRequest
	if err := validator.ValidateQuery(c, &req); err != nil {
		return err
	}

	var (
		ctx = c.UserContext()
		rid = c.Params(ResourceKey)
	)

	exists, err := h.resourceLogic.Exists(ctx, rid)
	if err != nil {
		return err
	}
	if !exists {
		return errs.ErrResourceNotFound
	}

	channel := req.Channel
	if channel != "" {
		ch, ok := ChannelMap[channel]
		if !ok {
			return errs.ErrResourceInvalidChannel
		}
		channel = ch
	}

	page, size := normalizePage(req.Page, req.PageSize)
	items, total, err := h.versionLogic.ListByResource(ctx, rid, (page-1)*size, size, channel)
	if err != nil {
		return err
	}

	list := make([]VersionItem, len(items))
	for i, it := range items {
		list[i] = VersionItem{
			ID:        it.ID,
			Channel:   string(it.Channel),
			Name:      it.Name,
			Number:    it.Number,
			CreatedAt: it.CreatedAt,
		}
	}
	return c.JSON(response.Success(&PageData{List: list, Total: total, Page: page, PageSize: size}))
}

func toResourceItem(r *ent.Resource) ResourceItem {
	return ResourceItem{
		ID:          r.ID,
		Name:        r.Name,
		Description: r.Description,
		UpdateType:  r.UpdateType,
		CreatedAt:   r.CreatedAt,
	}
}
