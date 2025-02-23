package handler

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	. "github.com/MirrorChyan/resource-backend/internal/logic/misc"
	"github.com/redis/go-redis/v9"
	"github.com/valyala/fasthttp"

	"github.com/MirrorChyan/resource-backend/internal/config"
	"github.com/MirrorChyan/resource-backend/internal/middleware"
	"github.com/MirrorChyan/resource-backend/internal/vercomp"
	"github.com/bytedance/sonic"

	"github.com/MirrorChyan/resource-backend/internal/ent"
	"github.com/MirrorChyan/resource-backend/internal/handler/response"
	. "github.com/MirrorChyan/resource-backend/internal/logic"
	"github.com/MirrorChyan/resource-backend/internal/logic/misc"
	. "github.com/MirrorChyan/resource-backend/internal/model"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

type VersionHandler struct {
	logger        *zap.Logger
	resourceLogic *ResourceLogic
	versionLogic  *VersionLogic
	verComparator *vercomp.VersionComparator
}

func NewVersionHandler(
	logger *zap.Logger,
	resourceLogic *ResourceLogic,
	versionLogic *VersionLogic,
	verComparator *vercomp.VersionComparator,
) *VersionHandler {
	return &VersionHandler{
		logger:        logger,
		resourceLogic: resourceLogic,
		versionLogic:  versionLogic,
		verComparator: verComparator,
	}
}

func (h *VersionHandler) Register(r fiber.Router) {

	// for daily active user
	dau := middleware.NewDailyActiveUserRecorder(h.versionLogic.GetRedisClient())

	r.Get("/resources/:rid/latest", dau, h.GetLatestV2)
	r.Get("/resources/download/:key", h.RedirectToDownload)

	// For Developer
	versions := r.Group("/resources/:rid/versions")
	versions.Use("/", middleware.NewValidateUploader())
	versions.Post("/", h.Create)

	versions.Put("/release-note", h.UpdateReleaseNote)
	versions.Put("/custom-data", h.UpdateCustomData)
}

func (h *VersionHandler) isValidExtension(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return ext == ".zip" || strings.HasSuffix(filename, ".tar.gz")
}

func (h *VersionHandler) BindRequiredParams(os, arch, channel *string) error {
	if o, ok := OsMap[*os]; !ok {
		return errors.New("invalid os")
	} else {
		*os = o
	}
	if a, ok := ArchMap[*arch]; !ok {
		return errors.New("invalid arch")
	} else {
		*arch = a
	}

	if c, ok := ChannelMap[*channel]; !ok {
		return errors.New("invalid channel")
	} else {
		*channel = c
	}
	return nil
}

