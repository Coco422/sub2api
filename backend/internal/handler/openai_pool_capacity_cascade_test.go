//go:build unit

package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestPoolCapacityExhaustionPreservesClassificationOnWire(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		failure := &service.UpstreamFailoverError{
			StatusCode:             http.StatusTooManyRequests,
			ResponseBody:           []byte(`{"error":{"message":"Selected model is at capacity. Please try a different model."}}`),
			ResponseHeaders:        http.Header{"Retry-After": {"2"}},
			RequestScopedTransient: true, RetryableOnSameAccount: true,
			ClientStatusCode: http.StatusServiceUnavailable,
			ClientMessage:    "Selected model is at capacity. Please try a different model.",
		}
		(&OpenAIGatewayHandler{}).handleFailoverExhausted(c, failure, streaming)
		if streaming {
			require.Contains(t, rec.Body.String(), `"code":"upstream_capacity_exhausted"`)
			require.Contains(t, rec.Body.String(), "event: response.failed")
		} else {
			require.Equal(t, http.StatusServiceUnavailable, rec.Code)
			require.Equal(t, "upstream_capacity_exhausted", gjson.GetBytes(rec.Body.Bytes(), "error.code").String())
		}
		require.Equal(t, "2", rec.Header().Get("Retry-After"))
	}
}
