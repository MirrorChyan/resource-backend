package metrics

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func NewMetrics(r fiber.Router) {
	handler := adaptor.HTTPHandler(promhttp.Handler())
	r.All("/metrics", handler)
}
