#!/usr/bin/env python3
"""Restore exact baseline function bodies while retaining additive identity types.

This makes regression failures behavioral, never missing-field compile failures.
The immutable baseline is the production SHA from which this branch started.
"""
from pathlib import Path
import subprocess

BASE = '00788a4705152cb90aab089d352a494531e4664b'
def function(source, prefix):
    start = source.index(prefix)
    # gofmt top-level functions end in a column-zero closing brace.
    end = source.index('\n}', start) + 2
    return start, end, source[start:end]

for file, prefix in [
 ('backend/internal/service/priority_error_rate.go', 'func priorityErrorEndpoint('),
 ('backend/internal/service/priority_scheduling_history.go', 'func (s *OpenAIGatewayService) priorityHistory('),
 ('backend/internal/handler/openai_gateway_handler.go', 'func openAIModelMappedBody('),
 ('backend/internal/service/openai_account_scheduler.go', 'func (s *defaultOpenAIAccountScheduler) selectBySessionHash('),
 ('backend/internal/service/openai_account_scheduler.go', 'func (s *defaultOpenAIAccountScheduler) tryFallbackToWeightedSticky('),
]:
    path = Path(file)
    old = subprocess.check_output(['git', 'show', f'{BASE}:{file}'], text=True)
    current = path.read_text()
    start, end, _ = function(current, prefix)
    path.write_text(current[:start] + function(old, prefix)[2] + current[end:])
    print(f'Restored exact old function: {file}: {prefix}')
