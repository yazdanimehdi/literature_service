package workflows

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/helixir/literature-review-service/internal/domain"
)

func TestSignal_PauseSignal(t *testing.T) {
	t.Run("user pause with message", func(t *testing.T) {
		signal := PauseSignal{
			Reason:  domain.PauseReasonUser,
			Message: "User requested pause",
		}
		assert.Equal(t, domain.PauseReasonUser, signal.Reason)
		assert.Equal(t, "User requested pause", signal.Message)
	})

	t.Run("budget exhausted pause", func(t *testing.T) {
		signal := PauseSignal{
			Reason:  domain.PauseReasonBudgetExhausted,
			Message: "Monthly budget limit reached",
		}
		assert.Equal(t, domain.PauseReasonBudgetExhausted, signal.Reason)
		assert.Equal(t, "Monthly budget limit reached", signal.Message)
	})

	t.Run("empty message is valid", func(t *testing.T) {
		signal := PauseSignal{
			Reason: domain.PauseReasonUser,
		}
		assert.Equal(t, domain.PauseReasonUser, signal.Reason)
		assert.Empty(t, signal.Message)
	})
}

func TestSignal_ResumeSignal(t *testing.T) {
	t.Run("user resume", func(t *testing.T) {
		signal := ResumeSignal{ResumedBy: "user"}
		assert.Equal(t, "user", signal.ResumedBy)
	})

	t.Run("budget refill resume", func(t *testing.T) {
		signal := ResumeSignal{ResumedBy: "budget_refill"}
		assert.Equal(t, "budget_refill", signal.ResumedBy)
	})

	t.Run("stop request resume", func(t *testing.T) {
		signal := ResumeSignal{ResumedBy: "stop_request"}
		assert.Equal(t, "stop_request", signal.ResumedBy)
	})
}

func TestSignal_StopSignal(t *testing.T) {
	t.Run("user requested stop", func(t *testing.T) {
		signal := StopSignal{Reason: "user_requested"}
		assert.Equal(t, "user_requested", signal.Reason)
	})

	t.Run("timeout stop", func(t *testing.T) {
		signal := StopSignal{Reason: "timeout"}
		assert.Equal(t, "timeout", signal.Reason)
	})

	t.Run("empty reason is valid", func(t *testing.T) {
		signal := StopSignal{}
		assert.Empty(t, signal.Reason)
	})
}
