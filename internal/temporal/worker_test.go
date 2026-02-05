package temporal

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultWorkerConfig(t *testing.T) {
	t.Run("sets task queue and defaults", func(t *testing.T) {
		cfg := DefaultWorkerConfig("test-queue")

		assert.Equal(t, "test-queue", cfg.TaskQueue)
		assert.Equal(t, 100, cfg.MaxConcurrentActivityExecutionSize)
		assert.Equal(t, 50, cfg.MaxConcurrentWorkflowTaskExecutionSize)
		assert.Equal(t, 4, cfg.MaxConcurrentActivityTaskPollers)
		assert.Equal(t, 2, cfg.MaxConcurrentWorkflowTaskPollers)
	})
}

func TestWorkerConfig(t *testing.T) {
	t.Run("stores custom values", func(t *testing.T) {
		cfg := WorkerConfig{
			TaskQueue:                              "custom-queue",
			MaxConcurrentActivityExecutionSize:     200,
			MaxConcurrentWorkflowTaskExecutionSize: 100,
			MaxConcurrentActivityTaskPollers:       8,
			MaxConcurrentWorkflowTaskPollers:       4,
		}

		assert.Equal(t, "custom-queue", cfg.TaskQueue)
		assert.Equal(t, 200, cfg.MaxConcurrentActivityExecutionSize)
		assert.Equal(t, 100, cfg.MaxConcurrentWorkflowTaskExecutionSize)
		assert.Equal(t, 8, cfg.MaxConcurrentActivityTaskPollers)
		assert.Equal(t, 4, cfg.MaxConcurrentWorkflowTaskPollers)
	})
}

func TestWorkflowRegistry(t *testing.T) {
	t.Run("creates empty registry", func(t *testing.T) {
		r := NewWorkflowRegistry()
		assert.NotNil(t, r)
		assert.Empty(t, r.workflows)
	})

	t.Run("registers workflows", func(t *testing.T) {
		r := NewWorkflowRegistry()

		// Register a mock workflow function
		workflow1 := func() {}
		workflow2 := func() {}

		r.Register(workflow1)
		r.Register(workflow2)

		assert.Len(t, r.workflows, 2)
	})
}

func TestActivityRegistry(t *testing.T) {
	t.Run("creates empty registry", func(t *testing.T) {
		r := NewActivityRegistry()
		assert.NotNil(t, r)
		assert.Empty(t, r.activities)
	})

	t.Run("registers activities", func(t *testing.T) {
		r := NewActivityRegistry()

		// Register mock activity functions
		activity1 := func() {}
		activity2 := func() {}

		r.Register(activity1)
		r.Register(activity2)

		assert.Len(t, r.activities, 2)
	})
}

func TestNewWorkerManager(t *testing.T) {
	t.Run("errors when task queue is empty", func(t *testing.T) {
		cfg := WorkerConfig{TaskQueue: ""}
		_, err := NewWorkerManager(nil, cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "task queue is required")
	})
}

func TestNewWorker(t *testing.T) {
	t.Run("errors when task queue is empty", func(t *testing.T) {
		cfg := WorkerConfig{TaskQueue: ""}
		_, err := NewWorker(nil, cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "task queue is required")
	})
}

func TestWorkerOptionsFromConfig(t *testing.T) {
	t.Run("zero values get defaults", func(t *testing.T) {
		cfg := WorkerConfig{}
		opts := workerOptionsFromConfig(cfg)

		assert.Equal(t, 100, opts.MaxConcurrentActivityExecutionSize)
		assert.Equal(t, 50, opts.MaxConcurrentWorkflowTaskExecutionSize)
		assert.Equal(t, 4, opts.MaxConcurrentActivityTaskPollers)
		assert.Equal(t, 2, opts.MaxConcurrentWorkflowTaskPollers)
	})

	t.Run("non-zero values are preserved", func(t *testing.T) {
		cfg := WorkerConfig{
			MaxConcurrentActivityExecutionSize:     200,
			MaxConcurrentWorkflowTaskExecutionSize: 75,
			MaxConcurrentActivityTaskPollers:       8,
			MaxConcurrentWorkflowTaskPollers:       4,
		}
		opts := workerOptionsFromConfig(cfg)

		assert.Equal(t, 200, opts.MaxConcurrentActivityExecutionSize)
		assert.Equal(t, 75, opts.MaxConcurrentWorkflowTaskExecutionSize)
		assert.Equal(t, 8, opts.MaxConcurrentActivityTaskPollers)
		assert.Equal(t, 4, opts.MaxConcurrentWorkflowTaskPollers)
	})

	t.Run("partial zero values get defaults selectively", func(t *testing.T) {
		cfg := WorkerConfig{
			MaxConcurrentActivityExecutionSize:     150,
			MaxConcurrentWorkflowTaskExecutionSize: 0,   // should default
			MaxConcurrentActivityTaskPollers:       6,
			MaxConcurrentWorkflowTaskPollers:       0,   // should default
		}
		opts := workerOptionsFromConfig(cfg)

		assert.Equal(t, 150, opts.MaxConcurrentActivityExecutionSize)
		assert.Equal(t, 50, opts.MaxConcurrentWorkflowTaskExecutionSize)
		assert.Equal(t, 6, opts.MaxConcurrentActivityTaskPollers)
		assert.Equal(t, 2, opts.MaxConcurrentWorkflowTaskPollers)
	})
}
