package handler

import (
	"github.com/MirrorChyan/resource-backend/internal/pkg/errs"
	"github.com/MirrorChyan/resource-backend/internal/pkg/restserver/response"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

func Error(c *fiber.Ctx, err error) error {

	switch e := err.(type) {

	case *fiber.Error:
		return fiber.DefaultErrorHandler(c, e)

	case *errs.Error:
		resp := response.BusinessError(
			e.Message(),
			e.Details(),
		).With(e.BizCode())
		return c.Status(e.HTTPCode()).JSON(resp)
	case nil:
		return nil
	default:
		zap.L().Error("unexpected error",
			zap.Error(err),
			zap.String("method", c.Method()),
			zap.String("path", c.Path()),
		)
		resp := response.UnexpectedError()
		return c.Status(fiber.StatusInternalServerError).JSON(resp)
	}
}