func (h *VersionHandler) Create(c *fiber.Ctx) error {
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

	verName := c.FormValue("name")
	file, err := c.FormFile("file")
	if err != nil {
		h.logger.Error("Failed to get file from form",
			zap.Error(err),
		)
		resp := response.BusinessError("invalid file")
		return c.Status(fiber.StatusBadRequest).JSON(resp)
	}

	if !h.isValidExtension(file.Filename) {
		resp := response.BusinessError("invalid file extension")
		return c.Status(fiber.StatusBadRequest).JSON(resp)
	}

	var (
		system  = c.FormValue("os")
		arch    = c.FormValue("arch")
		channel = c.FormValue("channel")
	)
	err = h.BindRequiredParams(&system, &arch, &channel)
	if err != nil {
		resp := response.BusinessError(err.Error())
		return c.Status(fiber.StatusBadRequest).JSON(resp)
	}

	if channel != misc.TypeStable {
		parsable := h.verComparator.IsVersionParsable(verName)
		if !parsable {
			resp := response.BusinessError("version name is not supported for parsing, please use the stable channel")
			return c.Status(fiber.StatusBadRequest).JSON(resp)
		}
	}

	exists, err := h.versionLogic.ExistNameWithOSAndArch(ctx, ExistVersionNameWithOSAndArchParam{
		ResourceId:  resourceId,
		VersionName: verName,
		OS:          system,
		Arch:        arch,
	})
	switch {
	case err != nil:
		h.logger.Error("failed to check if version name exists",
			zap.Error(err),
		)
		resp := response.UnexpectedError()
		return c.Status(fiber.StatusInternalServerError).JSON(resp)
	case exists:
		h.logger.Warn("version name already exists",
			zap.String("resource id", resourceId),
			zap.String("version name", verName),
			zap.String("resource os", system),
			zap.String("resource arch", arch),
		)
		resp := response.BusinessError("version name under the current platform architecture already exists")
		return c.Status(fiber.StatusConflict).JSON(resp)
	}

	tmp, err := os.MkdirTemp(os.TempDir(), misc.TmpDirPrefix)
	if err != nil {
		h.logger.Error("Failed to create temp tmp directory",
			zap.Error(err),
		)
		resp := response.UnexpectedError()
		return c.Status(fiber.StatusInternalServerError).JSON(resp)
	}

	defer func(path string) {
		go func(p string) {
			_ = os.RemoveAll(p)
		}(path)
	}(tmp)

	dest := strings.Join([]string{tmp, file.Filename}, string(os.PathSeparator))
	if err := c.SaveFile(file, dest); err != nil {
		h.logger.Error("failed to save file",
			zap.Error(err),
		)
		resp := response.UnexpectedError()
		return c.Status(fiber.StatusInternalServerError).JSON(resp)
	}

	ver, err := h.versionLogic.Create(ctx, CreateVersionParam{
		ResourceID:        resourceId,
		Name:              verName,
		UploadArchivePath: dest,
		OS:                system,
		Arch:              arch,
		Channel:           channel,
	})
	if err != nil {
		h.logger.Error("Failed to create version",
			zap.Error(err),
		)
		resp := response.UnexpectedError()
		return c.Status(fiber.StatusInternalServerError).JSON(resp)
	}

	data := CreateVersionResponseData{
		Name:   ver.Name,
		Number: ver.Number,
		OS:     system,
		Arch:   arch,
	}
	return c.Status(fiber.StatusCreated).JSON(response.Success(data))
}

func (h *VersionHandler) doValidateCDK(info *GetLatestVersionRequest, resourceId, ip string) error {
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
		return err
	}

	var (
		conf    = config.GConfig
		agent   = fiber.AcquireAgent()
		request = agent.Request()
		resp    = fasthttp.AcquireResponse()
		result  ValidateCDKResponse
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
		return err
	}

	if err := agent.Do(request, resp); err != nil {
		h.logger.Error("Failed to send request",
			zap.Error(err),
		)
		return err
	}

	if err := sonic.Unmarshal(resp.Body(), &result); err != nil {
		h.logger.Error("Failed to decode response",
			zap.Error(err),
		)
		return err
	}

	switch result.Code {
	case 1:
		h.logger.Info("cdk validation failed",
			zap.Int("code", result.Code),
			zap.String("msg", result.Msg),
		)
		return RemoteError(result.Msg)
	case -1:
		h.logger.Error("CDK validation failed",
			zap.Int("code", result.Code),
			zap.String("msg", result.Msg),
		)
		return errors.New("unknown error")
	}
	h.logger.Info("CDK validation success")
	return nil
}

func (h *VersionHandler) doHandleGetLatestParam(c *fiber.Ctx) (*GetLatestVersionRequest, error) {

	var request GetLatestVersionRequest

	if err := c.QueryParser(&request); err != nil {
		h.logger.Error("Failed to parse query",
			zap.Error(err),
		)
		return nil, errors.New("invalid param")
	}

	request.ResourceID = c.Params(ResourceKey)

	err := h.BindRequiredParams(&request.OS, &request.Arch, &request.Channel)
	if err != nil {
		return nil, err
	}
	return &request, nil
}

