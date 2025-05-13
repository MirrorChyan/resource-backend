package handler

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/MirrorChyan/resource-backend/internal/config"
	. "github.com/MirrorChyan/resource-backend/internal/logic/misc"
	"github.com/MirrorChyan/resource-backend/internal/middleware"
	"github.com/MirrorChyan/resource-backend/internal/model/types"
	"github.com/MirrorChyan/resource-backend/internal/pkg/errs"
	"github.com/MirrorChyan/resource-backend/internal/pkg/validator"
	"github.com/MirrorChyan/resource-backend/internal/pkg/vercomp"
	"github.com/bytedance/sonic"
	"github.com/redis/go-redis/v9"
	"github.com/valyala/fasthttp"

	"github.com/MirrorChyan/resource-backend/internal/logic"
	. "github.com/MirrorChyan/resource-backend/internal/model"
	"github.com/MirrorChyan/resource-backend/internal/pkg/restserver/response"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

type VersionHandler struct {
	logger        *zap.Logger
	resourceLogic *logic.ResourceLogic
	versionLogic  *logic.VersionLogic
	verComparator *vercomp.VersionComparator
	collect       func(string, string, string)
}

func NewVersionHandler(
	logger *zap.Logger,
	resourceLogic *logic.ResourceLogic,
	versionLogic *logic.VersionLogic,
	verComparator *vercomp.VersionComparator,
) *VersionHandler {
	handler := &VersionHandler{
		logger:        logger,
		resourceLogic: resourceLogic,
		versionLogic:  versionLogic,
		verComparator: verComparator,
	}

	handler.collect = handler.getCollector()
	return handler
}

func (h *VersionHandler) getCollector() func(rid string, version string, ip string) {
	rdb := h.versionLogic.GetRedisClient()
	type p struct {
		rid, version, ip string
	}

	var (
		ch     = make(chan p, 100)
		logger = zap.L()
	)

	go func() {
		var (
			ctx    = context.Background()
			buf    = make([]p, 0, 1000)
			ticker = time.NewTicker(time.Second * 12)
		)
		defer ticker.Stop()
		for {
			select {
			case part := <-ch:
				buf = append(buf, part)
			case <-ticker.C:
				if len(buf) > 0 {
					date := time.Now().Format(time.DateOnly)
					pipeliner := rdb.Pipeline()
					for _, val := range buf {
						key := strings.Join([]string{
							VersionPrefix,
							date,
							val.rid,
						}, ":")
						pipeliner.SAdd(ctx, key, val.version)
						pipeliner.PFAdd(ctx, strings.Join([]string{
							key,
							val.version,
						}, ":"), val.ip)
					}
					if _, e := pipeliner.Exec(ctx); e != nil {
						logger.Warn("update version stat error", zap.Error(e))
					}
					buf = buf[:0]
				}
			}
		}
	}()

	return func(rid string, version string, ip string) {
		arr := strings.Split(ip, ",")
		if len(arr) >= 2 {
			ip = arr[0]
		}
		ch <- p{rid, version, ip}
	}

}

func (h *VersionHandler) Register(r fiber.Router) {

	// for daily active user
	dau := middleware.NewDailyActiveUserRecorder(h.versionLogic.GetRedisClient())

	r.Get("/resources/:rid/latest", dau, h.GetLatest)
	r.Get("/resources/download/:key", h.RedirectToDownload)

	// For Developer
	versions := r.Group("/resources/:rid/versions")
	versions.Use("/", middleware.NewValidateUploader())
	versions.Post("/", h.Create)
	versions.Post("/callback", h.CreateVersionCallBack)

	versions.Put("/release-note", h.UpdateReleaseNote)
	versions.Put("/custom-data", h.UpdateCustomData)
}

func (h *VersionHandler) bindRequiredParams(os, arch, channel *string) error {
	if o, ok := OsMap[*os]; !ok {
		return errs.ErrResourceInvalidOS
	} else {
		*os = o
	}

	if a, ok := ArchMap[*arch]; !ok {
		return errs.ErrResourceInvalidArch
	} else {
		*arch = a
	}

	if c, ok := ChannelMap[*channel]; !ok {
		return errs.ErrResourceInvalidChannel
	} else {
		*channel = c
	}
	return nil
}

