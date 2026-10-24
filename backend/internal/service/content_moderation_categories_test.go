package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/typesafe"
	"github.com/stretchr/testify/require"
)

// All upstreams in these tests are intercepted: no network, model or database.
func categoryFixtureService(t *testing.T, category string, score float64, enabled bool) (*ContentModerationService, *contentModerationTestSettingRepo, *int) {
	t.Helper()
	cfg := defaultContentModerationConfig()
	cfg.Enabled, cfg.Engine, cfg.RecordNonHits = true, "typesafe", true
	cfg.EmailOnHit, cfg.AutoBanEnabled, cfg.RetryCount = false, false, 0
	cfg.TypeSafe = moderationEngineDefaults("typesafe")
	cfg.TypeSafe.APIKeys, cfg.TypeSafe.RetryCount = []string{"offline-fixture"}, 0
	cfg.TypeSafe.Thresholds[category] = 0.80 // Boundary fixture, not a calibration.
	cfg.TypeSafe.CategorySettings[category] = ContentModerationCategorySetting{Enabled: enabled}
	raw, err := json.Marshal(cfg)
	require.NoError(t, err)
	settings := &contentModerationTestSettingRepo{values: map[string]string{SettingKeyRiskControlEnabled: "true", SettingKeyContentModerationConfig: string(raw)}}
	calls := new(int)
	s := &ContentModerationService{settingRepo: settings, repo: &contentModerationTestRepo{}, asyncQueue: make(chan contentModerationTask, 32)}
	s.httpClient = &http.Client{Transport: typeSafeURLRoundTripper(func(req *http.Request) (*http.Response, error) {
		*calls++
		require.Equal(t, typesafe.SystemOnePath, req.URL.Path)
		var request typesafe.Request
		require.NoError(t, json.NewDecoder(req.Body).Decode(&request))
		require.Len(t, request.Questions, 15)
		require.Equal(t, "noul", request.Questions[category].Type)
		require.Contains(t, request.Questions["control_bypass"].Instructions, "不推断是否违法或未经授权")
		answers := map[string]any{}
		for _, id := range ContentModerationCategories("typesafe") {
			answers[id] = map[string]any{"type": "noul", "noul": 0.01}
		}
		answers[category] = map[string]any{"type": "noul", "noul": score}
		raw, err := json.Marshal(map[string]any{"model": "jev-fixture", "answers": answers})
		require.NoError(t, err)
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
	})}
	return s, settings, calls
}

func TestContentModerationTypeSafeCategoryBoundariesAndTestParity(t *testing.T) {
	for _, category := range []string{"jailbreak", "control_bypass"} {
		for _, score := range []float64{0.79, 0.80, 0.90, 1.0} {
			for _, enabled := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%.2f/enabled=%t", category, score, enabled), func(t *testing.T) {
					s, _, calls := categoryFixtureService(t, category, score, enabled)
					trial, err := s.TestAPIKeys(context.Background(), TestContentModerationAPIKeysInput{Prompt: "synthetic fixture"})
					require.NoError(t, err)
					require.NotNil(t, trial.AuditResult)
					decision, err := s.Check(context.Background(), exemptionInput(false))
					require.NoError(t, err)
					wantHit := enabled && score >= 0.80
					require.Equal(t, wantHit, decision.Blocked)
					require.Equal(t, wantHit, decision.Flagged)
					require.Equal(t, !wantHit, decision.Allowed)
					require.Equal(t, trial.AuditResult.Flagged, decision.Flagged)
					require.Equal(t, trial.AuditResult.HighestCategory, decision.HighestCategory)
					require.Equal(t, category, decision.HighestCategory)
					require.Equal(t, score, decision.HighestScore)
					require.Equal(t, 2, *calls)
					require.Len(t, s.asyncQueue, 1)
					task := <-s.asyncQueue
					require.Equal(t, wantHit, task.log.Flagged)
					require.Equal(t, score, task.log.CategoryScores[category])
					require.Equal(t, 0.80, task.log.ThresholdSnapshot[category])
					require.Equal(t, TypeSafeModerationRulesVersion, task.log.EngineMeta.RulesVersion)
					require.Equal(t, enabled, task.log.EngineMeta.CategorySettings[category].Enabled)
					require.Equal(t, trial.AuditResult.MatchedCategories, task.log.EngineMeta.MatchedCategories)
					if wantHit {
						require.Equal(t, []string{category}, task.log.EngineMeta.MatchedCategories)
					} else {
						require.Empty(t, task.log.EngineMeta.MatchedCategories)
					}
					// Serialization is the existing engine_meta JSON persistence contract.
					raw, err := json.Marshal(task.log)
					require.NoError(t, err)
					var restored ContentModerationLog
					require.NoError(t, json.Unmarshal(raw, &restored))
					require.Equal(t, task.log.EngineMeta, restored.EngineMeta)
					require.Equal(t, task.log.ThresholdSnapshot, restored.ThresholdSnapshot)
					// Queued snapshots cannot change when a later config is edited.
					cfg := task.config
					cfg.Thresholds[category] = 0.99
					cfg.CategorySettings[category] = ContentModerationCategorySetting{}
					require.Equal(t, 0.80, task.log.ThresholdSnapshot[category])
					require.Equal(t, enabled, task.log.EngineMeta.CategorySettings[category].Enabled)
				})
			}
		}
	}
}

