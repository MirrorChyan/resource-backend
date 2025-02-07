package handler

import (
	"bytes"
	"context"
	"errors"
	"net/http"
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

var (
	CdkNotfound  = errors.New("no cdk")
	SpIdNotfound = errors.New("no sp_id")
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
		h.logger.Info("Version name already exists",
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

func (h *VersionHandler) validateCDK(cdk, spId, ua, source string) (bool, error) {
	h.logger.Debug("Validating CDK")
	if cdk == "" {
		h.logger.Warn("Missing cdk param")
		return false, CdkNotfound
	}
	request := ValidateCDKRequest{
		CDK:             cdk,
		SpecificationID: spId,
		Source:          source,
		UA:              ua,
	}

	jsonData, err := sonic.Marshal(request)
	if err != nil {
		h.logger.Error("Failed to marshal JSON",
			zap.Error(err),
		)
		return false, err
	}

	var (
		conf   = config.CFG
		agent  = fiber.AcquireAgent()
		req    = agent.Request()
		resp   = fasthttp.AcquireResponse()
		result ValidateCDKResponse
	)
	defer func() {
		fiber.ReleaseAgent(agent)
		fasthttp.ReleaseResponse(resp)
	}()

	req.SetRequestURI(conf.Auth.CDKValidationURL)
	req.Header.SetMethod(fiber.MethodPost)
	req.Header.SetContentType(fiber.MIMEApplicationJSON)
	req.SetBody(jsonData)

	if err := agent.Parse(); err != nil {
		h.logger.Error("Failed to parse request",
			zap.Error(err),
		)
		return false, err
	}

	if err := agent.Do(req, resp); err != nil {
		h.logger.Error("Failed to send request",
			zap.Error(err),
		)
		return false, err
	}

	buf := resp.Body()
	if err := sonic.Unmarshal(buf, &result); err != nil {
		h.logger.Error("Failed to decode response",
			zap.Error(err),
		)
		return false, err
	}
	var code = result.Code

	switch code {
	case 1:
		h.logger.Info("cdk validation failed",
			zap.Int("code", result.Code),
			zap.String("msg", result.Msg),
		)
		return false, RemoteError(result.Msg)
	case -1:
		h.logger.Error("CDK validation failed",
			zap.Int("code", result.Code),
			zap.String("msg", result.Msg),
		)
		return false, errors.New("unknown error")
	}

	return result.Data, nil
}

func (h *VersionHandler) sendBillingCheckinRequest(resID, cdk, userAgent string) {
	request := BillingCheckinRequest{
		CDK:         cdk,
		Application: resID,
		UserAgent:   userAgent,
	}
	body, err := sonic.Marshal(request)
	if err != nil {
		h.logger.Warn("Checkin callback Failed to marshal JSON")
		return
	}

	var conf = config.CFG

	_, err = http.Post(conf.Billing.CheckinURL, fiber.MIMEApplicationJSON, bytes.NewBuffer(body))
	if err != nil {
		h.logger.Warn("Failed to send billing checkin request", zap.Error(err))
	}
}

func (h *VersionHandler) handleGetLatestParam(c *fiber.Ctx) (resID string, req *GetLatestVersionRequest, err error) {
	resID = c.Params(resourceKey)

	req = &GetLatestVersionRequest{}
	if err := c.QueryParser(req); err != nil {
		h.logger.Error("Failed to parse query",
			zap.Error(err),
		)
		return "", nil, errors.New("invalid param")
	}

	resOS, resArch, err := h.doProcessOsAndArch(req.OS, req.Arch)
	if err != nil {
		return "", nil, err
	}

	req.OS, req.Arch = resOS, resArch

	channel, ok := h.handleChannelParam(req.Channel)
	if !ok {
		return "", nil, errors.New("invalid channel")
	}

	req.Channel = channel

	return
}

func (h *VersionHandler) GetLatest(c *fiber.Ctx) error {
	resID, req, err := h.handleGetLatestParam(c)
	if err != nil {
		resp := response.BusinessError(err.Error())
		return c.Status(fiber.StatusBadRequest).JSON(resp)
	}

	ctx := c.UserContext()

	var getLatestFunc func(ctx context.Context, resID string) (*ent.Version, error)

	switch req.Channel {
	case "stable":
		getLatestFunc = h.versionLogic.GetLatestStableVersion
	case "beta":
		getLatestFunc = h.versionLogic.GetLatestBetaVersion
	case "alpha":
		getLatestFunc = h.versionLogic.GetLatestAlphaVersion
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
			OS:            req.OS,
			Arch:          req.Arch,
			CustomData:    latest.CustomData,
			ReleaseNote:   latest.ReleaseNote,
		}
		cdk = req.CDK
	)

	if cdk == "" {
		resp := response.Success(data, "current resource latest version is "+latest.Name)
		return c.Status(fiber.StatusOK).JSON(resp)
	}

	if isFirstBind, err := h.validateCDK(cdk, req.SpID, req.UserAgent, resID); err != nil {
		var e RemoteError
		switch {
		case errors.Is(err, CdkNotfound) || errors.Is(err, SpIdNotfound):
			resp := response.BusinessError(err.Error())
			return c.Status(fiber.StatusBadRequest).JSON(resp)
		case errors.As(err, &e):
			resp := response.BusinessError(e.Error())
			return c.Status(fiber.StatusForbidden).JSON(resp)
		default:
			resp := response.UnexpectedError()
			return c.Status(fiber.StatusInternalServerError).JSON(resp)
		}
	} else if isFirstBind {
		// at-most-once callback
		go h.sendBillingCheckinRequest(resID, cdk, req.UserAgent)
	}

	if latest.Name == req.CurrentVersion {
		resp := response.Success(data, "current version is latest")
		return c.Status(fiber.StatusOK).JSON(resp)
	}

	h.logger.Info("CDK validation success")

	m := c.GetReqHeaders()

	_, ok := m["X-Mirrorc-Hz"]

	url, packageSHA256, updateType, err := h.versionLogic.GetUpdateInfo(ctx, ok, cdk, ProcessUpdateParam{
		ResourceID:         resID,
		CurrentVersionName: req.CurrentVersion,
		TargetVersion:      latest,
		OS:                 req.OS,
		Arch:               req.Arch,
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
