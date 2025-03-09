package handler

import (
	"github.com/MirrorChyan/resource-backend/internal/handler/response"
	"github.com/MirrorChyan/resource-backend/internal/pkg/errs"
	"github.com/gofiber/fiber/v2"
)

func Error(c *fiber.Ctx, err error) error {

	var (
		statusCode int
		msg        string
		data       any
	)

	switch e := err.(type) {
	case *errs.Error:
		statusCode = e.StatusCode
		msg = e.Message
		data = e.Data
	default:
		resp := response.UnexpectedError()
		return c.Status(fiber.StatusInternalServerError).JSON(resp)
	}

	resp := response.BusinessError(msg, data)
	return c.Status(statusCode).JSON(resp)
}
