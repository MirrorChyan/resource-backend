package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/jmoiron/sqlx"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	_ "modernc.org/sqlite"

	"github.com/MirrorChyan/resource-backend/internal/cache"
	"github.com/MirrorChyan/resource-backend/internal/ent"
	"github.com/MirrorChyan/resource-backend/internal/logic"
	"github.com/MirrorChyan/resource-backend/internal/model"
	"github.com/MirrorChyan/resource-backend/internal/model/types"
	"github.com/MirrorChyan/resource-backend/internal/pkg/errs"
	"github.com/MirrorChyan/resource-backend/internal/repo"
)

func newAdminTestApp(t *testing.T) *fiber.App {
	t.Helper()
	db, err := sql.Open("sqlite", "file::memory:?_pragma=foreign_keys(1)")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.SQLite, db)))
	require.NoError(t, client.Schema.Create(context.Background()))
	client.Resource.Create().SetID("res").SetName("res").SetDescription("").
		SetUpdateType(types.UpdateIncremental.String()).SaveX(context.Background())

	rdb := redis.NewClient(&redis.Options{Addr: miniredis.RunT(t).Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	r := repo.NewRepo(client, sqlx.NewDb(db, "sqlite"))
	resourceLogic := logic.NewResourceLogic(zap.NewNop(),
		repo.NewResource(r), repo.NewPlatformConfig(r), repo.NewStorage(r),
		rdb, &cache.MultiCacheGroup{
			ResourceInfoCache:       cache.NewCache[string, *ent.Resource](-1),
			PlatformUpdateTypeCache: cache.NewCache[string, map[model.Platform]types.Update](-1),
		},
	)

	app := fiber.New(fiber.Config{ErrorHandler: Error})
	NewAdminHandler(zap.NewNop(), resourceLogic, nil).Register(app)
	return app
}

type adminTestResponse[T any] struct {
	Code int `json:"code"`
	Data T   `json:"data"`
}

func doAdminRequest[T any](t *testing.T, app *fiber.App, method, target, body string) (int, adminTestResponse[T]) {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, target, reader)
	if body != "" {
		req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	}
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	var out adminTestResponse[T]
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	return resp.StatusCode, out
}

func TestAdminPlatformUpdateTypes(t *testing.T) {
	app := newAdminTestApp(t)

	// os and arch are normalized the way the version endpoints take them
	status, set := doAdminRequest[model.PlatformUpdateTypeChangeData](t, app, http.MethodPut,
		"/admin/resources/res/platforms", `{"os":"Windows","arch":"x64","update_type":"full"}`)
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, "windows", set.Data.OS)
	require.Equal(t, "amd64", set.Data.Arch)
	require.Equal(t, model.PlatformUpdateTypeState{UpdateType: "incremental", Source: model.PlatformSourceInherit}, set.Data.Before)
	require.Equal(t, model.PlatformUpdateTypeState{UpdateType: "full", Source: model.PlatformSourceOverride}, set.Data.After)

	status, list := doAdminRequest[model.PlatformUpdateTypesData](t, app, http.MethodGet,
		"/admin/resources/res/platforms", "")
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, "incremental", list.Data.DefaultUpdateType)
	require.Len(t, list.Data.Platforms, 1)
	require.Equal(t, "full", list.Data.Platforms[0].UpdateType)
	require.False(t, list.Data.Platforms[0].HasStorage)

	status, page := doAdminRequest[struct {
		List []model.ResourceItem `json:"list"`
	}](t, app, http.MethodGet, "/admin/resources", "")
	require.Equal(t, http.StatusOK, status)
	require.Len(t, page.Data.List, 1)
	require.Equal(t, 1, page.Data.List[0].PlatformOverrideCount)

	status, del := doAdminRequest[model.PlatformUpdateTypeChangeData](t, app, http.MethodDelete,
		"/admin/resources/res/platforms?os=windows&arch=x86_64", "")
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, model.PlatformUpdateTypeState{UpdateType: "full", Source: model.PlatformSourceOverride}, del.Data.Before)
	require.Equal(t, model.PlatformUpdateTypeState{UpdateType: "incremental", Source: model.PlatformSourceInherit}, del.Data.After)

	status, list = doAdminRequest[model.PlatformUpdateTypesData](t, app, http.MethodGet,
		"/admin/resources/res/platforms", "")
	require.Equal(t, http.StatusOK, status)
	require.Empty(t, list.Data.Platforms)
}

func TestAdminPlatformUpdateTypeRejectsBadInput(t *testing.T) {
	app := newAdminTestApp(t)

	for _, c := range []struct {
		name, method, target, body string
		status, code               int
	}{
		{"bad update type", http.MethodPut, "/admin/resources/res/platforms",
			`{"os":"windows","arch":"amd64","update_type":"patch"}`, http.StatusBadRequest, errs.BizCodeInvalidParams},
		{"missing update type", http.MethodPut, "/admin/resources/res/platforms",
			`{"os":"windows","arch":"amd64"}`, http.StatusBadRequest, errs.BizCodeInvalidParams},
		{"bad os", http.MethodPut, "/admin/resources/res/platforms",
			`{"os":"beos","arch":"amd64","update_type":"full"}`, http.StatusBadRequest, errs.BizCodeResourceInvalidOS},
		{"bad arch", http.MethodDelete, "/admin/resources/res/platforms?os=windows&arch=mips", "",
			http.StatusBadRequest, errs.BizCodeResourceInvalidArch},
		// a missing os must not name the platform-independent package
		{"missing os", http.MethodDelete, "/admin/resources/res/platforms?arch=arm64", "",
			http.StatusBadRequest, errs.BizCodeInvalidParams},
		{"missing os in body", http.MethodPut, "/admin/resources/res/platforms",
			`{"arch":"arm64","update_type":"full"}`, http.StatusBadRequest, errs.BizCodeInvalidParams},
		{"unknown resource", http.MethodPut, "/admin/resources/missing/platforms",
			`{"os":"windows","arch":"amd64","update_type":"full"}`, http.StatusNotFound, errs.BizCodeResourceNotFound},
		{"unknown resource list", http.MethodGet, "/admin/resources/missing/platforms", "",
			http.StatusNotFound, errs.BizCodeResourceNotFound},
	} {
		t.Run(c.name, func(t *testing.T) {
			status, resp := doAdminRequest[any](t, app, c.method, c.target, c.body)
			require.Equal(t, c.status, status)
			require.Equal(t, c.code, resp.Code)
		})
	}
}
