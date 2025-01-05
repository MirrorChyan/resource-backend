package handler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/MirrorChyan/resource-backend/internal/logic"
	"github.com/MirrorChyan/resource-backend/internal/patcher"
	"github.com/MirrorChyan/resource-backend/internal/pkg/archive"
	"github.com/MirrorChyan/resource-backend/internal/pkg/rand"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

type VersionHandler struct {
	logger        *zap.Logger
	resourceLogic *logic.ResourceLogic
	versionLogic  *logic.VersionLogic
	storageLogic  *logic.StorageLogic
}

func NewVersionHandler(logger *zap.Logger, versionLogic *logic.VersionLogic, storageLogic *logic.StorageLogic) *VersionHandler {
	return &VersionHandler{
		logger:       logger,
		versionLogic: versionLogic,
		storageLogic: storageLogic,
	}
}

func (h *VersionHandler) Register(r fiber.Router) {
	r.Post("/resources/:resID/versions", h.Create)
	r.Get("/resources/:resID/versions/latest", h.GetLatest)
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
	resID := c.Params("resID")
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

	tempDirName, err := rand.TempDirName()
	if err != nil {
		h.logger.Error("Failed to generate temp directory name",
			zap.Error(err),
		)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to generate temp directory name",
		})
	}

	tempDir := fmt.Sprintf("./temp/%s", tempDirName)
	if err := os.MkdirAll(tempDir, os.ModePerm); err != nil {
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

	saveDir := fmt.Sprintf("./storage/%s/%s/resource", resID, name)
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

	resIDInt, err := strconv.Atoi(resID)
	if err != nil {
		h.logger.Error("Failed to convert resource ID to int",
			zap.Error(err),
		)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid resource ID",
		})
	}

	param := logic.CreateVersionParam{
		ResourceID:  resIDInt,
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

	resIDInt, err := strconv.Atoi(resIDStr)
	if err != nil {
		h.logger.Error("Failed to convert resource ID to int",
			zap.Error(err),
		)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid resource ID",
		})
	}

	ctx := context.Background()
	latest, err := h.versionLogic.GetLatest(ctx, resIDInt)
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
		ResourceID: resIDInt,
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

	patchDir := fmt.Sprintf("./storage/%s/%s/patch", resIDStr, latest.Name)

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
