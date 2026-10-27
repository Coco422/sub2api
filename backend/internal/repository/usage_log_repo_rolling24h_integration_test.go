//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUsageLogRolling24HoursHalfOpenBoundaries(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	client := tx.Client()
	repo := newUsageLogRepositoryWithSQL(client, tx)
	user := mustCreateUser(t, client, &service.User{Email: "rolling-window@example.test"})
	key := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: "rolling-window-fixture", Name: "window"})
	account := mustCreateAccount(t, client, &service.Account{Name: "rolling-window"})
	end, err := time.Parse(time.RFC3339Nano, "2026-10-01T00:00:00.123+09:00")
	require.NoError(t, err)
	start := end.Add(-24 * time.Hour)
	require.Equal(t, 24*time.Hour, end.Sub(start))
	accountStatsCost, multiplier := 5.0, 2.0
	// PostgreSQL timestamps have microsecond precision. Only start and end-1us belong.
	for _, instant := range []time.Time{start.Add(-time.Microsecond), start, end.Add(-time.Microsecond), end, end.Add(time.Microsecond)} {
		_, err := repo.Create(ctx, &service.UsageLog{
			UserID: user.ID, APIKeyID: key.ID, AccountID: account.ID,
			Model: "window-fixture", InputTokens: 1, OutputTokens: 2,
			TotalCost: 7, ActualCost: 3, AccountStatsCost: &accountStatsCost, AccountRateMultiplier: &multiplier,
			CreatedAt: instant,
		})
		require.NoError(t, err)
	}
	filters := usagestats.UsageLogFilters{UserID: user.ID, StartTime: &start, EndTime: &end, ExactTotal: true}
	logs, page, err := repo.ListWithFilters(ctx, pagination.PaginationParams{Page: 1, PageSize: 10, SortBy: "created_at", SortOrder: "asc"}, filters)
	require.NoError(t, err)
	require.EqualValues(t, 2, page.Total)
	require.Len(t, logs, 2)
	require.True(t, logs[0].CreatedAt.Equal(start), "lower boundary is inclusive")
	require.True(t, logs[1].CreatedAt.Equal(end.Add(-time.Microsecond)), "upper boundary is exclusive")
	stats, err := repo.GetStatsWithFilters(ctx, filters)
	require.NoError(t, err)
	require.EqualValues(t, 2, stats.TotalRequests)
	require.EqualValues(t, 6, stats.TotalTokens)
	require.InDelta(t, 14, stats.TotalCost, 1e-9, "standard cost")
	require.InDelta(t, 6, stats.TotalActualCost, 1e-9, "user consumption")
	require.NotNil(t, stats.TotalAccountCost)
	require.InDelta(t, 20, *stats.TotalAccountCost, 1e-9, "account cost")
	for _, granularity := range []string{"hour", "day"} {
		trend, err := repo.GetUsageTrendWithUsageFilters(ctx, start, end, granularity, filters)
		require.NoError(t, err)
		var requests int64
		var cost, actualCost float64
		for _, bucket := range trend {
			requests += bucket.Requests
			cost += bucket.Cost
			actualCost += bucket.ActualCost
		}
		require.EqualValues(t, 2, requests, granularity)
		require.InDelta(t, 14, cost, 1e-9, granularity)
		require.InDelta(t, 6, actualCost, 1e-9, granularity)
	}
}