func (h *VersionHandler) Create(c *fiber.Ctx) error {

	resourceId := c.Params(ResourceKey)

	var req CreateVersionRequest
	if err := validator.ValidateBody(c, &req); err != nil {
		return err
	}

	if err := h.bindRequiredParams(&req.OS, &req.Arch, &req.Channel); err != nil {
		return err
	}

	if req.Channel != types.ChannelStable.String() {
		parsable := h.verComparator.IsVersionParsable(req.Name)
		if !parsable {
			return errs.ErrResourceVersionNameUnparsable
		}
	}

	token, err := h.versionLogic.CreatePreSignedUrl(c.UserContext(), CreateVersionParam{
		ResourceID: resourceId,
		Name:       req.Name,
		OS:         req.OS,
		Arch:       req.Arch,
		Channel:    req.Channel,
		Filename:   req.Filename,
	})
	if err != nil {
		return err
	}

	return c.JSON(response.Success(token))
}

func (h *VersionHandler) CreateVersionCallBack(c *fiber.Ctx) error {

	resourceId := c.Params(ResourceKey)

	var req CreateVersionCallBackRequest
	if err := validator.ValidateBody(c, &req); err != nil {
		return err
	}

	if err := h.bindRequiredParams(&req.OS, &req.Arch, &req.Channel); err != nil {
		return err
	}

	err := h.versionLogic.ProcessCreateVersionCallback(c.UserContext(), CreateVersionCallBackParam{
		ResourceID: resourceId,
		Name:       req.Name,
		OS:         req.OS,
		Arch:       req.Arch,
		Channel:    req.Channel,
		Key:        req.Key,
	})
	if err != nil {
		return err
	}

	return c.JSON(response.Success(nil))
}

func (h *VersionHandler) doValidateCDK(info *GetLatestVersionRequest, resourceId, ip string) (int64, error) {

	h.logger.Info("Validating CDK")

	body, err := sonic.Marshal(ValidateCDKRequest{
		CDK:      info.CDK,
		Resource: resourceId,
		UA:       info.UserAgent,
		IP:       ip,
	})

	if err != nil {
		h.logger.Error("Failed to marshal JSON",
			zap.Error(err),
		)
		return 0, err
	}

	var (
		conf    = config.GConfig
		agent   = fiber.AcquireAgent()
		request = agent.Request()
		resp    = fasthttp.AcquireResponse()
		result  ValidateResponse
	)

	defer func() {
		fiber.ReleaseAgent(agent)
		fasthttp.ReleaseResponse(resp)
	}()

	request.SetRequestURI(conf.Auth.CDKValidationURL)
	request.Header.SetMethod(fiber.MethodPost)
	request.Header.SetContentType(fiber.MIMEApplicationJSON)
	request.SetBody(body)

	if err := agent.Parse(); err != nil {
		h.logger.Error("Failed to parse request",
			zap.Error(err),
		)
		return 0, err
	}

	if err := agent.Do(request, resp); err != nil {
		h.logger.Error("Failed to send request",
			zap.Error(err),
		)
		return 0, err
	}

	if err := sonic.Unmarshal(resp.Body(), &result); err != nil {
		h.logger.Error("Failed to decode response",
			zap.Error(err),
		)
		return 0, err
	}

	var (
		code = result.Code
		msg  = result.Msg
	)
	switch {
	case code > 0:
		h.logger.Info("cdk validation failed",
			zap.Int("code", code),
			zap.String("msg", msg),
		)
		return 0, errs.New(code, fiber.StatusForbidden, msg, nil)
	case code < 0:
		h.logger.Error("CDK validation failed",
			zap.Int("code", code),
			zap.String("msg", msg),
		)
		return 0, errors.New("unknown error")
	}

	h.logger.Info("CDK validation success")

	return result.Data, nil
}

func (h *VersionHandler) doHandleGetLatestParam(c *fiber.Ctx) (*GetLatestVersionRequest, error) {

	var req GetLatestVersionRequest
	if err := validator.ValidateQuery(c, &req); err != nil {
		return nil, err
	}

	req.ResourceID = c.Params(ResourceKey)

	err := h.bindRequiredParams(&req.OS, &req.Arch, &req.Channel)
	if err != nil {
		return nil, err
	}

	return &req, nil
}

