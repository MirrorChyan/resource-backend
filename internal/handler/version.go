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
	"github.com/MirrorChyan/resource-backend/internal/logic"
	"github.com/MirrorChyan/resource-backend/internal/patcher"
	"github.com/MirrorChyan/resource-backend/internal/pkg/archive"
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
	r.Use("/resources/:resID/versions", h.ValidateUploader)
	r.Post("/resources/:resID/versions", h.Create)
	r.Use("/resources/:resID/versions/latest", h.ValidateCDK)
	r.Get("/resources/:resID/versions/latest", h.GetLatest)
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

	storageRootDir := filepath.Join(cwd, "storage")
	saveDir := filepath.Join(storageRootDir, resIDStr, name, "resource")
	if err := os.MkdirAll(saveDir, os.ModePerm); err != nil {
		h.logger.Error("Failed to create storage directory",
			zap.Error(err),
		)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create storage directory",
		})
	}

	var unpackErr error
	if strings.HasSuffix(file.Filename, ".zip") {
		unpackErr = archive.UnpackZip(tempPath, saveDir)
	} else if strings.HasSuffix(file.Filename, ".tar.gz") {
		unpackErr = archive.UnpackTarGz(tempPath, saveDir)
	}

	if unpackErr != nil {
		h.logger.Error("Failed to unpack file",
			zap.Error(unpackErr),
		)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to unpack file",
		})
	}

	if err := os.Remove(tempPath); err != nil {
		h.logger.Error("Failed to remove temp file",
			zap.Error(err),
		)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to remove temp file",
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

	param := logic.CreateVersionParam{
		ResourceID:  resID,
		Name:        name,
		ResourceDir: saveDir,
	}

	ctx := context.Background()
	version, err := h.versionLogic.Create(ctx, param)
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
		ID:     version.ID,
		Name:   version.Name,
		Number: version.Number,
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
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Missing X-CDK header",
		})
	}
	specificationID := c.Get("X-Specification-ID")
	if specificationID == "" {
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
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "CDK validation failed",
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

	if res.Code != 0 {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "CDK validation failed",
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
		c.Set("X-Latest-Version", latest.Name)
		c.Set("X-New-Version-Available", "false")
		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"message": "No new version available",
		})
	}

	getCurrentVersionParam := logic.GetVersionByNameParam{
		ResourceID: resID,
		Name:       req.CurrentVersion,
	}
	current, err := h.versionLogic.GetByName(ctx, getCurrentVersionParam)
	if err != nil {
		h.logger.Error("Failed to get current version",
			zap.String("version name", req.CurrentVersion),
			zap.Error(err),
		)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to get current version",
		})
	}

	changes, err := patcher.CalculateDiff(latest.FileHashes, current.FileHashes)

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
	patchDir := filepath.Join(storageRootDir, resIDStr, latest.Name, "patch")
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
	return c.Status(fiber.StatusOK).Download(filepath.Join(patchDir, archvieName))
}
