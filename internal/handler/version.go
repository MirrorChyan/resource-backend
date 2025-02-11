package handler

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/valyala/fasthttp"

	"github.com/MirrorChyan/resource-backend/internal/config"
	"github.com/MirrorChyan/resource-backend/internal/ent/version"
	"github.com/MirrorChyan/resource-backend/internal/middleware"
	"github.com/MirrorChyan/resource-backend/internal/vercomp"
	"github.com/bytedance/sonic"

	"github.com/MirrorChyan/resource-backend/internal/ent"
	"github.com/MirrorChyan/resource-backend/internal/handler/response"
	"github.com/MirrorChyan/resource-backend/internal/logic"
	. "github.com/MirrorChyan/resource-backend/internal/model"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

type VersionHandler struct {
	logger        *zap.Logger
	resourceLogic *logic.ResourceLogic
	versionLogic  *logic.VersionLogic
	verComparator *vercomp.VersionComparator
}

func NewVersionHandler(
	logger *zap.Logger,
	resourceLogic *logic.ResourceLogic,
	versionLogic *logic.VersionLogic,
	verComparator *vercomp.VersionComparator,
) *VersionHandler {
	return &VersionHandler{
		logger:        logger,
		resourceLogic: resourceLogic,
		versionLogic:  versionLogic,
		verComparator: verComparator,
	}
}

const (
	resourceKey = "rid"
)

type RemoteError string

func (r RemoteError) Error() string {
	return string(r)
}

