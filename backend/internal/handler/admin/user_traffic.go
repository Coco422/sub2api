package admin

import (
	"context"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler/trafficquery"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func (h *UsageHandler) TrafficKeys(c *gin.Context) {
	userID, query, ok := trafficquery.ParseKeys(c, 0, true)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 4*time.Second)
	defer cancel()
	options, err := h.usageService.TrafficKeyOptions(ctx, userID, query)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, options)
}

func (h *UsageHandler) TrafficHealth(c *gin.Context) {
	if h.trafficDelivery == nil {
		response.Error(c, http.StatusServiceUnavailable, "Traffic delivery unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 4*time.Second)
	defer cancel()
	health, err := h.trafficDelivery.Health(ctx)
	if err != nil {
		response.Error(c, http.StatusServiceUnavailable, "Traffic delivery health unavailable")
		return
	}
	response.Success(c, health)
}

func (h *UsageHandler) ConfigureTrafficDelivery(dir string) (func(), error) {
	d, err := service.NewTrafficDelivery(dir, h.usageService.RecordTraffic)
	if err != nil {
		return nil, err
	}
	h.trafficDelivery = d
	return d.Stop, nil
}

func (h *UsageHandler) RecordTraffic(e service.TrafficEvent) {
	if h.trafficDelivery != nil {
		h.trafficDelivery.RecordTraffic(e)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := h.usageService.RecordTraffic(ctx, e); err != nil {
		logger.LegacyPrintf("user_traffic", "persistence failed event=%s: %v", e.EventID, err)
	}
}

func (h *UsageHandler) Traffic(c *gin.Context) {
	f, ok := trafficquery.Parse(c, 0, true)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
	defer cancel()
	report, err := h.usageService.Traffic(ctx, f)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, report)
}