func (h *VersionHandler) GetLatest(c *fiber.Ctx) error {
	param, err := h.doHandleGetLatestParam(c)
	if err != nil {
		resp := response.BusinessError(err.Error())
		return c.Status(fiber.StatusBadRequest).JSON(resp)
	}

	var (
		resID         = param.ResourceID
		ctx           = c.UserContext()
		getLatestFunc func(context.Context, string) (*ent.Version, error)
	)

	switch param.Channel {
	case "alpha":
		getLatestFunc = h.versionLogic.GetLatestAlphaVersion
	case "beta":
		getLatestFunc = h.versionLogic.GetLatestBetaVersion
	default:
		getLatestFunc = h.versionLogic.GetLatestStableVersion
	}

	latest, err := getLatestFunc(ctx, resID)

	if err != nil {
		if ent.IsNotFound(err) {
			resp := response.BusinessError("resources can't be found")
			return c.Status(fiber.StatusNotFound).JSON(resp)
		}

		h.logger.Error("Failed to get latest version",
			zap.Error(err),
		)
		return c.Status(fiber.StatusInternalServerError).
			JSON(response.UnexpectedError())
	}

	var (
		data = QueryLatestResponseData{
			VersionName:   latest.Name,
			VersionNumber: latest.Number,
			Channel:       latest.Channel.String(),
			OS:            param.OS,
			Arch:          param.Arch,
			ReleaseNote:   latest.ReleaseNote,
		}
		cdk     = param.CDK
		con     = config.GConfig.Extra.Concurrency
		counter = CompareIfAbsent(LIT, resID)
	)

	counter.Add(1)
	defer func() {
		counter.Add(-1)
	}()

	if cdk == "" {
		if latest.Name == param.CurrentVersion {
			data.ReleaseNote = "placeholder"
		}
		resp := response.Success(data, "current resource latest version is "+latest.Name)
		return c.Status(fiber.StatusOK).JSON(resp)
	}

	// limit concurrent requests by download
	if con != 0 {
		if cv := counter.Load(); cv > con {
			data.VersionName = param.CurrentVersion
			resp := response.Success(data, "current resource latest version is "+latest.Name)
			h.logger.Info("limit by", zap.Int32("concurrency", cv))
			return c.Status(fiber.StatusMultiStatus).JSON(resp)
		}
	}

	if err := h.doValidateCDK(param, resID, c.IP()); err != nil {
		var e RemoteError
		if errors.As(err, &e) {
			resp := response.BusinessError(e.Error())
			return c.Status(fiber.StatusForbidden).JSON(resp)
		} else {
			resp := response.UnexpectedError()
			return c.Status(fiber.StatusInternalServerError).JSON(resp)
		}
	}

	if latest.Name == param.CurrentVersion {
		data.ReleaseNote = "placeholder"
		resp := response.Success(data, "current version is latest")
		return c.Status(fiber.StatusOK).JSON(resp)
	}

	region := string(c.Request().Header.Peek(RegionHeaderKey))
	if region == "" {
		region = config.GConfig.Instance.RegionId
	}

	info, err := h.versionLogic.GetUpdateInfo(ctx, ProcessUpdateParam{
		ResourceId:         resID,
		CurrentVersionName: param.CurrentVersion,
		TargetVersion:      latest,
		OS:                 param.OS,
		Arch:               param.Arch,
	})

	if err != nil {

		if errors.Is(err, StorageInfoNotFound) {
			resp := response.BusinessError("the corresponding resource does not exist")
			return c.Status(fiber.StatusNotFound).JSON(resp)
		}

		h.logger.Error("failed to get update info",
			zap.Error(err),
		)
		resp := response.UnexpectedError()
		return c.Status(fiber.StatusInternalServerError).JSON(resp)
	}

	url, err := h.versionLogic.GetDistributeURL(&DistributeInfo{
		Region:   region,
		CDK:      cdk,
		RelPath:  info.RelPath,
		Resource: resID,
	})

	if err != nil {
		h.logger.Error("failed to get download url",
			zap.Error(err),
		)
		resp := response.UnexpectedError()
		return c.Status(fiber.StatusInternalServerError).JSON(resp)
	}

	data.Url = url
	data.SHA256 = info.SHA256
	data.UpdateType = info.UpdateType
	data.CustomData = latest.CustomData

	return c.Status(fiber.StatusOK).JSON(response.Success(data))
}

