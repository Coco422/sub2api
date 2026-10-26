//go:build unit

package service

import (
	"context"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// Exercise the HTTP failover entry, including persistent account policy, rather
// than just matching a capacity string. This used to persist a 30 minute model
// ban before the pool's same-account retry could run.
func TestPoolCapacityHTTPDoesNotCascadeIntoModelCooldown(t *testing.T) {
	for _, status := range []int{http.StatusTooManyRequests, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			repo := &errorPolicyRepoStub{}
			rateLimits := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
			svc := &OpenAIGatewayService{rateLimitService: rateLimits}
			account := newTempUnschedScopeAccount("model", map[string]any{
				"error_code": float64(status), "keywords": []any{"capacity"}, "duration_minutes": float64(30),
			})
			account.Credentials["pool_mode"] = true
			body := []byte(`{"error":{"type":"invalid_request_error","message":"Selected model is at capacity. Please try a different model."}}`)
			for attempt := 0; attempt < 3; attempt++ {
				failure := svc.failoverOpenAIUpstreamHTTPError(context.Background(), nil, account,
					&http.Response{StatusCode: status, Header: http.Header{"Retry-After": {"2"}}}, body,
					"Selected model is at capacity. Please try a different model.", "gpt-6.1-sol")
				require.NotNil(t, failure)
				require.True(t, failure.RequestScopedTransient)
				require.True(t, failure.RetryableOnSameAccount)
				require.Equal(t, http.StatusServiceUnavailable, failure.ClientStatusCode)
				require.Equal(t, "2", failure.ResponseHeaders.Get("Retry-After"))
			}
			require.Empty(t, repo.modelRateLimitCalls)
			require.Zero(t, repo.tempCalls)
			require.Zero(t, repo.setErrCalls)
			require.False(t, svc.isOpenAIAccountRequestRuntimeBlocked(account, "gpt-6.1-sol", false))
		})
	}
}
