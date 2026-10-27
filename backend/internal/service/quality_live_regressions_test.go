package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

// Exercise the production CreatePlan -> account policy -> discovery seam, not
// only catalog matching. Fixtures never use real credentials or paid inference.
func TestQualityLiveManualModelMissingFromManifest(t *testing.T) {
	for _, mapping := range []map[string]any{nil, {"gpt-6.1-sol": "upstream-sol"}, {"gpt-*": "*"}} {
		t.Run(fmt.Sprint(mapping), func(t *testing.T) {
			accounts := &qualitySupportAccounts{account: &Account{ID: 42, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"model_mapping": mapping}}}
			plans := newTemplatePlanRepo()
			svc := NewScheduledTestService(plans, &pelicanResults{})
			svc.accountTests = &AccountTestService{accountRepo: accounts}
			svc.qualityModels = func(context.Context, int64) ([]string, error) {
				t.Fatal("manual native test must not require manifest availability")
				return nil, nil
			}
			plan := multiModelQualityPlan(t, `["gpt-6.1-sol"]`)
			_, err := svc.CreatePlan(context.Background(), plan)
			require.NoError(t, err)
			require.Len(t, plans.plans, 1)
		})
	}
}

func TestQualityLiveUnknownProbeCatalogIsNotUnsupported(t *testing.T) {
	accounts := &qualitySupportAccounts{account: &Account{ID: 42, Platform: PlatformOpenAI, Type: AccountTypeOAuth}}
	svc := NewScheduledTestService(nil, nil)
	svc.accountTests = &AccountTestService{accountRepo: accounts}
	svc.qualityModels = func(context.Context, int64) ([]string, error) { return []string{"gpt-6-sol"}, nil }
	plan := multiModelQualityPlan(t, `["gpt-6.1-sol"]`)
	plan.PelicanConfig.QuestionKind = "state_probe"
	_, _, err := svc.supportedQualityModels(context.Background(), plan)
	require.Equal(t, "QUALITY_MODEL_CATALOG_UNKNOWN", infraerrors.Reason(err))
	require.NotContains(t, err.Error(), "不支持检测模型")
}

func TestQualityLiveManualSelectionKeepsPolicyAndObservedRejection(t *testing.T) {
	for _, kind := range []string{"allowlist", "observed", "alias-target"} {
		t.Run(kind, func(t *testing.T) {
			account := &Account{ID: 42, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{}, Credentials: map[string]any{}}
			switch kind {
			case "allowlist":
				account.Credentials["model_mapping"] = map[string]any{"other": "other"}
			case "observed":
				account.Extra[qualityUnsupportedModelKey("gpt-6.1-sol")] = "gpt-6.1-sol"
			case "alias-target":
				account.Credentials["model_mapping"] = map[string]any{"gpt-6.1-sol": "target"}
				account.Extra[qualityUnsupportedModelKey("target")] = "target"
			}
			svc := NewScheduledTestService(nil, nil)
			svc.accountTests = &AccountTestService{accountRepo: &qualitySupportAccounts{account: account}}
			svc.qualityModels = func(context.Context, int64) ([]string, error) { return []string{"gpt-6.1-sol"}, nil }
			_, unsupported, err := svc.supportedQualityModels(context.Background(), multiModelQualityPlan(t, `["gpt-6.1-sol"]`))
			require.NoError(t, err)
			require.Equal(t, []string{"gpt-6.1-sol"}, unsupported)
		})
	}
}

func TestQualityLiveModelListCredentialErrorIsStructured(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"code":"token_revoked","message":"synthetic-private-provider-text"}}`))
	}))
	defer server.Close()
	original := chatgptCodexModelsURL
	chatgptCodexModelsURL = server.URL
	t.Cleanup(func() { chatgptCodexModelsURL = original })
	svc := &OpenAIGatewayService{}
	tests := &AccountTestService{openaiGatewayService: svc}
	_, err := tests.FetchOpenAIAccountModels(context.Background(), newCodexModelsTestAccount())
	require.Error(t, err)
	status, body := infraerrors.ToHTTP(err)
	require.Equal(t, http.StatusBadGateway, status, "upstream 401 must not expire admin login")
	require.Equal(t, "OPENAI_CODEX_MODELS_CREDENTIALS_INVALID", body.Reason)
	require.Equal(t, "401", body.Metadata["upstream_status"])
	require.Equal(t, "revoked", body.Metadata["credential_state"])
	require.NotContains(t, body.Message, "synthetic-private-provider-text")
	require.NotContains(t, err.Error(), "synthetic-private-provider-text")
	require.False(t, qualityModelUnsupported(err.Error()))
	require.True(t, IsRetryableCodexModelsManifestError(err))
}