func (h *VersionHandler) GetLatestV2(c *fiber.Ctx) error {
	param, err := h.doHandleGetLatestParam(c)
	if err != nil {
		resp := response.BusinessError(err.Error())
		return c.Status(fiber.StatusBadRequest).JSON(resp)
	}

	var (
		ctx            = c.UserContext()
		resourceId     = param.ResourceID
		system         = param.OS
		arch           = param.Arch
		channel        = param.Channel
		currentVersion = param.CurrentVersion
		cdk            = param.CDK
	)

	latest, err := h.versionLogic.GetMultiLatestVersionInfo(resourceId, system, arch, channel)
	if err != nil {
		if errors.Is(err, misc.ResourceNotFound) {
			resp := response.BusinessError("resource not found under current platform architecture")
			return c.Status(fiber.StatusNotFound).JSON(resp)
		}
		h.logger.Error("Failed to get latest version",
			zap.Error(err),
		)
		return c.Status(fiber.StatusInternalServerError).JSON(response.UnexpectedError())
	}

	var (
		data = QueryLatestResponseData{
			VersionName:   latest.VersionName,
			VersionNumber: latest.VersionNumber,
			ReleaseNote:   latest.ReleaseNote,
			Channel:       channel,
			OS:            param.OS,
			Arch:          param.Arch,
		}
	)

	if cdk == "" {
		if latest.VersionName == currentVersion {
			data.ReleaseNote = "placeholder"
		}
		resp := response.Success(data, "current resource latest version is "+latest.VersionName)
		return c.Status(fiber.StatusOK).JSON(resp)
	}

	release, limited := h.doLimitByConfig(resourceId)
	defer release()
	if limited {
		data.VersionName = param.CurrentVersion
		resp := response.Success(data, "current resource latest version is "+latest.VersionName)
		return c.Status(fiber.StatusMultiStatus).JSON(resp)
	}

	//if err := h.doValidateCDK(param, resourceId, c.IP()); err != nil {
	//	var e RemoteError
	//	if errors.As(err, &e) {
	//		resp := response.BusinessError(e.Error())
	//		return c.Status(fiber.StatusForbidden).JSON(resp)
	//	} else {
	//		resp := response.UnexpectedError()
	//		return c.Status(fiber.StatusInternalServerError).JSON(resp)
	//	}
	//}

	if latest.VersionName == currentVersion {
		data.ReleaseNote = "placeholder"
		resp := response.Success(data, "current version is latest")
		return c.Status(fiber.StatusOK).JSON(resp)
	}

	result, err := h.versionLogic.GetUpdateInfoV2(ctx, UpdateRequestParam{
		ResourceId:         resourceId,
		CurrentVersionName: currentVersion,
		TargetVersionInfo:  latest,
	})
	if err != nil {
		return err
	}
	data.SHA256 = result.SHA256
	data.UpdateType = result.UpdateType

	s, _ := sonic.MarshalString(result)

	data.CustomData = s
	data.ReleaseNote = ""

	region := h.doPickRegionInfo(c)
	url, err := h.versionLogic.GetDistributeURL(&DistributeInfo{
		Region:   region,
		CDK:      cdk,
		RelPath:  result.RelPath,
		Resource: resourceId,
	})

	data.Url = url
	return c.Status(fiber.StatusOK).JSON(response.Success(data))

}

func (h *VersionHandler) doLimitByConfig(resourceId string) (func(), bool) {
	var (
		counter = CompareIfAbsent(LIT, resourceId)
		con     = config.GConfig.Extra.Concurrency
	)
	counter.Add(1)

	if con != 0 {
		if cv := counter.Load(); cv > con {
			h.logger.Warn("limit by", zap.Int32("concurrency", cv))
			return nil, true
		}
	}

	return func() {
		counter.Add(-1)
	}, false
}

