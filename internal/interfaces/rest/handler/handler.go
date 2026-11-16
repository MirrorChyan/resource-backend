package handler

import "github.com/gofiber/fiber/v3"

type Handler interface {
	Register(r fiber.Router)
}
