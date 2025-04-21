package handler

import "github.com/gofiber/fiber/v2"

type Handler interface {
	Register(r fiber.Router)
}
