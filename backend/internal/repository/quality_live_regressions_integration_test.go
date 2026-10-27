//go:build integration

package repository

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func qualityLiveRouter(f *qualityBPSFixture) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := admin.NewScheduledTestHandler(service.NewScheduledTestService(f.plans, NewScheduledTestResultRepository(integrationDB)))
	r := gin.New()
	r.GET("/plans/:id/ownership", h.QualityOwnershipPreview)
	r.POST("/plans/:id/restore", h.QualityOwnershipRestore)
	r.DELETE("/plans/:id", h.Delete)
	return r
}
func qualityLiveRequest(r *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	data, _ := json.Marshal(body)
	req := httptest.NewRequest(method, path, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}
func qualityLivePreview(t *testing.T, r *gin.Engine, id int64) service.QualityOwnershipPreview {
	t.Helper()
	w := qualityLiveRequest(r, "GET", fmt.Sprintf("/plans/%d/ownership", id), nil)
	require.Equal(t, 200, w.Code, w.Body.String())
	var p service.QualityOwnershipPreview
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &p))
	return p
}
func qualityLiveFinish(t *testing.T, f *qualityBPSFixture) {
	t.Helper()
	require.NoError(t, f.plans.FinishPelican(context.Background(), f.plan.ID, f.until, time.Now()))
}
func qualityLiveRestore(r *gin.Engine, id int64, fence string) *httptest.ResponseRecorder {
	return qualityLiveRequest(r, "POST", fmt.Sprintf("/plans/%d/restore", id), map[string]any{"confirm": true, "fence": fence})
}

func TestQualityLiveDeleteHTTPRestoresMatchingOwnershipThenDeletes(t *testing.T) {
	f, group, other := newQualityIsolationFixture(t)
	require.Equal(t, "groups_removed", isolationRound(f, "failed", "failed"))
	qualityLiveFinish(t, f)
	r := qualityLiveRouter(f)
	path := fmt.Sprintf("/plans/%d", f.plan.ID)
	require.Equal(t, 409, qualityLiveRequest(r, "DELETE", path, nil).Code, "retain the original isolation deletion guard")
	p := qualityLivePreview(t, r, f.plan.ID)
	require.True(t, p.RestoreReady)
	require.Equal(t, 400, qualityLiveRequest(r, "POST", path+"/restore", map[string]any{"fence": p.Fence}).Code)
	require.Equal(t, 1, f.count(`SELECT count(*) FROM account_groups WHERE account_id=$1`))
	w := qualityLiveRestore(r, f.plan.ID, p.Fence)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.NotContains(t, f.extra(), service.QualityModelIsolationKey)
	var status string
	var schedulable, enabled bool
	var concurrency, priority int
	var allowed string
	require.NoError(t, integrationDB.QueryRow(`SELECT status,schedulable,concurrency FROM accounts WHERE id=$1`, f.account).Scan(&status, &schedulable, &concurrency))
	require.Equal(t, "active", status)
	require.True(t, schedulable)
	require.Equal(t, 20, concurrency)
	require.NoError(t, integrationDB.QueryRow(`SELECT enabled FROM scheduled_test_plans WHERE id=$1`, f.plan.ID).Scan(&enabled))
	require.False(t, enabled)
	require.NoError(t, integrationDB.QueryRow(`SELECT priority,allowed_models::text FROM account_groups WHERE account_id=$1 AND group_id=$2`, f.account, group).Scan(&priority, &allowed))
	require.Equal(t, 7, priority)
	require.JSONEq(t, `["sol","astra"]`, allowed)
	require.Equal(t, 2, f.count(`SELECT count(*) FROM account_groups WHERE account_id=$1`))
	require.Positive(t, f.count(`SELECT count(*) FROM scheduler_outbox WHERE account_id=$1`))
	require.Equal(t, "stale_run", isolationRound(f, "failed", "failed"), "an old worker cannot recreate restored ownership")
	require.Equal(t, 200, qualityLiveRequest(r, "DELETE", path, nil).Code)
	require.Equal(t, 200, qualityLiveRequest(r, "DELETE", path, nil).Code, "deletion retry is idempotent")
	require.Equal(t, 2, f.count(`SELECT count(*) FROM account_groups WHERE account_id=$1 AND group_id IN (`+qualityIsolationID(group)+`,`+qualityIsolationID(other)+`)`))
	var states int
	require.NoError(t, integrationDB.QueryRow(`SELECT count(*) FROM account_quality_states WHERE plan_id=$1`, f.plan.ID).Scan(&states))
	require.Zero(t, states)
}

