package handler

import (
	"github.com/gofiber/fiber/v2"
)

type HeathCheckHandler struct {
}

func NewHeathCheckHandlerHandler() *HeathCheckHandler {
	return &HeathCheckHandler{}
}

func (h *HeathCheckHandler) Register(r fiber.Router) {
	r.Get("/health", func(ctx *fiber.Ctx) error {
		return ctx.SendString("OK")
	})
}