func (h *VersionHandler) Register(r fiber.Router) {

	// for daily active user
	dau := middleware.NewDailyActiveUserRecorder(h.versionLogic.GetRedisClient())

	r.Get("/resources/:rid/latest", dau, h.GetLatest)

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

var (
	osMap = map[string]string{
		// any
		"": "",

		// windows
		"windows": "windows",
		"win":     "windows",
		"win32":   "windows",

		// linux
		"linux": "linux",

		// darwin
		"darwin": "darwin",
		"macos":  "darwin",
		"mac":    "darwin",
		"osx":    "darwin",

		// android
		"android": "android",
	}

	archMap = map[string]string{
		// any
		"": "",

		// 386
		"386":    "386",
		"x86":    "386",
		"x86_32": "386",
		"i386":   "386",

		// amd64
		"amd64":   "amd64",
		"x64":     "amd64",
		"x86_64":  "amd64",
		"intel64": "amd64",

		// arm
		"arm": "arm",

		// arm64
		"arm64":   "arm64",
		"aarch64": "arm64",
	}

	channelMap = map[string]string{
		// stable
		"":       "stable",
		"stable": "stable",

		// beta
		"beta": "beta",

		// alpha
		"alpha": "alpha",
	}
)

func (h *VersionHandler) handleOSParam(os string) (string, bool) {
	if standardOS, ok := osMap[os]; ok {
		return standardOS, true
	}
	return "", false
}

func (h *VersionHandler) handleArchParam(arch string) (string, bool) {
	if standardArch, ok := archMap[arch]; ok {
		return standardArch, true
	}
	return "", false
}

func (h *VersionHandler) doProcessOsAndArch(inputOS string, inputArch string) (resOS string, resArch string, err error) {
	resOS, ok := h.handleOSParam(inputOS)
	if !ok {
		return "", "", errors.New("invalid os")
	}

	resArch, ok = h.handleArchParam(inputArch)
	if !ok {
		return "", "", errors.New("invalid arch")
	}

	return
}

func (h *VersionHandler) handleChannelParam(channel string) (string, bool) {
	if standardChannel, ok := channelMap[channel]; ok {
		return standardChannel, true
	}
	return "", false
}

func (h *VersionHandler) Create(c *fiber.Ctx) error {
	var ctx = c.UserContext()

	resID := c.Params(resourceKey)
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

	resOS := c.FormValue("os")
	resArch := c.FormValue("arch")
	resOS, resArch, err = h.doProcessOsAndArch(resOS, resArch)
	if err != nil {
		resp := response.BusinessError(err.Error())
		return c.Status(fiber.StatusBadRequest).JSON(resp)
	}

	ch := c.FormValue("channel")
	channel, ok := h.handleChannelParam(ch)
	if !ok {
		resp := response.BusinessError("invalid channel")
		return c.Status(fiber.StatusBadRequest).JSON(resp)
	}

	if channel != version.ChannelStable.String() {
		parsable := h.verComparator.IsVersionParsable(verName)
		if !parsable {
			resp := response.BusinessError("version name is not supported for parsing, please use the stable channel")
			return c.Status(fiber.StatusBadRequest).JSON(resp)
		}
	}

	exists, err := h.versionLogic.ExistNameWithOSAndArch(ctx, ExistVersionNameWithOSAndArchParam{
		ResourceID:  resID,
		VersionName: verName,
		OS:          resOS,
		Arch:        resArch,
	})
	switch {
	case err != nil:
		h.logger.Error("Failed to check if version name exists",
			zap.Error(err),
		)
		resp := response.UnexpectedError()
		return c.Status(fiber.StatusInternalServerError).JSON(resp)
	case exists:
		h.logger.Warn("Version name already exists",
			zap.String("resource id", resID),
			zap.String("version name", verName),
			zap.String("resource os", resOS),
			zap.String("resource arch", resArch),
		)
		resp := response.BusinessError("version name under the current platform architecture already exists")
		return c.Status(fiber.StatusConflict).JSON(resp)
	}

	// create temp root dir
	root, err := os.MkdirTemp(os.TempDir(), "process-temp")
	if err != nil {
		h.logger.Error("Failed to create temp root directory",
			zap.Error(err),
		)
		resp := response.UnexpectedError()
		return c.Status(fiber.StatusInternalServerError).JSON(resp)
	}
	// remove temp root dir
	defer func(path string) {
		go func(p string) {
			_ = os.RemoveAll(p)
		}(path)
	}(root)

	dest := strings.Join([]string{root, file.Filename}, string(os.PathSeparator))
	if err := c.SaveFile(file, dest); err != nil {
		h.logger.Error("failed to save file",
			zap.Error(err),
		)
		resp := response.UnexpectedError()
		return c.Status(fiber.StatusInternalServerError).JSON(resp)
	}

	ver, err := h.versionLogic.Create(ctx, CreateVersionParam{
		ResourceID:        resID,
		Name:              verName,
		UploadArchivePath: dest,
		OS:                resOS,
		Arch:              resArch,
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
		OS:     resOS,
		Arch:   resArch,
	}
	return c.Status(fiber.StatusCreated).JSON(response.Success(data))
}

func (h *VersionHandler) doValidateCDK(info *GetLatestVersionRequest, resId, ip string) error {
	h.logger.Info("Validating CDK")

	body, err := sonic.Marshal(ValidateCDKRequest{
		CDK:      info.CDK,
		Resource: resId,
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

func (h *VersionHandler) handleGetLatestParam(c *fiber.Ctx) (*GetLatestVersionRequest, error) {

	var (
		request GetLatestVersionRequest
	)

	if err := c.QueryParser(&request); err != nil {
		h.logger.Error("Failed to parse query",
			zap.Error(err),
		)
		return nil, errors.New("invalid param")
	}

	request.ResourceID = c.Params(resourceKey)

	resOS, resArch, err := h.doProcessOsAndArch(request.OS, request.Arch)
	if err != nil {
		return nil, err
	}

	request.OS, request.Arch = resOS, resArch

	channel, ok := h.handleChannelParam(request.Channel)
	if !ok {
		return nil, errors.New("invalid channel")
	}

	request.Channel = channel

	return &request, nil
}

func (h *VersionHandler) GetLatest(c *fiber.Ctx) error {
	param, err := h.handleGetLatestParam(c)
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
			CustomData:    latest.CustomData,
			ReleaseNote:   latest.ReleaseNote,
		}
		cdk = param.CDK
	)

	if cdk == "" {
		resp := response.Success(data, "current resource latest version is "+latest.Name)
		return c.Status(fiber.StatusOK).JSON(resp)
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
		resp := response.Success(data, "current version is latest")
		return c.Status(fiber.StatusOK).JSON(resp)
	}

	m := c.GetReqHeaders()

	_, ok := m["X-Mirrorc-Hz"]

	url, packageSHA256, updateType, err := h.versionLogic.GetUpdateInfo(ctx, ok, cdk, ProcessUpdateParam{
		ResourceID:         resID,
		CurrentVersionName: param.CurrentVersion,
		TargetVersion:      latest,
		OS:                 param.OS,
		Arch:               param.Arch,
	})

	if err != nil {

		if errors.Is(err, logic.StorageInfoNotFound) {
			resp := response.BusinessError("the corresponding resource does not exist")
			return c.Status(fiber.StatusNotFound).JSON(resp)
		}

		h.logger.Error("failed to get download url",
			zap.Error(err),
		)
		resp := response.UnexpectedError()
		return c.Status(fiber.StatusInternalServerError).JSON(resp)
	}

	data.Url, data.SHA256, data.UpdateType = url, packageSHA256, updateType

	return c.Status(fiber.StatusOK).JSON(response.Success(data))
}

func (h *VersionHandler) UpdateReleaseNote(c *fiber.Ctx) error {
	ctx := c.UserContext()

	resID := c.Params(resourceKey)
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
		resp := response.BusinessError("release note too long, max length is 10000")
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

	resp := response.Success(nil)
	return c.Status(fiber.StatusOK).JSON(resp)
}

func (h *VersionHandler) UpdateCustomData(c *fiber.Ctx) error {
	ctx := c.UserContext()

	resID := c.Params(resourceKey)
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

	resp := response.Success(nil)
	return c.Status(fiber.StatusOK).JSON(resp)
}
