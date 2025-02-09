package middleware

import (
	"fmt"
	"io"
	"net/http"

	"github.com/MirrorChyan/resource-backend/internal/config"
	"github.com/MirrorChyan/resource-backend/internal/handler/response"
	"github.com/MirrorChyan/resource-backend/internal/model"
	"github.com/bytedance/sonic"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

const (
	resourceKey = "rid"
)

func NewValidateUploader() fiber.Handler {
	return func(c *fiber.Ctx) error {
		token := c.Get("Authorization")
		if token == "" {
			resp := response.BusinessError("missing Authorization header")
			return c.Status(fiber.StatusUnauthorized).JSON(resp)
		}

		var conf = config.GConfig

		rid := c.Params(resourceKey)

		url := fmt.Sprintf("%s?token=%s&rid=%s", conf.Auth.UploaderValidationURL, token, rid)
		resp, err := http.Post(url, "application/json", nil)
		if err != nil {
			zap.L().Error("Failed to request uploader validation",
				zap.Error(err),
			)
			resp := response.UnexpectedError()
			return c.Status(fiber.StatusInternalServerError).JSON(resp)
		}
		defer func(b io.ReadCloser) {
			err := b.Close()
			if err != nil {
				zap.L().Error("Failed to close response body")
			}
		}(resp.Body)

		if resp.StatusCode != http.StatusOK {
			zap.L().Error("Request uploader validation status code not 200",
				zap.Int("status code", resp.StatusCode),
			)
			resp := response.UnexpectedError()
			return c.Status(fiber.StatusUnauthorized).JSON(resp)
		}

		var res model.ValidateUploaderResponse
		buf, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		if err := sonic.Unmarshal(buf, &res); err != nil {
			zap.L().Error("Failed to decode response body",
				zap.Error(err),
			)
			resp := response.UnexpectedError()
			return c.Status(fiber.StatusInternalServerError).JSON(resp)
		}

		if res.Code == 1 {
			zap.L().Info("Uploader validation failed",
				zap.Int("code", res.Code),
				zap.String("msg", res.Msg),
			)
			resp := response.BusinessError(res.Msg)
			return c.Status(fiber.StatusUnauthorized).JSON(resp)
		} else if res.Code == -1 {
			zap.L().Error("Uploader validation failed",
				zap.Int("code", res.Code),
				zap.String("msg", res.Msg),
			)
			resp := response.UnexpectedError()
			return c.Status(fiber.StatusInternalServerError).JSON(resp)
		}

		return c.Next()
	}
}
