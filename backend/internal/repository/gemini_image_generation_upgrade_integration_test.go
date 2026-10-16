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
			g := mustCreateGroup(t, client, &service.Group{
				Name:     fmt.Sprintf("image-upgrade-%s-%t-%d", platform, enabled, time.Now().UnixNano()),
				Platform: platform, AllowImageGeneration: enabled,
			})
			t.Cleanup(func() { _ = client.Group.DeleteOneID(g.ID).Exec(ctx) })
			require.NoError(t, ApplyMigrations(ctx, integrationDB))
			require.NoError(t, ensureSimpleModeDefaultGroups(ctx, client))
			actual, err := client.Group.Get(ctx, g.ID)
			require.NoError(t, err)
			require.Equal(t, enabled, actual.AllowImageGeneration, platform)
		}
	}
}
