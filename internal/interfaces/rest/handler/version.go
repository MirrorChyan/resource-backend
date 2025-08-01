package handler

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/MirrorChyan/resource-backend/internal/config"
	. "github.com/MirrorChyan/resource-backend/internal/logic/misc"
	"github.com/MirrorChyan/resource-backend/internal/middleware"
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

type tuple struct {
	rid     string
	version string
	ip      string
}

func (h *VersionHandler) getCollector() func(string, string, string) {
	rdb := h.versionLogic.GetRedisClient()

	var ch = make(chan tuple, 1000)

	go func() {
		var ctx = context.Background()
		for {
			select {
			case val := <-ch:
				date := time.Now().Format(time.DateOnly)
				skey := strings.Join([]string{
					VersionPrefix,
					date,
					val.rid,
				}, ":")
				hkey := strings.Join([]string{
					skey,
					val.version,
				}, ":")
				rdb.PFAdd(ctx, hkey, val.ip)
				rdb.SAdd(ctx, skey, val.version)
			}
		}
	}()

	return func(rid string, version string, ip string) {
		arr := strings.Split(ip, ",")
		if len(arr) >= 2 {
			ip = arr[0]
		}
		ch <- tuple{
			rid:     rid,
			version: version,
			ip:      ip,
		}
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

	var data = &QueryLatestResponseData{
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
			data.ReleaseNote = "placeholder"
		}
		resp := response.Success(data, "current resource latest version is "+latest.VersionName)
		return c.JSON(resp)
	}

	ts, err := h.doValidateCDK(param, resourceId, ip)
	if err != nil {
		var biz *errs.Error
		if errors.As(err, &biz) {
			return biz.WithDetails(data)
		}
		return err
	}

	if latest.VersionName == currentVersion {
		data.ReleaseNote = "placeholder"
		resp := response.Success(data, "current version is latest")
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

	data.SHA256 = result.SHA256
	data.Filesize = result.Filesize
	data.UpdateType = result.UpdateType
	data.CustomData = latest.CustomData
	data.CDKExpiredTime = ts
	data.Url = url

	return c.JSON(response.Success(data))
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
			zap.String("input", string(c.Body())),
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
