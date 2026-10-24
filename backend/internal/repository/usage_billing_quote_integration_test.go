//go:build integration

package repository

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestUsageBillingQuoteKeepsRawQuoteAndExactMoneyDelta(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	user := mustCreateUser(t, client, &service.User{Email: uuid.NewString() + "@example.com", PasswordHash: "hash", Balance: 1})
	key := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: "sk-mock-" + uuid.NewString(), Name: "quote", Quota: 1})
	account := mustCreateAccount(t, client, &service.Account{Name: "quote-" + uuid.NewString(), Type: service.AccountTypeAPIKey})
	raw := 0.000078125
	cmd := &service.UsageBillingCommand{RequestID: uuid.NewString(), UserID: user.ID, APIKeyID: key.ID, AccountID: account.ID,
		BalanceCost: raw, APIKeyQuotaCost: raw, QuotedActualCost: &raw}
	repo := NewUsageBillingRepository(nil, integrationDB)
	for i := 0; i < 2; i++ {
		_, err := repo.Apply(ctx, cmd)
		require.NoError(t, err)
	}
	var balance, quota, quote, charged string
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT balance::text FROM users WHERE id=$1", user.ID).Scan(&balance))
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT quota_used::text FROM api_keys WHERE id=$1", key.ID).Scan(&quota))
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT quoted_actual_cost::text,charged_actual_cost::text FROM usage_billing_quotes WHERE request_id=$1 AND api_key_id=$2", cmd.RequestID, key.ID).Scan(&quote, &charged))
	require.Equal(t, "0.99992187", balance)
	require.Equal(t, "0.00007813", quota)
	require.Equal(t, "0.0000781250", quote)
	require.Equal(t, "0.00007813", charged)
}
