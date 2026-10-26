//go:build unit

package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Use the actual Forward transport observer and dashboard reader, with a CC
// capability for a Responses request. No paid upstream or projected counters.
func TestPriorityObservationResponsesEndpoint(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5","input":"mock","stream":false}`)
	upstream := &httpUpstreamRecorder{resp: priorityMockResponse(200, `{"id":"resp_mock","object":"response","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1}}`, false)}
	svc := newOpenAIRejectedFieldTestService(upstream)
	account := newOpenAIRejectedFieldTestAccount()
	ctx := WithPrioritySchedulingObservation(context.Background(), nil, "gpt-5.5", "/v1/responses")
	_, err := svc.Forward(ctx, newOpenAIRejectedFieldTestContext(body), account, body)
	require.NoError(t, err)
	req := OpenAIAccountScheduleRequest{RequestedModel: "gpt-5.5", HistoryModel: "gpt-5.5", InboundEndpoint: "/v1/responses", RequiredCapability: OpenAIEndpointCapabilityChatCompletions}
	scheduler := &defaultOpenAIAccountScheduler{service: svc, stats: svc.openaiAccountStats}
	evidence := scheduler.priorityErrorEvidence(req, DefaultPrioritySchedulingConfig(), account.ID, time.Now())
	require.Equal(t, int64(1), evidence.Successes, "Responses capability fallback must read actual Responses ingress")
	require.Equal(t, "/v1/responses", evidence.Endpoint)
	svc.priorityScheduling.latestInput = newPrioritySnapshotInput(req, DefaultPrioritySchedulingConfig(), []openAIAccountCandidateScore{{account: account}}, time.Now())
	snapshot := svc.PrioritySchedulingSnapshot()
	require.Equal(t, int64(1), snapshot.Candidates[0].Successes)
}

type observationQueryReader struct {
	UsageLogRepository
	mu      sync.Mutex
	queries []PrioritySchedulingQuery
}

func (r *observationQueryReader) ReadPrioritySchedulingSignals(_ context.Context, q PrioritySchedulingQuery) (map[int64]PrioritySchedulingSignal, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.queries = append(r.queries, q)
	return map[int64]PrioritySchedulingSignal{1: {Samples: 6, P90TTFTMs: 1234}}, nil
}

func TestPriorityObservationPublicHistoryModel(t *testing.T) {
	c := DefaultPrioritySchedulingConfig()
	reader := &observationQueryReader{}
	svc := &OpenAIGatewayService{usageLogRepo: reader}
	group := int64(11)
	req := OpenAIAccountScheduleRequest{RequestedModel: "route-model", HistoryModel: "public-alias", GroupID: &group}
	pool := []openAIAccountCandidateScore{priorityCandidate(1, .1, 0)}
	require.Eventually(t, func() bool { return svc.priorityHistory(req, c, pool).ready() }, time.Second, time.Millisecond)
	reader.mu.Lock()
	require.Len(t, reader.queries, 1)
	require.Equal(t, "public-alias", reader.queries[0].Model)
	require.Equal(t, "route-model", reader.queries[0].QualityModel)
	require.Equal(t, group, *reader.queries[0].GroupID)
	reader.mu.Unlock()
	other := req
	other.HistoryModel = "distinct-public-alias"
	require.Empty(t, svc.priorityHistory(other, c, pool).signals)
	other = req
	other.GroupID = nil
	require.Empty(t, svc.priorityHistory(other, c, pool).signals)
}

func TestPriorityObservationAttemptGroupAndAliasIsolation(t *testing.T) {
	stats := newOpenAIAccountRuntimeStats()
	svc := &OpenAIGatewayService{openaiAccountStats: stats}
	group := int64(11)
	ctx := WithPrioritySchedulingObservation(context.Background(), &group, " public-alias ", "/v1/chat/completions")
	ctx, scope := withPriorityAttemptScope(ctx, " padded-route ", "/v1/chat/completions")
	require.Equal(t, "public-alias", scope.model)
	require.Equal(t, int64(11), scope.groupID)
	_ = ctx
	req := OpenAIAccountScheduleRequest{RequestedModel: "padded-route", HistoryModel: "public-alias", InboundEndpoint: "/v1/chat/completions", GroupID: &group}
	stats.attempts.report(priorityRequestAttemptKey(req, 1), true, time.Now())
	scheduler := &defaultOpenAIAccountScheduler{service: svc, stats: stats}
	cfg := DefaultPrioritySchedulingConfig()
	require.Equal(t, int64(1), scheduler.priorityErrorEvidence(req, cfg, 1, time.Now()).Successes)
	other := req
	other.HistoryModel = "other-alias"
	require.Zero(t, scheduler.priorityErrorEvidence(other, cfg, 1, time.Now()).Successes)
	other = req
	other.GroupID = nil
	require.Zero(t, scheduler.priorityErrorEvidence(other, cfg, 1, time.Now()).Successes)
	group = 12
	require.Equal(t, int64(11), scope.groupID, "observer owns group value")
	require.Zero(t, scheduler.priorityErrorEvidence(req, cfg, 1, time.Now()).Successes)
}
