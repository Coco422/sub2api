//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// Fork migration 134 already initialized these flags. Re-running the actual
// upgrade/startup paths must preserve later operator choices, including false.
func TestGeminiImageGenerationUpgradePreservesOperatorChoices(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	for _, platform := range []string{service.PlatformGemini, service.PlatformAntigravity, service.PlatformComposite} {
		for _, enabled := range []bool{false, true} {
			// The shared group fixture does not persist AllowImageGeneration; use
			// the real Ent builder so both true and false are actually stored.
			g, err := client.Group.Create().
				SetName(fmt.Sprintf("image-upgrade-%s-%t-%d", platform, enabled, time.Now().UnixNano())).
				SetPlatform(platform).
				SetStatus(service.StatusActive).
				SetSubscriptionType(service.SubscriptionTypeStandard).
				SetRateMultiplier(1).
				SetAllowImageGeneration(enabled).
				Save(ctx)
			require.NoError(t, err)
			t.Cleanup(func() { _ = client.Group.DeleteOneID(g.ID).Exec(ctx) })
			require.NoError(t, ApplyMigrations(ctx, integrationDB))
			require.NoError(t, ensureSimpleModeDefaultGroups(ctx, client))
			actual, err := client.Group.Get(ctx, g.ID)
			require.NoError(t, err)
			require.Equal(t, enabled, actual.AllowImageGeneration, platform)
		}
	}
}
