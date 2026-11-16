package handler

import (
	"github.com/gofiber/fiber/v3"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type MetricsHandler struct {
}

func NewMetricsHandler() *MetricsHandler {
	return &MetricsHandler{}
}

func (h *MetricsHandler) Register(r fiber.Router) {
	r.All("/metrics", promhttp.Handler())
}
