package activities

import (
	"context"

	"github.com/helixir/outbox"
)

// outboxBudgetReporter writes budget usage events to the outbox table.
type outboxBudgetReporter struct {
	pool outbox.Querier
}

// NewOutboxBudgetReporter creates a BudgetUsageReporter backed by the outbox.
func NewOutboxBudgetReporter(pool outbox.Querier) BudgetUsageReporter {
	return &outboxBudgetReporter{pool: pool}
}

func (r *outboxBudgetReporter) ReportUsage(ctx context.Context, params BudgetUsageParams) error {
	return outbox.AddBudgetUsageEvent(ctx, r.pool, outbox.BudgetUsagePayload{
		LeaseID:      params.LeaseID,
		OrgID:        params.OrgID,
		ProjectID:    params.ProjectID,
		Model:        params.Model,
		InputTokens:  int64(params.InputTokens),
		OutputTokens: int64(params.OutputTokens),
		TotalTokens:  int64(params.TotalTokens),
		CostUSD:      params.CostUSD,
	})
}
