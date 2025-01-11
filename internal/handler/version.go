package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/MirrorChyan/resource-backend/internal/config"
	"github.com/MirrorChyan/resource-backend/internal/db"
	"github.com/MirrorChyan/resource-backend/internal/ent"
	"github.com/MirrorChyan/resource-backend/internal/handler/response"
	"github.com/MirrorChyan/resource-backend/internal/logic"
	. "github.com/MirrorChyan/resource-backend/internal/model"
	"github.com/MirrorChyan/resource-backend/internal/patcher"
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
	CTX = context.Background()

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
	resIDStr := c.Params(resourceKey)
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

	if !h.isAllowedMimeType(file.Header.Get("Content-Type")) {
		resp := response.BusinessError("invalid file type")
		return c.Status(fiber.StatusBadRequest).JSON(resp)
	}

	if !h.isValidExtension(file.Filename) {
		resp := response.BusinessError("invalid file extension")
		return c.Status(fiber.StatusBadRequest).JSON(resp)
	}

	resID, err := strconv.Atoi(resIDStr)
	if err != nil {
		h.logger.Error("Failed to convert resource ID to int",
			zap.Error(err),
		)
		resp := response.BusinessError("invalid resource ID")
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
			zap.String("resource id", resIDStr),
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

func (h *VersionHandler) ValidateCDK(cdk, specificationID string) error {
	h.logger.Debug("Validating CDK")
	if cdk == "" {
		h.logger.Error("Missing cdk param")
		return CdkNotfound
	}
	if specificationID == "" {
		h.logger.Error("Missing spId param")
		return SpIdNotfound
	}

	reqData := ValidateCDKRequest{
		CDK:             cdk,
		SpecificationID: specificationID,
	}

	jsonData, err := json.Marshal(reqData)
	if err != nil {
		h.logger.Error("Failed to marshal JSON",
			zap.Error(err),
		)
		//resp := response.UnexpectedError()
		return err
	}

	resp, err := http.Post(h.conf.Auth.CDKValidationURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		h.logger.Error("Failed to send request",
			zap.Error(err),
		)
		return err
	}

	var result ValidateCDKResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		h.logger.Error("Failed to decode response",
			zap.Error(err),
		)
		return err
	}
	var code = result.Code

	switch code {
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
	return nil
}

func (h *VersionHandler) GetLatest(c *fiber.Ctx) error {
	resIDStr := c.Params(resourceKey)

	resourceID, err := strconv.Atoi(resIDStr)
	if err != nil {
		h.logger.Error("Failed to convert resource ID to int",
			zap.Error(err),
		)
		resp := response.BusinessError("invalid resource ID")
		return c.Status(fiber.StatusBadRequest).JSON(resp)
	}

	req := &GetLatestVersionRequest{}
	if err := c.QueryParser(req); err != nil {
		h.logger.Error("Failed to parse query",
			zap.Error(err),
		)
		resp := response.BusinessError("invalid param")
		return c.Status(fiber.StatusBadRequest).JSON(resp)
	}

	var ctx = c.UserContext()

	latest, err := h.versionLogic.GetLatest(ctx, resourceID)

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

	resp := QueryLatestResponseData{
		VersionName:   latest.Name,
		VersionNumber: latest.Number,
	}
	if req.CDK == "" {
		return c.Status(fiber.StatusOK).JSON(response.Success(resp, "current resource latest version is "+latest.Name))
	}

	if err := h.ValidateCDK(req.CDK, req.SpID); err != nil {
		var e RemoteError
		switch {
		case errors.Is(err, CdkNotfound):
			resp := response.BusinessError(err.Error())
			return c.Status(fiber.StatusBadRequest).JSON(resp)
		case errors.Is(err, SpIdNotfound):
			resp := response.BusinessError(err.Error())
			return c.Status(fiber.StatusBadRequest).JSON(resp)
		case errors.As(err, &e):
			resp := response.BusinessError(e.Error())
			return c.Status(fiber.StatusForbidden).JSON(resp)
		default:
			resp := response.UnexpectedError()
			return c.Status(fiber.StatusInternalServerError).JSON(resp)
		}
	}

	if latest.Name == req.CurrentVersion {
		resp := response.Success(resp, "current version is latest")
		return c.Status(fiber.StatusOK).JSON(resp)
	}

	h.logger.Info("CDK validation success")

	rk := ksuid.New().String()

	info := TempDownloadInfo{
		ID:             resourceID,
		Full:           req.CurrentVersion == "",
		VersionID:      latest.ID,
		VersionName:    latest.Name,
		CurrentVersion: req.CurrentVersion,
		FileHashes:     latest.FileHashes,
	}

	if buf, err := json.Marshal(info); err != nil {
		h.logger.Error("Failed to marshal JSON",
			zap.Error(err),
		)
		return c.Status(fiber.StatusInternalServerError).JSON(response.UnexpectedError())
	} else {
		db.IRS.Set(CTX, fmt.Sprintf("RES:%v", rk), string(buf), 20*time.Minute)

		url := strings.Join([]string{h.conf.Extra.DownloadPrefix, rk}, "/")
		resp.Url = url
		return c.Status(fiber.StatusOK).JSON(response.Success(resp, "success"))
	}

}

func (h *VersionHandler) Download(c *fiber.Ctx) error {

	key := c.Params("key", "")

	if key == "" {
		return c.Status(fiber.StatusNotFound).JSON(response.BusinessError("missing key"))
	}

	val, err := db.IRS.GetDel(CTX, fmt.Sprintf("RES:%v", key)).Result()

	var info TempDownloadInfo
	if err != nil || val == "" || json.Unmarshal([]byte(val), &info) != nil {
		h.logger.Error("invalid key or resource not found", zap.String("key", key))
		return c.Status(fiber.StatusNotFound).JSON(response.BusinessError("invalid key or resource not found"))
	}

	var (
		resourceID  = info.ID
		versionID   = info.VersionID
		versionName = info.VersionName

		fileHashes     = info.FileHashes
		currentVersion = info.CurrentVersion
	)

	ctx := c.UserContext()

	cwd, err := os.Getwd()
	if err != nil {
		h.logger.Error("Failed to get current directory",
			zap.Error(err),
		)
		resp := response.UnexpectedError()
		return c.Status(fiber.StatusInternalServerError).JSON(resp)
	}
	dir := filepath.Join(cwd, "storage")
	versionDir := filepath.Join(dir, strconv.Itoa(resourceID), strconv.Itoa(versionID))

	resArchivePath := filepath.Join(versionDir, "resource.zip")
	if info.Full {
		return c.Status(fiber.StatusOK).Download(resArchivePath)
	}

	param := GetVersionByNameParam{
		ResourceID: resourceID,
		Name:       currentVersion,
	}
	current, err := h.versionLogic.GetByName(ctx, param)
	if ent.IsNotFound(err) {
		c.Set("X-Update-Type", "full")
		return c.Status(fiber.StatusOK).Download(resArchivePath)

	} else if err != nil {
		h.logger.Error("Failed to get current version",
			zap.String("version name", currentVersion),
			zap.Error(err),
		)
		resp := response.UnexpectedError()
		return c.Status(fiber.StatusInternalServerError).JSON(resp)
	}

	changes, err := patcher.CalculateDiff(fileHashes, current.FileHashes)
	if err != nil {
		h.logger.Error("Failed to calculate diff",
			zap.Int("resource ID", resourceID),
		)
		resp := response.UnexpectedError()
		return c.Status(fiber.StatusInternalServerError).JSON(resp)
	}

	patchDir := filepath.Join(versionDir, "patch")
	patchName := fmt.Sprintf("%s-%s", current.Name, versionName)
	latestStorage, err := h.storageLogic.GetByVersionID(ctx, versionID)
	if err != nil {
		h.logger.Error("Failed to get storage",
			zap.Error(err),
		)
		resp := response.UnexpectedError()
		return c.Status(fiber.StatusInternalServerError).JSON(resp)
	}
	archiving, err := patcher.Generate(patchName, latestStorage.Directory, patchDir, changes)
	if err != nil {
		h.logger.Error("Failed to generate patch package",
			zap.Error(err),
		)
		resp := response.UnexpectedError()
		return c.Status(fiber.StatusInternalServerError).JSON(resp)
	}

	c.Set("X-New-Version-Available", "true")
	c.Set("X-Update-Type", "incremental")
	return c.Status(fiber.StatusOK).Download(filepath.Join(patchDir, archiving))
}
