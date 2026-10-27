package handler

import (
	"github.com/MirrorChyan/resource-backend/internal/ent"
	"github.com/MirrorChyan/resource-backend/internal/logic"
	. "github.com/MirrorChyan/resource-backend/internal/logic/misc"
	. "github.com/MirrorChyan/resource-backend/internal/model"
	"github.com/MirrorChyan/resource-backend/internal/model/types"
	"github.com/MirrorChyan/resource-backend/internal/pkg/errs"
	"github.com/MirrorChyan/resource-backend/internal/pkg/restserver/response"
	"github.com/MirrorChyan/resource-backend/internal/pkg/validator"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

// AdminHandler serves the resource/version queries of the admin console and the
// update types the platforms of a resource have of their own.
//
// For Admin: these endpoints are NOT authenticated in-process.
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
	g.Get("/:rid/platforms", h.ListPlatformUpdateTypes)
	g.Put("/:rid/platforms", h.SetPlatformUpdateType)
	g.Delete("/:rid/platforms", h.DeletePlatformUpdateType)
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

	var (
		ctx        = c.UserContext()
		page, size = normalizePage(req.Page, req.PageSize)
	)
	items, total, err := h.resourceLogic.ListResources(ctx, (page-1)*size, size, req.ID, req.Name)
	if err != nil {
		return err
	}

	ids := make([]string, len(items))
	for i, it := range items {
		ids[i] = it.ID
	}
	overrides, err := h.resourceLogic.CountPlatformOverrides(ctx, ids)
	if err != nil {
		return err
	}

	list := make([]ResourceItem, len(items))
	for i, it := range items {
		list[i] = toResourceItem(it, overrides[it.ID])
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
	// keyed by the stored id, which may differ in case from the requested one
	overrides, err := h.resourceLogic.CountPlatformOverrides(ctx, []string{res.ID})
	if err != nil {
		return err
	}

	return c.JSON(response.Success(&ResourceDetailData{
		ResourceItem: toResourceItem(res, overrides[res.ID]),
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

func (h *AdminHandler) ListPlatformUpdateTypes(c *fiber.Ctx) error {
	data, err := h.resourceLogic.ListPlatformUpdateTypes(c.UserContext(), c.Params(ResourceKey))
	if err != nil {
		return err
	}
	return c.JSON(response.Success(data))
}

func (h *AdminHandler) SetPlatformUpdateType(c *fiber.Ctx) error {
	var req SetPlatformUpdateTypeRequest
	if err := validator.ValidateBody(c, &req); err != nil {
		return err
	}
	os, arch := *req.OS, *req.Arch
	if err := bindPlatform(&os, &arch); err != nil {
		return err
	}

	rid := c.Params(ResourceKey)
	change, err := h.resourceLogic.SetPlatformUpdateType(c.UserContext(), rid, os, arch, types.Update(req.UpdateType))
	if err != nil {
		return err
	}
	h.logPlatformUpdateTypeChange(rid, change)
	return c.JSON(response.Success(change))
}

func (h *AdminHandler) DeletePlatformUpdateType(c *fiber.Ctx) error {
	// "" names the platform-independent package, so a missing os or arch must not default to it
	if args := c.Context().QueryArgs(); !args.Has("os") || !args.Has("arch") {
		return errs.ErrInvalidParams
	}
	os, arch := c.Query("os"), c.Query("arch")
	if err := bindPlatform(&os, &arch); err != nil {
		return err
	}

	rid := c.Params(ResourceKey)
	change, err := h.resourceLogic.DeletePlatformUpdateType(c.UserContext(), rid, os, arch)
	if err != nil {
		return err
	}
	h.logPlatformUpdateTypeChange(rid, change)
	return c.JSON(response.Success(change))
}

func (h *AdminHandler) logPlatformUpdateTypeChange(rid string, change *PlatformUpdateTypeChangeData) {
	h.logger.Info("platform update type changed",
		zap.String("resource id", rid),
		zap.String("os", change.OS),
		zap.String("arch", change.Arch),
		zap.String("before", change.Before.Source+":"+change.Before.UpdateType),
		zap.String("after", change.After.Source+":"+change.After.UpdateType),
	)
}

func toResourceItem(r *ent.Resource, platformOverrides int) ResourceItem {
	return ResourceItem{
		ID:                    r.ID,
		Name:                  r.Name,
		Description:           r.Description,
		UpdateType:            r.UpdateType,
		CreatedAt:             r.CreatedAt,
		PlatformOverrideCount: platformOverrides,
	}
}
