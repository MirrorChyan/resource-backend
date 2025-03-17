package handler

import (
	"github.com/MirrorChyan/resource-backend/internal/handler/response"
	"github.com/MirrorChyan/resource-backend/internal/pkg/errs"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

func Error(c *fiber.Ctx, err error) error {

	var (
		bizCode  int
		httpCode int
		msg      string
		data     any
	)

	switch e := err.(type) {

	case *fiber.Error:

		return fiber.DefaultErrorHandler(c, e)

	case *errs.Error:

		bizCode = e.BizCode()
		httpCode = e.HTTPCode()
		msg = e.Message()
		data = e.Details()

	default:

		zap.L().Error("Unexpected error",
			zap.Error(err),
		)
		resp := response.UnexpectedError()
		return c.Status(fiber.StatusInternalServerError).JSON(resp)
	}

	resp := response.BusinessError(msg, data).With(bizCode)
	return c.Status(httpCode).JSON(resp)
}
