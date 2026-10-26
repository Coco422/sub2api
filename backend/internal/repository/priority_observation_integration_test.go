//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestPriorityObservationPersistedPublicAliasAndGroup(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	client := tx.Client()
	repo := newUsageLogRepositoryWithSQL(client, tx)
	user := mustCreateUser(t, client, &service.User{Email: "observation@test.invalid"})
	key := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: "sk-observation-synthetic", Name: "test"})
	account := mustCreateAccount(t, client, &service.Account{Name: "observation", Platform: service.PlatformOpenAI})
	group := mustCreateGroup(t, client, &service.Group{Name: "observation", Platform: service.PlatformOpenAI})
	query := service.PrioritySchedulingQuery{AccountIDs: []int64{account.ID}, Model: "public-composite", QualityModel: "channel-route", GroupID: &group.ID, UsageSince: time.Now().Add(-time.Hour), QualitySince: time.Now().Add(-time.Hour)}
	before, err := repo.ReadPrioritySchedulingSignals(ctx, query)
	require.NoError(t, err)
	require.Zero(t, before[account.ID].Samples)
	ttft := 1234
	upstream := "account-upstream"
	add := func(id, public string, groupID *int64, first *int) {
		t.Helper()
		_, err := repo.Create(ctx, &service.UsageLog{UserID: user.ID, APIKeyID: key.ID, AccountID: account.ID, RequestID: id, Model: "channel-route", UpstreamModel: &upstream, RequestedModel: public, GroupID: groupID, Stream: true, FirstTokenMs: first, InputTokens: 10, ActualCost: 1, TotalCost: 2, CreatedAt: time.Now()})
		require.NoError(t, err)
	}
	add("observation-completed", "public-composite", &group.ID, &ttft)
	add("observation-unknown", "public-composite", &group.ID, nil)
	add("observation-other-model", "other-public", &group.ID, &ttft)
	add("observation-other-group", "public-composite", nil, &ttft)
	// The same public alias and group now sees committed rows in this transaction.
	got, err := repo.ReadPrioritySchedulingSignals(ctx, query)
	require.NoError(t, err)
	require.Equal(t, 1, got[account.ID].Samples)
	require.Equal(t, 1, got[account.ID].LatencyUnknownSamples, "NULL TTFT remains unknown")
	require.Equal(t, 2, got[account.ID].ProfitSamples)
	require.InDelta(t, 1234, got[account.ID].P90TTFTMs, .01)
	query.Model = "other-public"
	other, err := repo.ReadPrioritySchedulingSignals(ctx, query)
	require.NoError(t, err)
	require.Equal(t, 1, other[account.ID].Samples)
	require.Zero(t, other[account.ID].LatencyUnknownSamples)
	query.Model = "public-composite"
	query.GroupID = nil
	other, err = repo.ReadPrioritySchedulingSignals(ctx, query)
	require.NoError(t, err)
	require.Equal(t, 1, other[account.ID].Samples)
	query.Model = "channel-route"
	other, err = repo.ReadPrioritySchedulingSignals(ctx, query)
	require.NoError(t, err)
	require.Zero(t, other[account.ID].Samples, "routing identity cannot borrow a different public model")
}