func (h *VersionHandler) GetLatest(c *fiber.Ctx) error {

	param, err := h.doHandleGetLatestParam(c)
	if err != nil {
		return err
	}

	var (
		ctx            = c.UserContext()
		ip             = c.IP()
		resourceId     = param.ResourceID
		system         = param.OS
		arch           = param.Arch
		channel        = param.Channel
		currentVersion = param.CurrentVersion
		cdk            = param.CDK
	)

	latest, err := h.versionLogic.GetMultiLatestVersionInfo(resourceId, system, arch, channel)
	if err != nil {
		return err
	}

	var resp = &QueryLatestResponseData{
		VersionName:   latest.VersionName,
		VersionNumber: latest.VersionNumber,
		ReleaseNote:   latest.ReleaseNote,
		Channel:       channel,
		OS:            param.OS,
		Arch:          param.Arch,
	}

	h.collect(resourceId, currentVersion, ip)

	if cdk == "" {
		if latest.VersionName == currentVersion {
			resp.ReleaseNote = "placeholder"
		}
		resp := response.Success(resp, "current resource latest version is "+latest.VersionName)
		return c.JSON(resp)
	}

	release, limited := h.doLimitByConfig(resourceId)
	defer release()
	if limited {
		resp.VersionName = param.CurrentVersion
		resp := response.Success(resp, "current resource latest version is "+latest.VersionName)
		return c.JSON(resp)
	}

	ts, err := h.doValidateCDK(param, resourceId, ip)
	if err != nil {
		return err
	}

	if latest.VersionName == currentVersion {
		resp.ReleaseNote = "placeholder"
		resp := response.Success(resp, "current version is latest")
		return c.JSON(resp)
	}

	result, err := h.versionLogic.GetUpdateInfo(ctx, UpdateRequestParam{
		ResourceId:         resourceId,
		CurrentVersionName: currentVersion,
		TargetVersionInfo:  latest,
	})
	if err != nil {
		return err
	}

	url, err := h.versionLogic.GetDistributeURL(&DistributeInfo{
		CDK:      cdk,
		UA:       param.UserAgent,
		IP:       ip,
		Resource: resourceId,
		Version:  latest.VersionName,
		Filesize: result.Filesize,
		RelPath:  result.RelPath,
	})
	if err != nil {
		return err
	}

	resp.SHA256 = result.SHA256
	resp.Filesize = result.Filesize
	resp.UpdateType = result.UpdateType
	resp.CustomData = latest.CustomData
	resp.ExpiredTime = ts
	resp.Url = url

	return c.JSON(response.Success(resp))
}

func (h *VersionHandler) doLimitByConfig(resourceId string) (func(), bool) {
	var (
		counter = CompareIfAbsent(LIT, resourceId)
		con     = config.GConfig.Extra.Concurrency
		rf      = func() {
			counter.Add(-1)
		}
		cv = counter.Add(1)
	)

	if con != 0 {
		if cv > con {
			h.logger.Warn("limit by", zap.Int32("concurrency", cv))
			return rf, true
		}
	}

	return rf, false
}

func (h *VersionHandler) RedirectToDownload(c *fiber.Ctx) error {
	var (
		rk  = c.Params("key")
		ctx = c.UserContext()
	)

	url, err := h.versionLogic.GetDistributeLocation(ctx, rk)
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return c.Status(fiber.StatusNotFound).JSON(response.BusinessError("resource not found"))
		}
		if errors.Is(err, ResourceLimitError) {
			return c.Status(fiber.StatusForbidden).SendString(err.Error())
		}
		return err
	}
	h.logger.Info("RedirectToDownload",
		zap.String("distribute key", rk),
		zap.String("download url", url),
	)
	return c.Redirect(url)
}

