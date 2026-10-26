package service

import (
	"context"
	"strings"
)

// Observation identity is captured before channel routing. It must never drive
// upstream model support, account mapping, pricing, or response ownership.
type priorityObservationContextKey struct{}
type priorityObservationIdentity struct {
	model, endpoint string
	groupID         int64
	hasGroup        bool
}

func WithPrioritySchedulingObservation(ctx context.Context, group *int64, publicModel, endpoint string) context.Context {
	identity := priorityObservationIdentity{model: strings.TrimSpace(publicModel), endpoint: strings.TrimSpace(endpoint)}
	if group != nil {
		identity.groupID, identity.hasGroup = *group, true
	}
	return context.WithValue(ctx, priorityObservationContextKey{}, identity)
}

func priorityHistoryModel(req OpenAIAccountScheduleRequest) string {
	if model := strings.TrimSpace(req.HistoryModel); model != "" {
		return model
	}
	return strings.TrimSpace(req.RequestedModel)
}

func priorityObservationFromContext(ctx context.Context, fallback string) priorityObservationIdentity {
	identity, _ := ctx.Value(priorityObservationContextKey{}).(priorityObservationIdentity)
	if identity.model == "" {
		identity.model = strings.TrimSpace(fallback)
		if public, ok := RequestedPublicModelFromContext(ctx); ok {
			identity.model = strings.TrimSpace(public)
		}
	}
	return identity
}

func priorityRequestAttemptKey(req OpenAIAccountScheduleRequest, accountID int64) priorityAttemptKey {
	key := priorityAttemptKey{accountID: accountID, model: priorityHistoryModel(req), endpoint: priorityErrorEndpoint(req)}
	if req.GroupID != nil {
		key.groupID, key.hasGroup = *req.GroupID, true
	}
	return key
}
