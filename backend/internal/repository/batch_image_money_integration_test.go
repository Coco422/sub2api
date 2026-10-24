//go:build integration

package repository

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// Real NUMERIC columns and simultaneous jobs, rather than a float-only mock.
func TestBatchImageMoneyConcurrentJobsConserveBalance(t *testing.T) {
	ctx := context.Background()
	user := mustCreateUser(t, testEntClient(t), &service.User{Email: uuid.NewString() + "@example.com", PasswordHash: "hash", Balance: 100})
	repo := NewUsageBillingRepository(nil, integrationDB)
	amounts := []float64{0.07619951, 0.07621186}
	jobs := make([]string, len(amounts))
	var wg sync.WaitGroup
	errs := make(chan error, len(amounts))
	for i, amount := range amounts {
		jobs[i] = uuid.NewString()
		wg.Add(1)
		go func(id string, hold float64) {
			defer wg.Done()
			_, err := repo.ReserveBatchImageBalance(ctx, &service.BatchImageBalanceHoldCommand{
				RequestID: service.BatchImageHoldRequestID(id), APIKeyID: 17, UserID: user.ID, BatchID: id, HoldAmount: hold,
			})
			errs <- err
		}(jobs[i], amount)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	// Capture one job, refund the other, and replay both operations.
	for attempt := 0; attempt < 2; attempt++ {
		_, err := repo.CaptureBatchImageBalance(ctx, &service.BatchImageBalanceHoldCommand{
			RequestID: service.BatchImageCaptureRequestID(jobs[0]), APIKeyID: 17, UserID: user.ID, BatchID: jobs[0], HoldAmount: amounts[0], ActualAmount: 0.05,
		})
		require.NoError(t, err)
		_, err = repo.ReleaseBatchImageBalance(ctx, &service.BatchImageBalanceHoldCommand{
			RequestID: service.BatchImageReleaseRequestID(jobs[1]), APIKeyID: 17, UserID: user.ID, BatchID: jobs[1], HoldAmount: amounts[1],
		})
		require.NoError(t, err)
	}
	var balance, frozen string
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT balance::text,frozen_balance::text FROM users WHERE id=$1", user.ID).Scan(&balance, &frozen))
	require.Equal(t, "99.95000000", balance)
	require.Equal(t, "0.00000000", frozen)
	// A legacy ambiguous hold must not consume money belonging to another job.
	_, err := repo.ReleaseBatchImageBalance(ctx, &service.BatchImageBalanceHoldCommand{
		RequestID: fmt.Sprintf("legacy:%s", jobs[0]), APIKeyID: 17, UserID: user.ID, BatchID: jobs[0], HoldAmount: 0.0761995125,
	})
	require.ErrorIs(t, err, service.ErrBatchImageLegacyMoneyPrecision)
}