func TestQualityLiveDeleteHTTPRejectsStaleFenceAndConflictsWithoutWrites(t *testing.T) {
	for _, change := range []string{"rule", "account", "state", "model", "membership", "orphan", "inactive", "running"} {
		t.Run(change, func(t *testing.T) {
			f, group, _ := newQualityIsolationFixture(t)
			isolationRound(f, "failed", "failed")
			qualityLiveFinish(t, f)
			r := qualityLiveRouter(f)
			p := qualityLivePreview(t, r, f.plan.ID)
			require.True(t, p.RestoreReady)
			var err error
			switch change {
			case "rule":
				_, err = integrationDB.Exec(`UPDATE scheduled_test_plans SET updated_at=clock_timestamp() WHERE id=$1`, f.plan.ID)
			case "account":
				f.exec(`UPDATE accounts SET updated_at=clock_timestamp() WHERE id=$1`)
			case "state":
				_, err = integrationDB.Exec(`UPDATE account_quality_states SET state=jsonb_set(state,'{quality_5xx_episode}','123') WHERE plan_id=$1`, f.plan.ID)
			case "model":
				f.exec(`UPDATE accounts SET extra=jsonb_set(extra,'{quality_model_isolation,sol}', '{"reason":"quality_rule:999"}'),updated_at=clock_timestamp() WHERE id=$1`)
			case "membership":
				f.exec(`INSERT INTO account_groups(account_id,group_id,priority,allowed_models) VALUES($1,$2,99,'["external"]')`, group)
			case "orphan":
				f.exec(`UPDATE accounts SET extra=jsonb_set(extra,'{quality_model_isolation,orphan}',jsonb_build_object('reason',$2::text)) WHERE id=$1`, fmt.Sprintf("quality_rule:%d", f.plan.ID))
			case "inactive":
				f.exec(`UPDATE accounts SET status='disabled',updated_at=clock_timestamp() WHERE id=$1`)
			case "running":
				_, err = integrationDB.Exec(`UPDATE scheduled_test_plans SET running_until=NOW()+interval '5 minutes' WHERE id=$1`, f.plan.ID)
			}
			require.NoError(t, err)
			var beforeAccount, beforeState string
			require.NoError(t, integrationDB.QueryRow(`SELECT row_to_json(a)::text FROM accounts a WHERE id=$1`, f.account).Scan(&beforeAccount))
			require.NoError(t, integrationDB.QueryRow(`SELECT state::text FROM account_quality_states WHERE plan_id=$1`, f.plan.ID).Scan(&beforeState))
			w := qualityLiveRestore(r, f.plan.ID, p.Fence)
			require.Equal(t, 409, w.Code, w.Body.String())
			require.Contains(t, w.Body.String(), "QUALITY_OWNERSHIP_PREVIEW_CHANGED")
			fresh := qualityLivePreview(t, r, f.plan.ID)
			if change == "model" || change == "membership" || change == "orphan" || change == "inactive" || change == "running" {
				require.False(t, fresh.RestoreReady)
				require.NotEmpty(t, fresh.RestoreBlockedReason)
				require.Equal(t, 409, qualityLiveRestore(r, f.plan.ID, fresh.Fence).Code)
			}
			var afterAccount, afterState string
			require.NoError(t, integrationDB.QueryRow(`SELECT row_to_json(a)::text FROM accounts a WHERE id=$1`, f.account).Scan(&afterAccount))
			require.NoError(t, integrationDB.QueryRow(`SELECT state::text FROM account_quality_states WHERE plan_id=$1`, f.plan.ID).Scan(&afterState))
			require.JSONEq(t, beforeAccount, afterAccount)
			require.JSONEq(t, beforeState, afterState)
			require.Equal(t, 409, qualityLiveRequest(r, "DELETE", fmt.Sprintf("/plans/%d", f.plan.ID), nil).Code)
		})
	}
}