func (h *VersionHandler) doPickRegionInfo(c *fiber.Ctx) string {
	region := string(c.Request().Header.Peek(RegionHeaderKey))
	if region == "" {
		region = config.GConfig.Instance.RegionId
	}
	return region
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
		if errors.Is(err, misc.ResourceLimitError) {
			return c.Status(fiber.StatusForbidden).SendString(err.Error())
		}

		h.logger.Error("failed to RedirectToDownload",
			zap.Error(err),
		)
		return c.Status(fiber.StatusInternalServerError).JSON(response.UnexpectedError())
	}
	h.logger.Info("RedirectToDownload",
		zap.String("distribute key", rk),
		zap.String("download url", url),
	)
	return c.Redirect(url)
}

func (h *VersionHandler) UpdateReleaseNote(c *fiber.Ctx) error {
	ctx := c.UserContext()

	resID := c.Params(ResourceKey)
	resExist, err := h.resourceLogic.Exists(ctx, resID)
	switch {
	case err != nil:
		h.logger.Error("Failed to check if resource exists",
			zap.Error(err),
		)
		resp := response.UnexpectedError()
		return c.Status(fiber.StatusInternalServerError).JSON(resp)

	case !resExist:
		h.logger.Info("Resource not found",
			zap.String("resource id", resID),
		)
		resp := response.BusinessError("resource not found")
		return c.Status(fiber.StatusNotFound).JSON(resp)

	}

	req := &UpdateReleaseNoteDetailRequest{}
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

	ver, err := h.versionLogic.GetVersionByName(ctx, GetVersionByNameParam{
		ResourceID:  resID,
		VersionName: req.VersionName,
	})
	switch {
	case ent.IsNotFound(err):
		h.logger.Info("version not found",
			zap.String("resource id", resID),
			zap.String("version name", req.VersionName),
		)
		resp := response.BusinessError("version not found")
		return c.Status(fiber.StatusNotFound).JSON(resp)
	case err != nil:
		h.logger.Error("failed to check if version exists",
			zap.String("resource id", resID),
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
			zap.String("resource id", resID),
			zap.String("version name", req.VersionName),
			zap.Error(err),
		)
		resp := response.UnexpectedError()
		return c.Status(fiber.StatusInternalServerError).JSON(resp)
	}

	h.clearAllChannelCache(resID)

	resp := response.Success(nil)
	return c.Status(fiber.StatusOK).JSON(resp)
}

func (h *VersionHandler) UpdateCustomData(c *fiber.Ctx) error {
	ctx := c.UserContext()

	resID := c.Params(ResourceKey)
	resExist, err := h.resourceLogic.Exists(ctx, resID)
	switch {
	case err != nil:
		h.logger.Error("Failed to check if resource exists",
			zap.Error(err),
		)
		resp := response.UnexpectedError()
		return c.Status(fiber.StatusInternalServerError).JSON(resp)

	case !resExist:
		h.logger.Info("Resource not found",
			zap.String("resource id", resID),
		)
		resp := response.BusinessError("resource not found")
		return c.Status(fiber.StatusNotFound).JSON(resp)

	}

	req := &UpdateReleaseNoteDetailRequest{}
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

	ver, err := h.versionLogic.GetVersionByName(ctx, GetVersionByNameParam{
		ResourceID:  resID,
		VersionName: req.VersionName,
	})
	switch {
	case ent.IsNotFound(err):
		h.logger.Info("version not found",
			zap.String("resource id", resID),
			zap.String("version name", req.VersionName),
		)
		resp := response.BusinessError("version not found")
		return c.Status(fiber.StatusNotFound).JSON(resp)
	case err != nil:
		h.logger.Error("failed to check if version exists",
			zap.String("resource id", resID),
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
			zap.String("resource id", resID),
			zap.String("version name", req.VersionName),
			zap.Error(err),
		)
		resp := response.UnexpectedError()
		return c.Status(fiber.StatusInternalServerError).JSON(resp)
	}

	h.clearAllChannelCache(resID)

	resp := response.Success(nil)
	return c.Status(fiber.StatusOK).JSON(resp)
}

func (h *VersionHandler) clearAllChannelCache(resourceId string) {
	var (
		cg    = h.versionLogic.GetCacheGroup()
		cache = cg.VersionLatestCache
	)
	for _, ch := range AllChannel {
		key := cg.GetCacheKey(resourceId, ch)
		cache.Delete(key)
	}
}