func TestContentModerationCategorySettingsSaveReloadAndSwitch(t *testing.T) {
	s, settings, _ := categoryFixtureService(t, "jailbreak", 0.90, false)
	ctx := context.Background()
	thresholds := map[string]float64{"jailbreak": 0.80, "control_bypass": 0.91, "unsupported": 0}
	categories := map[string]ContentModerationCategorySetting{"jailbreak": {Enabled: true}, "control_bypass": {Enabled: false}, "unsupported": {Enabled: true}}
	_, err := s.UpdateConfig(ctx, UpdateContentModerationConfigInput{Thresholds: &thresholds, CategorySettings: &categories})
	require.NoError(t, err)
	for _, engine := range []string{"openai", "typesafe"} {
		// Even flat requests that attempt to leak the new categories must be scoped.
		view, err := s.UpdateConfig(ctx, UpdateContentModerationConfigInput{Engine: &engine})
		require.NoError(t, err)
		reloaded := &ContentModerationService{settingRepo: settings}
		view, err = reloaded.GetConfig(ctx)
		require.NoError(t, err)
		require.Len(t, view.EngineConfigs["openai"].Thresholds, 13)
		require.Empty(t, view.EngineConfigs["openai"].CategorySettings)
		require.Len(t, view.EngineConfigs["typesafe"].Thresholds, 15)
		require.Equal(t, 0.80, view.EngineConfigs["typesafe"].Thresholds["jailbreak"])
		require.True(t, view.EngineConfigs["typesafe"].CategorySettings["jailbreak"].Enabled)
		require.False(t, view.EngineConfigs["typesafe"].CategorySettings["control_bypass"].Enabled)
		require.NotContains(t, view.CategorySettings, "unsupported")
	}
	engine := "openai"
	view, err := s.UpdateConfig(ctx, UpdateContentModerationConfigInput{Engine: &engine, Thresholds: &thresholds, CategorySettings: &categories})
	require.NoError(t, err)
	require.Len(t, view.Thresholds, 13)
	require.Empty(t, view.CategorySettings)
	require.True(t, view.EngineConfigs["typesafe"].CategorySettings["jailbreak"].Enabled)
}

func TestContentModerationOldCategoryConfigAndMissingScores(t *testing.T) {
	for _, raw := range []string{`{}`, `{"engine":"typesafe","typesafe":{"thresholds":{"jailbreak":0,"control_bypass":0}}}`} {
		cfg, err := parseContentModerationConfig(raw)
		require.NoError(t, err)
		cfg = cfg.effectiveEngine("typesafe")
		for _, category := range []string{"jailbreak", "control_bypass"} {
			require.False(t, cfg.CategorySettings[category].Enabled)
			flagged, _, _, matched := evaluateModerationScores(map[string]float64{category: 1}, cfg)
			require.False(t, flagged)
			require.Empty(t, matched)
			cfg.CategorySettings[category] = ContentModerationCategorySetting{Enabled: true}
			cfg.Thresholds[category] = 0
			flagged, _, _, _ = evaluateModerationScores(map[string]float64{}, cfg)
			require.False(t, flagged, "a missing answer must never be fabricated as zero")
		}
	}
	for _, category := range ContentModerationCategories() {
		cfg := defaultContentModerationConfig()
		flagged, _, _, _ := evaluateModerationScores(map[string]float64{category: cfg.Thresholds[category]}, cfg)
		require.True(t, flagged, category)
	}
}