func (h *VersionHandler) UpdateReleaseNote(c *fiber.Ctx) error {

	var (
		ctx        = c.UserContext()
		resourceId = c.Params(ResourceKey)
	)

	resExist, err := h.resourceLogic.Exists(ctx, resourceId)
	switch {
	case err != nil:
		h.logger.Error("Failed to check if resource exists",
			zap.Error(err),
		)
		resp := response.UnexpectedError()
		return c.Status(fiber.StatusInternalServerError).JSON(resp)

	case !resExist:
		h.logger.Info("Resource not found",
			zap.String("resource id", resourceId),
		)
		resp := response.BusinessError("resource not found")
		return c.Status(fiber.StatusNotFound).JSON(resp)

	}

	req := &UpdateReleaseNoteRequest{}
	if err := c.BodyParser(req); err != nil {
		h.logger.Error("failed to parse request body",
			zap.Error(err),
		)
		resp := response.BusinessError("invalid param")
		return c.Status(fiber.StatusBadRequest).JSON(resp)
	}

	if len(req.Content) > 30000 {
		req.Content = req.Content[:30000]
	}

	if ch, ok := ChannelMap[req.Channel]; ok {
		req.Channel = ch
	} else {
		return errors.New("invalid channel")
	}

	ver, err := h.versionLogic.LoadStoreNewVersionTx(ctx, resourceId, req.VersionName, req.Channel)
	if err != nil {
		h.logger.Error("failed to load store version",
			zap.String("resource id", resourceId),
			zap.String("version name", req.VersionName),
			zap.Error(err),
		)
		resp := response.UnexpectedError()
		return c.Status(fiber.StatusInternalServerError).JSON(resp)
	}
	err = h.versionLogic.UpdateReleaseNote(ctx, UpdateReleaseNoteDetailParam{
		VersionID:         ver.ID,
		ReleaseNoteDetail: req.Content,
	})
	if err != nil {
		h.logger.Error("failed to update version release note",
			zap.String("resource id", resourceId),
			zap.String("version name", req.VersionName),
			zap.Error(err),
		)
		resp := response.UnexpectedError()
		return c.Status(fiber.StatusInternalServerError).JSON(resp)
	}

	h.doEvictCache(resourceId)

	resp := response.Success(nil)
	return c.Status(fiber.StatusOK).JSON(resp)
}

func (h *VersionHandler) UpdateCustomData(c *fiber.Ctx) error {
	var (
		ctx        = c.UserContext()
		resourceId = c.Params(ResourceKey)
	)
	resExist, err := h.resourceLogic.Exists(ctx, resourceId)
	switch {
	case err != nil:
		h.logger.Error("Failed to check if resource exists",
			zap.Error(err),
		)
		resp := response.UnexpectedError()
		return c.Status(fiber.StatusInternalServerError).JSON(resp)

	case !resExist:
		h.logger.Info("Resource not found",
			zap.String("resource id", resourceId),
		)
		resp := response.BusinessError("resource not found")
		return c.Status(fiber.StatusNotFound).JSON(resp)

	}

	req := &UpdateCustomDataRequest{}
	if err := c.BodyParser(req); err != nil {
		h.logger.Error("failed to parse request body",
			zap.Error(err),
		)
		resp := response.BusinessError("invalid param")
		return c.Status(fiber.StatusBadRequest).JSON(resp)
	}

	if len(req.Content) > 10000 {
		resp := response.BusinessError("cumstom data too long, max length is 10000")
		return c.Status(fiber.StatusBadRequest).JSON(resp)
	}

	if ch, ok := ChannelMap[req.Channel]; ok {
		req.Channel = ch
	} else {
		return errors.New("invalid channel")
	}

	ver, err := h.versionLogic.LoadStoreNewVersionTx(ctx, resourceId, req.VersionName, req.Channel)
	if err != nil {
		h.logger.Error("failed to load store version",
			zap.String("resource id", resourceId),
			zap.String("version name", req.VersionName),
			zap.Error(err),
		)
		resp := response.UnexpectedError()
		return c.Status(fiber.StatusInternalServerError).JSON(resp)
	}

	err = h.versionLogic.UpdateCustomData(ctx, UpdateReleaseNoteSummaryParam{
		VersionID:          ver.ID,
		ReleaseNoteSummary: req.Content,
	})
	if err != nil {
		h.logger.Error("failed to update version custom data",
			zap.String("resource id", resourceId),
			zap.String("version name", req.VersionName),
			zap.Error(err),
		)
		resp := response.UnexpectedError()
		return c.Status(fiber.StatusInternalServerError).JSON(resp)
	}

	h.doEvictCache(resourceId)

	resp := response.Success(nil)
	return c.Status(fiber.StatusOK).JSON(resp)
}

func (h *VersionHandler) doEvictCache(resourceId string) {
	// The cache does not support iteration to temporarily clear all
	cg := h.versionLogic.GetCacheGroup()
	for _, system := range TotalOs {
		for _, arch := range TotalArch {
			for _, channel := range TotalChannel {
				key := cg.GetCacheKey(resourceId, system, arch, channel)
				cg.MultiVersionInfoCache.Delete(key)
			}
		}
	}
}
