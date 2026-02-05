package temporal

import (
	"context"
	"fmt"

	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
)

// WorkerConfig contains configuration for the Temporal worker.
type WorkerConfig struct {
	// TaskQueue is the name of the task queue to poll.
	TaskQueue string

	// MaxConcurrentActivityExecutionSize is the maximum concurrent activity executions.
	// Default: 100
	MaxConcurrentActivityExecutionSize int

	// MaxConcurrentWorkflowTaskExecutionSize is the maximum concurrent workflow task executions.
	// Default: 50
	MaxConcurrentWorkflowTaskExecutionSize int

	// MaxConcurrentActivityTaskPollers is the number of activity task pollers.
	// Default: 4
	MaxConcurrentActivityTaskPollers int

	// MaxConcurrentWorkflowTaskPollers is the number of workflow task pollers.
	// Default: 2
	MaxConcurrentWorkflowTaskPollers int
}

// DefaultWorkerConfig returns a WorkerConfig with default values.
func DefaultWorkerConfig(taskQueue string) WorkerConfig {
	return WorkerConfig{
		TaskQueue:                              taskQueue,
		MaxConcurrentActivityExecutionSize:     100,
		MaxConcurrentWorkflowTaskExecutionSize: 50,
		MaxConcurrentActivityTaskPollers:       4,
		MaxConcurrentWorkflowTaskPollers:       2,
	}
}

// WorkflowRegistry holds workflow functions to be registered with the worker.
type WorkflowRegistry struct {
	workflows []interface{}
}

// NewWorkflowRegistry creates a new empty workflow registry.
func NewWorkflowRegistry() *WorkflowRegistry {
	return &WorkflowRegistry{
		workflows: make([]interface{}, 0),
	}
}

// Register adds a workflow function to the registry.
func (r *WorkflowRegistry) Register(workflow interface{}) {
	r.workflows = append(r.workflows, workflow)
}

// ActivityRegistry holds activity functions to be registered with the worker.
type ActivityRegistry struct {
	activities []interface{}
}

// NewActivityRegistry creates a new empty activity registry.
func NewActivityRegistry() *ActivityRegistry {
	return &ActivityRegistry{
		activities: make([]interface{}, 0),
	}
}

// Register adds an activity function or struct to the registry.
func (r *ActivityRegistry) Register(activity interface{}) {
	r.activities = append(r.activities, activity)
}

// WorkerManager manages the lifecycle of a Temporal worker.
type WorkerManager struct {
	worker     worker.Worker
	taskQueue  string
	workflows  *WorkflowRegistry
	activities *ActivityRegistry
}

// workerOptionsFromConfig builds worker.Options from WorkerConfig, applying defaults
// for any zero-valued fields.
func workerOptionsFromConfig(config WorkerConfig) worker.Options {
	options := worker.Options{
		MaxConcurrentActivityExecutionSize:     config.MaxConcurrentActivityExecutionSize,
		MaxConcurrentWorkflowTaskExecutionSize: config.MaxConcurrentWorkflowTaskExecutionSize,
		MaxConcurrentActivityTaskPollers:       config.MaxConcurrentActivityTaskPollers,
		MaxConcurrentWorkflowTaskPollers:       config.MaxConcurrentWorkflowTaskPollers,
	}

	if options.MaxConcurrentActivityExecutionSize == 0 {
		options.MaxConcurrentActivityExecutionSize = 100
	}
	if options.MaxConcurrentWorkflowTaskExecutionSize == 0 {
		options.MaxConcurrentWorkflowTaskExecutionSize = 50
	}
	if options.MaxConcurrentActivityTaskPollers == 0 {
		options.MaxConcurrentActivityTaskPollers = 4
	}
	if options.MaxConcurrentWorkflowTaskPollers == 0 {
		options.MaxConcurrentWorkflowTaskPollers = 2
	}

	return options
}

// NewWorkerManager creates a new WorkerManager with the given configuration.
func NewWorkerManager(c client.Client, config WorkerConfig) (*WorkerManager, error) {
	if config.TaskQueue == "" {
		return nil, fmt.Errorf("task queue is required")
	}

	options := workerOptionsFromConfig(config)
	w := worker.New(c, config.TaskQueue, options)

	return &WorkerManager{
		worker:     w,
		taskQueue:  config.TaskQueue,
		workflows:  NewWorkflowRegistry(),
		activities: NewActivityRegistry(),
	}, nil
}

// RegisterWorkflow adds a workflow function to be registered when the worker starts.
func (m *WorkerManager) RegisterWorkflow(workflow interface{}) {
	m.workflows.Register(workflow)
	m.worker.RegisterWorkflow(workflow)
}

// RegisterActivity adds an activity function or struct to be registered when the worker starts.
func (m *WorkerManager) RegisterActivity(activity interface{}) {
	m.activities.Register(activity)
	m.worker.RegisterActivity(activity)
}

// Worker returns the underlying Temporal worker.
func (m *WorkerManager) Worker() worker.Worker {
	return m.worker
}

// TaskQueue returns the configured task queue name.
func (m *WorkerManager) TaskQueue() string {
	return m.taskQueue
}

// Start starts the worker and blocks until the context is cancelled.
func (m *WorkerManager) Start(ctx context.Context) error {
	return StartWorker(ctx, m.worker)
}

// Stop stops the worker gracefully.
func (m *WorkerManager) Stop() {
	m.worker.Stop()
}

// NewWorker creates a new Temporal worker with the given configuration.
// This is a convenience function that creates a worker without the manager wrapper.
func NewWorker(c client.Client, config WorkerConfig) (worker.Worker, error) {
	if config.TaskQueue == "" {
		return nil, fmt.Errorf("task queue is required")
	}

	options := workerOptionsFromConfig(config)
	return worker.New(c, config.TaskQueue, options), nil
}

// StartWorker starts the worker and blocks until the context is cancelled.
func StartWorker(ctx context.Context, w worker.Worker) error {
	// Start the worker in a goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- w.Run(worker.InterruptCh())
	}()

	// Wait for context cancellation or worker error
	select {
	case <-ctx.Done():
		w.Stop()
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}