func TestContentModerationTypeSafeInvalidNewAnswersFollowFailurePolicy(t *testing.T) {
	for _, category := range []string{"jailbreak", "control_bypass"} {
		for _, invalid := range []string{"missing", "null", "wrong-type", "string", "negative", "too-large"} {
			t.Run(category+"/"+invalid, func(t *testing.T) {
				s, _, _ := categoryFixtureService(t, category, 0.90, true)
				calls := 0
				s.httpClient = &http.Client{Transport: typeSafeURLRoundTripper(func(req *http.Request) (*http.Response, error) {
					calls++
					answers := map[string]any{}
					for _, id := range ContentModerationCategories("typesafe") {
						answers[id] = map[string]any{"type": "noul", "noul": 0.01}
					}
					switch invalid {
					case "missing":
						delete(answers, category)
					case "null":
						answers[category] = map[string]any{"type": "noul", "noul": nil}
					case "wrong-type":
						answers[category] = map[string]any{"type": "score", "noul": 0.90}
					case "string":
						answers[category] = map[string]any{"type": "noul", "noul": "0.90"}
					case "negative":
						answers[category] = map[string]any{"type": "noul", "noul": -0.1}
					case "too-large":
						answers[category] = map[string]any{"type": "noul", "noul": 1.1}
					}
					raw, err := json.Marshal(map[string]any{"model": "jev-fixture", "answers": answers})
					require.NoError(t, err)
					return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
				})}
				trial, err := s.TestAPIKeys(context.Background(), TestContentModerationAPIKeysInput{Prompt: "fixture"})
				require.NoError(t, err)
				require.Nil(t, trial.AuditResult)
				require.Equal(t, "error", trial.Items[0].Status)
				require.Zero(t, trial.Items[0].SuccessCount)
				decision, err := s.Check(context.Background(), exemptionInput(false))
				require.NoError(t, err)
				require.True(t, decision.Allowed, "existing failure-open policy")
				require.False(t, decision.Flagged)
				require.Len(t, s.asyncQueue, 0)
				repo := s.repo.(*contentModerationTestRepo)
				require.Len(t, repo.logs, 1)
				require.Equal(t, "error", repo.logs[0].Action)
				require.False(t, repo.logs[0].Flagged)
				require.Empty(t, repo.logs[0].CategoryScores)
				require.Equal(t, TypeSafeModerationRulesVersion, repo.logs[0].EngineMeta.RulesVersion)
				require.Equal(t, 2, calls)
			})
		}
	}
}

func TestContentModerationThresholdValidationScopesAndClamps(t *testing.T) {
	s, _, _ := categoryFixtureService(t, "jailbreak", 0.90, true)
	for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		thresholds := map[string]float64{"jailbreak": value}
		_, err := s.UpdateConfig(context.Background(), UpdateContentModerationConfigInput{Thresholds: &thresholds})
		require.ErrorContains(t, err, "有限数值")
		_, err = s.TestAPIKeys(context.Background(), TestContentModerationAPIKeysInput{Thresholds: &thresholds})
		require.Error(t, err)
	}
	thresholds := map[string]float64{"jailbreak": -0.1, "control_bypass": 1.1, "unknown": 0}
	view, err := s.UpdateConfig(context.Background(), UpdateContentModerationConfigInput{Thresholds: &thresholds})
	require.NoError(t, err)
	require.Equal(t, 0.0, view.Thresholds["jailbreak"])
	require.Equal(t, 1.0, view.Thresholds["control_bypass"])
	require.NotContains(t, view.Thresholds, "unknown")
}
