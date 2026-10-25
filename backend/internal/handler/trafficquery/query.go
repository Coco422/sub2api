package trafficquery

import (
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func Parse(c *gin.Context, userID int64, admin bool) (service.TrafficFilter, bool) {
	f, err := service.NewTrafficFilter(c.DefaultQuery("period", "month"), c.Query("date"), time.Now())
	if err != nil {
		response.BadRequest(c, err.Error())
		return f, false
	}
	f.UserID = userID
	fields := []struct {
		name   string
		target *int64
	}{{"api_key_id", &f.APIKeyID}}
	if admin {
		fields = append(fields, struct {
			name   string
			target *int64
		}{"user_id", &f.UserID})
	}
	for _, field := range fields {
		if raw := c.Query(field.name); raw != "" {
			n, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || n <= 0 {
				response.BadRequest(c, "Invalid "+field.name)
				return f, false
			}
			*field.target = n
		}
	}
	for _, field := range []struct {
		name   string
		target *int
		max    int
	}{{"page", &f.Page, 10000}, {"page_size", &f.PageSize, 100}} {
		if raw := c.Query(field.name); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil || n < 1 || n > field.max {
				response.BadRequest(c, "Invalid "+field.name)
				return f, false
			}
			*field.target = n
		}
	}
	return f, true
}
