package handler

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/MirrorChyan/resource-backend/internal/config"
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
}

func NewVersionHandler(
	logger *zap.Logger,
	resourceLogic *logic.ResourceLogic,
	versionLogic *logic.VersionLogic,
) *VersionHandler {
	return &VersionHandler{
		logger:        logger,
		resourceLogic: resourceLogic,
		versionLogic:  versionLogic,
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
	// stable channel
	r.Get("/resources/:rid/latest", h.GetLatestStable)
	r.Get("/resources/:rid/stable/latest", h.GetLatestStable)
	// beta channel
	r.Get("/resources/:rid/beta/latest", h.GetLatestBeta)
	// alpha channel
	r.Get("/rerources/:rid/alpha/latest", h.GetLatestAlpha)

	// For Developer
	r.Use("/resources/:rid/versions", h.ValidateUploader)
	r.Post("/resources/:rid/versions", h.Create)
}

func (h *VersionHandler) ValidateUploader(c *fiber.Ctx) error {
	token := c.Get("Authorization")
	if token == "" {
		resp := response.BusinessError("missing Authorization header")
		return c.Status(fiber.StatusUnauthorized).JSON(resp)
	}

	var conf = config.CFG

	url := fmt.Sprintf("%s?token=%s", conf.Auth.UploaderValidationURL, token)
	resp, err := http.Post(url, "application/json", nil)
	if err != nil {
		h.logger.Error("Failed to request uploader validation",
			zap.Error(err),
		)
		resp := response.UnexpectedError()
		return c.Status(fiber.StatusInternalServerError).JSON(resp)
	}
	defer func(b io.ReadCloser) {
		err := b.Close()
		if err != nil {
			h.logger.Error("Failed to close response body")
		}
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		h.logger.Error("Request uploader validation status code not 200",
			zap.Int("status code", resp.StatusCode),
		)
		resp := response.UnexpectedError()
		return c.Status(fiber.StatusUnauthorized).JSON(resp)
	}

	var res ValidateUploaderResponse
	buf, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if err := sonic.Unmarshal(buf, &res); err != nil {
		h.logger.Error("Failed to decode response body",
			zap.Error(err),
		)
		resp := response.UnexpectedError()
		return c.Status(fiber.StatusInternalServerError).JSON(resp)
	}

	if res.Code == 1 {
		h.logger.Info("Uploader validation failed",
			zap.Int("code", res.Code),
			zap.String("msg", res.Msg),
		)
		resp := response.BusinessError("invalid authorization token")
		return c.Status(fiber.StatusUnauthorized).JSON(resp)
	} else if res.Code == -1 {
		h.logger.Error("Uploader validation failed",
			zap.Int("code", res.Code),
			zap.String("msg", res.Msg),
		)
		resp := response.UnexpectedError()
		return c.Status(fiber.StatusInternalServerError).JSON(resp)
	}

	return c.Next()
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

func (h *VersionHandler) doProcessOsAndArch(c *fiber.Ctx) (string, string, error) {
	var (
		resOS   = c.FormValue("os")
		resArch = c.FormValue("arch")
	)
	resOS, ok := h.handleOSParam(resOS)
	if !ok {
		return "", "", errors.New("invalid os")
	}

	resArch, ok = h.handleArchParam(resArch)
	if !ok {
		return "", "", errors.New("invalid arch")
	}
	return resOS, resArch, nil
}

var channelMap = map[string]string{
	// stable
	"":       "stable",
	"stable": "stable",

	// beta
	"beta": "beta",

	// alpha
	"alpha": "alpha",
}

func (h *VersionHandler) handleChannelParam(channel string) (string, bool) {
	if standardChannel, ok := channelMap[channel]; ok {
		return standardChannel, true
	}
	return "", false
}

func (h *VersionHandler) Create(c *fiber.Ctx) error {
	resID := c.Params(resourceKey)
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

	resourceOS, resourceArch, err := h.doProcessOsAndArch(c)
	if err != nil {
		resp := response.BusinessError(err.Error())
		return c.Status(fiber.StatusBadRequest).JSON(resp)
	}

	channel := c.FormValue("channel")
	channel, ok := h.handleChannelParam(channel)
	if !ok {
		resp := response.BusinessError("invalid channel")
		return c.Status(fiber.StatusBadRequest).JSON(resp)
	}

	var ctx = c.UserContext()

	exists, err := h.versionLogic.NameExists(ctx, VersionNameExistsParam{
		ResourceID: resID,
		Name:       verName,
		OS:         resourceOS,
		Arch:       resourceArch,
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
			zap.String("resource os", resourceOS),
			zap.String("resource arch", resourceArch),
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

	version, err := h.versionLogic.Create(ctx, CreateVersionParam{
		ResourceID:        resID,
		Name:              verName,
		UploadArchivePath: dest,
		OS:                resourceOS,
		Arch:              resourceArch,
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
		ID:     version.ID,
		Name:   version.Name,
		Number: version.Number,
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

	var conf = config.CFG
	resp, err := http.Post(conf.Auth.CDKValidationURL, fiber.MIMEApplicationJSON, bytes.NewBuffer(jsonData))
	if err != nil {
		h.logger.Error("Failed to send request",
			zap.Error(err),
		)
		return false, err
	}

	var result ValidateCDKResponse
	buf, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, err
	}
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

	resOS, resArch, err := h.doProcessOsAndArch(c)
	if err != nil {
		return "", nil, err
	}

	req.OS, req.Arch = resOS, resArch

	return
}

func (h *VersionHandler) handleGetLatest(c *fiber.Ctx, getLatestFunc func(ctx context.Context, resID string) (*ent.Version, error)) error {
	resID, req, err := h.handleGetLatestParam(c)
	if err != nil {
		resp := response.BusinessError(err.Error())
		return c.Status(fiber.StatusBadRequest).JSON(resp)
	}

	ctx := c.UserContext()

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

	data := QueryLatestResponseData{
		VersionName:   latest.Name,
		VersionNumber: latest.Number,
	}

	if req.CDK == "" {
		resp := response.Success(data, "current resource latest version is "+latest.Name)
		return c.Status(fiber.StatusOK).JSON(resp)
	}

	if isFirstBind, err := h.validateCDK(req.CDK, req.SpID, req.UserAgent, resID); err != nil {
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
		go h.sendBillingCheckinRequest(resID, req.CDK, req.UserAgent)
	}

	if latest.Name == req.CurrentVersion {
		resp := response.Success(data, "current version is latest")
		return c.Status(fiber.StatusOK).JSON(resp)
	}

	h.logger.Info("CDK validation success")

	url, err := h.versionLogic.GetDownloadUrl(ctx, ProcessUpdateParam{
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

	data.Url = url

	return c.Status(fiber.StatusOK).JSON(response.Success(data))
}

func (h *VersionHandler) GetLatestStable(c *fiber.Ctx) error {
	return h.handleGetLatest(c, h.versionLogic.GetLatestStableVersion)
}

func (h *VersionHandler) GetLatestBeta(c *fiber.Ctx) error {
	return h.handleGetLatest(c, h.versionLogic.GetLatestBetaVersion)
}

func (h *VersionHandler) GetLatestAlpha(c *fiber.Ctx) error {
	return h.handleGetLatest(c, h.versionLogic.GetLatestAlphaVersion)
}
