package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/MirrorChyan/resource-backend/internal/config"
	"github.com/MirrorChyan/resource-backend/internal/db"
	"github.com/MirrorChyan/resource-backend/internal/ent"
	"github.com/MirrorChyan/resource-backend/internal/handler/response"
	"github.com/MirrorChyan/resource-backend/internal/logic"
	. "github.com/MirrorChyan/resource-backend/internal/model"
	"github.com/gofiber/fiber/v2"
	"github.com/segmentio/ksuid"
	"go.uber.org/zap"
)

type VersionHandler struct {
	conf          *config.Config
	logger        *zap.Logger
	resourceLogic *logic.ResourceLogic
	versionLogic  *logic.VersionLogic
	storageLogic  *logic.StorageLogic
}

func NewVersionHandler(
	conf *config.Config,
	logger *zap.Logger,
	resourceLogic *logic.ResourceLogic,
	versionLogic *logic.VersionLogic,
	storageLogic *logic.StorageLogic,
) *VersionHandler {
	return &VersionHandler{
		conf:          conf,
		logger:        logger,
		resourceLogic: resourceLogic,
		versionLogic:  versionLogic,
		storageLogic:  storageLogic,
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

	r.Get("/resources/:rid/latest", h.GetLatest)
	r.Get("/resources/download/:key", h.Download)

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

	url := fmt.Sprintf("%s?token=%s", h.conf.Auth.UploaderValidationURL, token)
	resp, err := http.Post(url, "application/json", nil)
	if err != nil {
		h.logger.Error("Failed to request uploader validation",
			zap.Error(err),
		)
		resp := response.UnexpectedError()
		return c.Status(fiber.StatusInternalServerError).JSON(resp)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
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
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
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

func (h *VersionHandler) isAllowedMimeType(mime string) bool {
	allowedTypes := []string{
		"application/zip",
		"application/x-zip-compressed",
		"application/x-gzip",
		"application/gzip",
	}
	for _, allowedType := range allowedTypes {
		if strings.EqualFold(mime, allowedType) {
			return true
		}
	}
	return false
}

func (h *VersionHandler) Create(c *fiber.Ctx) error {
	resID := c.Params(resourceKey)
	name := c.FormValue("name")
	file, err := c.FormFile("file")
	if err != nil {
		h.logger.Error("Failed to get file from form",
			zap.Error(err),
		)
		resp := response.BusinessError("invalid file")
		return c.Status(fiber.StatusBadRequest).JSON(resp)
	}

	ctx := c.UserContext()

	// if !h.isAllowedMimeType(file.Header.Get("Content-Type")) {
	// 	resp := response.BusinessError("invalid file type")
	// 	return c.Status(fiber.StatusBadRequest).JSON(resp)
	// }

	if !h.isValidExtension(file.Filename) {
		resp := response.BusinessError("invalid file extension")
		return c.Status(fiber.StatusBadRequest).JSON(resp)
	}

	versionNameExistsParam := VersionNameExistsParam{
		ResourceID: resID,
		Name:       name,
	}
	exists, err := h.versionLogic.NameExists(ctx, versionNameExistsParam)
	if err != nil {
		h.logger.Error("Failed to check if version name exists",
			zap.Error(err),
		)
		resp := response.UnexpectedError()
		return c.Status(fiber.StatusInternalServerError).JSON(resp)
	}
	if exists {
		h.logger.Info("Version name already exists",
			zap.String("resource id", resID),
			zap.String("version name", name),
		)
		resp := response.BusinessError("version name already exists")
		return c.Status(fiber.StatusConflict).JSON(resp)
	}

	cwd, err := os.Getwd()
	if err != nil {
		h.logger.Error("Failed to get current directory",
			zap.Error(err),
		)
		resp := response.UnexpectedError()
		return c.Status(fiber.StatusInternalServerError).JSON(resp)
	}
	tempRootDir := filepath.Join(cwd, "temp")
	if err := os.MkdirAll(tempRootDir, os.ModePerm); err != nil {
		h.logger.Error("Failed to create temp root directory",
			zap.Error(err),
		)
		resp := response.UnexpectedError()
		return c.Status(fiber.StatusInternalServerError).JSON(resp)
	}
	tempDir, err := os.MkdirTemp(tempRootDir, "version")
	if err != nil {
		h.logger.Error("Failed to create temp directory", zap.Error(err))
		resp := response.UnexpectedError()
		return c.Status(fiber.StatusInternalServerError).JSON(resp)
	}
	defer func(path string) {
		err := os.RemoveAll(path)
		if err != nil {
			h.logger.Error("Failed to remove temp directory")
		}
	}(tempDir)

	tempPath := fmt.Sprintf("%s/%s", tempDir, file.Filename)
	if err := c.SaveFile(file, tempPath); err != nil {
		h.logger.Error("Failed to save file",
			zap.Error(err),
		)
		resp := response.UnexpectedError()
		return c.Status(fiber.StatusInternalServerError).JSON(resp)
	}

	createVersionParam := CreateVersionParam{
		ResourceID:        resID,
		Name:              name,
		UploadArchivePath: tempPath,
	}
	version, saveDir, err := h.versionLogic.Create(ctx, createVersionParam)
	if err != nil {
		h.logger.Error("Failed to create version",
			zap.Error(err),
		)
		resp := response.UnexpectedError()
		return c.Status(fiber.StatusInternalServerError).JSON(resp)
	}

	createStorageParam := CreateStorageParam{
		VersionID: version.ID,
		Directory: saveDir,
	}
	_, err = h.storageLogic.Create(ctx, createStorageParam)
	if err != nil {
		h.logger.Error("Failed to create storage",
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
	resp := response.Success(data)
	return c.Status(fiber.StatusCreated).JSON(resp)
}

func (h *VersionHandler) ValidateCDK(cdk, spId, ua, source string) (bool, error) {
	h.logger.Debug("Validating CDK")
	if cdk == "" {
		h.logger.Error("Missing cdk param")
		return false, CdkNotfound
	}
	if spId == "" {
		h.logger.Error("Missing spId param")
		return false, SpIdNotfound
	}

	request := ValidateCDKRequest{
		CDK:             cdk,
		SpecificationID: spId,
		Source:          source,
		UA:              ua,
	}

	jsonData, err := json.Marshal(request)
	if err != nil {
		h.logger.Error("Failed to marshal JSON",
			zap.Error(err),
		)
		return false, err
	}

	resp, err := http.Post(h.conf.Auth.CDKValidationURL, fiber.MIMEApplicationJSON, bytes.NewBuffer(jsonData))
	if err != nil {
		h.logger.Error("Failed to send request",
			zap.Error(err),
		)
		return false, err
	}

	var result ValidateCDKResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
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

func (h *VersionHandler) GetLatest(c *fiber.Ctx) error {
	resID := c.Params(resourceKey)

	req := &GetLatestVersionRequest{}
	if err := c.QueryParser(req); err != nil {
		h.logger.Error("Failed to parse query",
			zap.Error(err),
		)
		resp := response.BusinessError("invalid param")
		return c.Status(fiber.StatusBadRequest).JSON(resp)
	}

	var ctx = c.UserContext()

	latest, err := h.versionLogic.GetLatest(ctx, resID)

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

	if isFirstBind, err := h.ValidateCDK(req.CDK, req.SpID, req.UserAgent, resID); err != nil {
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
		go func() {
			request := BillingCheckinRequest{
				CDK:         req.CDK,
				Application: resID,
				UserAgent:   req.UserAgent,
			}
			body, err := json.Marshal(request)
			if err != nil {
				h.logger.Warn("Checkin callback Failed to marshal JSON")
				return
			}
			_, err = http.Post(h.conf.Billing.CheckinURL, fiber.MIMEApplicationJSON, bytes.NewBuffer(body))
			if err != nil {
				h.logger.Warn("Failed to send billing checkin request", zap.Error(err))
			}
		}()
	}

	if latest.Name == req.CurrentVersion {
		resp := response.Success(data, "current version is latest")
		return c.Status(fiber.StatusOK).JSON(resp)
	}

	h.logger.Info("CDK validation success")

	var isFull = req.CurrentVersion == ""

	// if current version is not provided, we will download the full version
	var current *ent.Version
	if !isFull {
		getVersionByNameParam := GetVersionByNameParam{
			ResourceID: resID,
			Name:       req.CurrentVersion,
		}
		current, err = h.versionLogic.GetByName(ctx, getVersionByNameParam)
		if err != nil {
			if !ent.IsNotFound(err) {
				h.logger.Error("Failed to get current version",
					zap.Error(err),
				)
				resp := response.UnexpectedError()
				return c.Status(fiber.StatusInternalServerError).JSON(resp)
			}
			isFull = true
		}
	}

	var info = TempDownloadInfo{
		ResourceID:      resID,
		Full:            isFull,
		TargetVersionID: latest.ID,
	}

	if !isFull {
		info.TargetVersionFileHashes = latest.FileHashes
		info.CurrentVersionID = current.ID
		info.CurrentVersionFileHashes = current.FileHashes
	}

	rk := ksuid.New().String()

	if buf, err := json.Marshal(info); err != nil {
		h.logger.Error("Failed to marshal JSON",
			zap.Error(err),
		)
		return c.Status(fiber.StatusInternalServerError).JSON(response.UnexpectedError())
	} else {
		db.IRS.Set(ctx, fmt.Sprintf("RES:%v", rk), string(buf), 20*time.Minute)

		url := strings.Join([]string{h.conf.Extra.DownloadPrefix, rk}, "/")
		data.Url = url
		return c.Status(fiber.StatusOK).JSON(response.Success(data, "success"))
	}

}

func (h *VersionHandler) Download(c *fiber.Ctx) error {
	key := c.Params("key", "")

	if key == "" {
		return c.Status(fiber.StatusNotFound).JSON(response.BusinessError("missing key"))
	}

	ctx := c.UserContext()

	val, err := db.IRS.GetDel(ctx, fmt.Sprintf("RES:%v", key)).Result()

	var info TempDownloadInfo
	if err != nil || val == "" || json.Unmarshal([]byte(val), &info) != nil {
		h.logger.Warn("invalid key or resource not found",
			zap.String("key", key),
		)
		resp := response.BusinessError("invalid key or resource not found")
		return c.Status(fiber.StatusNotFound).JSON(resp)
	}

	// full update
	getResourcePathParam := GetResourcePathParam{
		ResourceID: info.ResourceID,
		VersionID:  info.TargetVersionID,
	}
	resArchivePath := h.versionLogic.GetResourcePath(getResourcePathParam)
	if info.Full {
		c.Set("X-Update-Type", "full")
		return c.Status(fiber.StatusOK).Download(resArchivePath)
	}

	// incremental update
	getPatchPathParam := GetVersionPatchParam{
		ResourceID:               info.ResourceID,
		TargetVersionID:          info.TargetVersionID,
		TargetVersionFileHashes:  info.TargetVersionFileHashes,
		CurrentVersionID:         info.CurrentVersionID,
		CurrentVersionFileHashes: info.CurrentVersionFileHashes,
	}
	patchPath, err := h.versionLogic.GetPatchPath(ctx, getPatchPathParam)
	if err != nil {
		h.logger.Error("Failed to get patch",
			zap.String("resource id", info.ResourceID),
			zap.Int("target version id", info.TargetVersionID),
			zap.Int("current version id", info.CurrentVersionID),
			zap.Error(err),
		)
		resp := response.UnexpectedError
		return c.Status(fiber.StatusInternalServerError).JSON(resp())
	}

	c.Set("X-New-Version-Available", "true")
	c.Set("X-Update-Type", "incremental")
	return c.Status(fiber.StatusOK).Download(patchPath, "ota.zip")
}
