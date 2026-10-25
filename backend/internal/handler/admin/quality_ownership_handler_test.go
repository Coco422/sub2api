package admin

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type qualityConflictRepo struct{ reusePlanRepo }

func (r *qualityConflictRepo) Update(context.Context, *service.ScheduledTestPlan) (*service.ScheduledTestPlan, error) {
	return nil, service.QualityScopeConflict("restore scope before changing targets")
}
func (r *qualityConflictRepo) Delete(context.Context, int64) error {
	return service.QualityScopeConflict("restore scope before deleting")
}
func TestQualityScopeConflictsUse409ForUpdateAndDelete(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &qualityConflictRepo{reusePlanRepo{plan: &service.ScheduledTestPlan{ID: 7, AccountID: 42, ModelID: "test", CronExpression: "0 * * * *", Enabled: false}}}
	h := NewScheduledTestHandler(service.NewScheduledTestService(repo, nil))
	router := gin.New()
	router.PUT("/plans/:id", h.Update)
	router.DELETE("/plans/:id", h.Delete)
	for _, method := range []string{"PUT", "DELETE"} {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/plans/7", strings.NewReader(`{"enabled":false}`))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			require.Equal(t, 409, w.Code)
			require.Contains(t, w.Body.String(), service.QualityScopeConflictReason)
		})
	}
}

type ownershipHandlerRepo struct {
	reusePlanRepo
	calls int
}

func (r *ownershipHandlerRepo) QualityOwnership(_ context.Context, id int64, request *service.QualityOwnershipRequest) (*service.QualityOwnershipPreview, error) {
	r.calls++
	if request != nil {
		return nil, infraerrors.Conflict("QUALITY_OWNERSHIP_PREVIEW_CHANGED", "preview again")
	}
	return &service.QualityOwnershipPreview{PlanID: id, Entries: []service.QualityOwnershipEntry{}}, nil
}
func TestQualityOwnershipRequiresExplicitConfirmationAndReturnsFencedConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &ownershipHandlerRepo{}
	h := NewScheduledTestHandler(service.NewScheduledTestService(repo, nil))
	router := gin.New()
	router.GET("/plans/:id", h.QualityOwnershipPreview)
	router.POST("/plans/:id", h.QualityOwnershipReconcile)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/plans/7", nil))
	require.Equal(t, 200, w.Code)
	require.Equal(t, 1, repo.calls)
	for _, body := range []string{`{}`, `{"confirm":false,"fence":"` + strings.Repeat("a", 64) + `"}`} {
		req := httptest.NewRequest("POST", "/plans/7", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, 400, w.Code)
		require.Equal(t, 1, repo.calls)
	}
	req := httptest.NewRequest("POST", "/plans/7", strings.NewReader(`{"confirm":true,"fence":"`+strings.Repeat("a", 64)+`"}`))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, 409, w.Code)
	require.Contains(t, w.Body.String(), "QUALITY_OWNERSHIP_PREVIEW_CHANGED")
}
