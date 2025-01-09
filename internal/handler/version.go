package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/MirrorChyan/resource-backend/internal/config"
	"github.com/MirrorChyan/resource-backend/internal/ent"
	"github.com/MirrorChyan/resource-backend/internal/logic"
	"github.com/MirrorChyan/resource-backend/internal/patcher"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

type VersionHandler struct {
	conf          *config.Config
	logger        *zap.Logger
	resourceLogic *logic.ResourceLogic
	versionLogic  *logic.VersionLogic
	storageLogic  *logic.StorageLogic
}

func NewVersionHandler(conf *config.Config, logger *zap.Logger, versionLogic *logic.VersionLogic, storageLogic *logic.StorageLogic) *VersionHandler {
	return &VersionHandler{
		conf:         conf,
		logger:       logger,
		versionLogic: versionLogic,
		storageLogic: storageLogic,
	}
}

func (h *VersionHandler) Register(r fiber.Router) {
	r.Use("/resources/:resID/versions/latest", h.ValidateCDK)
	r.Get("/resources/:resID/versions/latest", h.GetLatest)
	r.Use("/resources/:resID/versions", h.ValidateUploader)
	r.Post("/resources/:resID/versions", h.Create)
}

type ValidateUploaderResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

func (h *VersionHandler) ValidateUploader(c *fiber.Ctx) error {
	token := c.Get("Authorization")
	if token == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Missing authorization header",
		})
	}

	url := fmt.Sprintf("%s?token=%s", h.conf.Auth.UploaderValidationURL, token)
	resp, err := http.Post(url, "application/json", nil)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Invalid authorization token",
		})
	}

	var res ValidateUploaderResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	if res.Code != 0 {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Invalid authorization token",
		})
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

type CreateVersionResponse struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	Number    uint64    `json:"number"`
	CreatedAt time.Time `json:"created_at"`
}

func (h *VersionHandler) Create(c *fiber.Ctx) error {
	resIDStr := c.Params("resID")
	name := c.FormValue("name")
	file, err := c.FormFile("file")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	if !h.isAllowedMimeType(file.Header.Get("Content-Type")) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid file type",
		})
	}

	if !h.isValidExtension(file.Filename) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid file extension",
		})
	}

	resID, err := strconv.Atoi(resIDStr)
	if err != nil {
		h.logger.Error("Failed to convert resource ID to int",
			zap.Error(err),
		)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid resource ID",
		})
	}

	ctx := context.Background()
	versionNameExistsParam := logic.VersionNameExistsParam{
		ResourceID: resID,
		Name:       name,
	}
	exists, err := h.versionLogic.NameExists(ctx, versionNameExistsParam)
	if err != nil {
		h.logger.Error("Failed to check if version name exists")
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to check if version name exists",
		})
	}
	if exists {
		h.logger.Info("Version name already exists",
			zap.String("resource id", resIDStr),
			zap.String("version name", name),
		)
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{
			"error": "Version name already exists",
		})
	}

	cwd, err := os.Getwd()
	if err != nil {
		h.logger.Error("Failed to get current directory",
			zap.Error(err),
		)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to get current directory",
		})
	}
	tempRootDir := filepath.Join(cwd, "temp")
	if err := os.MkdirAll(tempRootDir, os.ModePerm); err != nil {
		h.logger.Error("Failed to create temp root directory",
			zap.Error(err),
		)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create temp root directory",
		})
	}
	tempDir, err := os.MkdirTemp(tempRootDir, "version")
	if err != nil {
		h.logger.Error("Failed to create temp directory", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create temp directory",
		})
	}
	defer os.RemoveAll(tempDir)

	tempPath := fmt.Sprintf("%s/%s", tempDir, file.Filename)
	if err := c.SaveFile(file, tempPath); err != nil {
		h.logger.Error("Failed to save file",
			zap.Error(err),
		)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to save file",
		})
	}

	createVersionParam := logic.CreateVersionParam{
		ResourceID:        resID,
		Name:              name,
		UploadArchivePath: tempPath,
	}
	version, saveDir, err := h.versionLogic.Create(ctx, createVersionParam)
	if err != nil {
		h.logger.Error("Failed to create version",
			zap.Error(err),
		)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create version",
		})
	}

	createStorageParam := logic.CreateStorageParam{
		VersionID: version.ID,
		Directory: saveDir,
	}
	_, err = h.storageLogic.Create(ctx, createStorageParam)
	if err != nil {
		h.logger.Error("Failed to create storage",
			zap.Error(err),
		)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create storage",
		})
	}

	resp := CreateVersionResponse{
		ID:        version.ID,
		Name:      version.Name,
		Number:    version.Number,
		CreatedAt: version.CreatedAt,
	}

	return c.Status(fiber.StatusCreated).JSON(resp)
}

type ValidateCDKRequest struct {
	CDK             string `json:"cdk"`
	SpecificationID string `json:"specificationId"`
}

type ValidateCDKResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

func (h *VersionHandler) ValidateCDK(c *fiber.Ctx) error {
	h.logger.Debug("Validating CDK")
	cdk := c.Get("X-CDK")
	if cdk == "" {
		h.logger.Error("Missing X-CDK header")
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Missing X-CDK header",
		})
	}
	specificationID := c.Get("X-Specification-ID")
	if specificationID == "" {
		h.logger.Error("Missing X-Specification-ID header")
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Missing X-Specification-ID header",
		})
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
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to marshal JSON",
		})
	}

	resp, err := http.Post(h.conf.Auth.CDKValidationURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		h.logger.Error("Failed to send request",
			zap.Error(err),
		)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to send request",
		})
	}

	if resp.StatusCode != http.StatusOK {
		h.logger.Error("CDK validation request error",
			zap.Int("status_code", resp.StatusCode),
		)
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "Internal Server Error",
		})
	}

	var res ValidateCDKResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		h.logger.Error("Failed to decode response",
			zap.Error(err),
		)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to decode response",
		})
	}

	if res.Code == 1 {
		h.logger.Info("CDK validation failed",
			zap.Int("code", res.Code),
			zap.String("msg", res.Msg),
		)
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": res.Msg,
		})
	} else if res.Code == -1 {
		h.logger.Error("CDK validation failed",
			zap.Int("code", res.Code),
			zap.String("msg", res.Msg),
		)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Internal Server Error",
		})
	}

	return c.Next()
}

type GetLatestVersionRequest struct {
	CurrentVersion string `query:"current_version"`
}

func (h *VersionHandler) GetLatest(c *fiber.Ctx) error {
	resIDStr := c.Params("resID")
	req := &GetLatestVersionRequest{}
	if err := c.QueryParser(req); err != nil {
		h.logger.Error("Failed to parse query",
			zap.Error(err),
		)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Failed to parse query",
		})
	}

	resID, err := strconv.Atoi(resIDStr)
	if err != nil {
		h.logger.Error("Failed to convert resource ID to int",
			zap.Error(err),
		)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid resource ID",
		})
	}

	ctx := context.Background()
	latest, err := h.versionLogic.GetLatest(ctx, resID)
	if err != nil {
		h.logger.Error("Failed to get latest version",
			zap.Error(err),
		)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to get latest version",
		})
	}

	if latest.Name == req.CurrentVersion {
		c.Set("X-New-Version-Available", "false")
		c.Set("X-Latest-Version", latest.Name)
		c.Set("X-Update-Type", "none")
		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"message": "No new version available",
		})
	}

	cwd, err := os.Getwd()
	if err != nil {
		h.logger.Error("Failed to get current directory",
			zap.Error(err),
		)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to get current directory",
		})
	}
	storageRootDir := filepath.Join(cwd, "storage")
	versionDir := filepath.Join(storageRootDir, resIDStr, strconv.Itoa(latest.ID))

	resArchivePath := filepath.Join(versionDir, "resource.zip")
	if req.CurrentVersion == "" {
		c.Set("X-New-Version-Available", "true")
		c.Set("X-Latest-Version", latest.Name)
		c.Set("X-Update-Type", "full")
		return c.Status(fiber.StatusOK).Download(resArchivePath)
	}

	getCurrentVersionParam := logic.GetVersionByNameParam{
		ResourceID: resID,
		Name:       req.CurrentVersion,
	}
	current, err := h.versionLogic.GetByName(ctx, getCurrentVersionParam)
	if ent.IsNotFound(err) {
		c.Set("X-New-Version-Available", "true")
		c.Set("X-Latest-Version", latest.Name)
		c.Set("X-Update-Type", "full")
		return c.Status(fiber.StatusOK).Download(resArchivePath)

	} else if err != nil {
		h.logger.Error("Failed to get current version",
			zap.String("version name", req.CurrentVersion),
			zap.Error(err),
		)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to get current version",
		})
	}

	changes, err := patcher.CalculateDiff(latest.FileHashes, current.FileHashes)

	patchDir := filepath.Join(versionDir, "patch")
	patchName := fmt.Sprintf("%s-%s", current.Name, latest.Name)
	latestStorage, err := h.storageLogic.GetByVersionID(ctx, latest.ID)
	if err != nil {
		h.logger.Error("Failed to get storage",
			zap.Error(err),
		)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to get storage",
		})
	}
	archvieName, err := patcher.Generate(patchName, latestStorage.Directory, patchDir, changes)
	if err != nil {
		h.logger.Error("Failed to generate patch package",
			zap.Error(err),
		)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to generate patch package",
		})
	}

	c.Set("X-New-Version-Available", "true")
	c.Set("X-Latest-Version", latest.Name)
	c.Set("X-Update-Type", "incremental")
	return c.Status(fiber.StatusOK).Download(filepath.Join(patchDir, archvieName))
}
