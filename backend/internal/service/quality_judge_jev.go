package service

import "context"

func (s *QualityJudgeService) judgeJev(ctx context.Context, cfg *PelicanTestConfig, answer string) *QualityJudgment {
	return &QualityJudgment{Kind: "jev", Verdict: "unknown", Reason: "judge_jev_not_configured"}
}
