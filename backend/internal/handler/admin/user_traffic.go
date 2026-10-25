package admin

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler/trafficquery"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

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
