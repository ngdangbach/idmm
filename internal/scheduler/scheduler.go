package scheduler

import (
	"errors"
	"sync"
	"time"
)

// JobStatus represents the state of a scheduled download task.
type JobStatus string

const (
	StatusScheduled JobStatus = "SCHEDULED"
	StatusRunning   JobStatus = "RUNNING"
	StatusCompleted JobStatus = "COMPLETED"
	StatusCancelled JobStatus = "CANCELLED"
)

// Job holds scheduling metadata and the underlying execution timer.
type Job struct {
	ID        string    `json:"id"`
	TaskID    string    `json:"task_id"`
	RunAt     time.Time `json:"run_at"`
	Status    JobStatus `json:"status"`
	timer     *time.Timer
	action    func()
	cancelMu  sync.Mutex
}

// Scheduler manages scheduled download executions.
type Scheduler struct {
	mu   sync.RWMutex
	jobs map[string]*Job
}

// NewScheduler creates a new scheduler instance.
func NewScheduler() *Scheduler {
	return &Scheduler{
		jobs: make(map[string]*Job),
	}
}

// Schedule queues an action to execute at a specific future time.
func (s *Scheduler) Schedule(taskID string, runAt time.Time, action func()) (*Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	delay := runAt.Sub(now)
	if delay < 0 {
		return nil, errors.New("scheduled time must be in the future")
	}

	// Cancel previous job for the same task if exists
	if existing, ok := s.jobs[taskID]; ok {
		if existing.timer != nil {
			existing.timer.Stop()
		}
		existing.Status = StatusCancelled
	}

	job := &Job{
		ID:     taskID,
		TaskID: taskID,
		RunAt:  runAt,
		Status: StatusScheduled,
		action: action,
	}

	job.timer = time.AfterFunc(delay, func() {
		job.cancelMu.Lock()
		if job.Status == StatusCancelled {
			job.cancelMu.Unlock()
			return
		}
		job.Status = StatusRunning
		job.cancelMu.Unlock()

		if job.action != nil {
			job.action()
		}

		s.mu.Lock()
		job.Status = StatusCompleted
		s.mu.Unlock()
	})

	s.jobs[taskID] = job
	return job, nil
}

// Cancel aborts a scheduled job before it executes.
func (s *Scheduler) Cancel(taskID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	job, exists := s.jobs[taskID]
	if !exists {
		return false
	}

	job.cancelMu.Lock()
	defer job.cancelMu.Unlock()

	if job.Status == StatusScheduled {
		if job.timer != nil {
			job.timer.Stop()
		}
		job.Status = StatusCancelled
		return true
	}
	return false
}

// GetJob returns a scheduled job by task ID.
func (s *Scheduler) GetJob(taskID string) *Job {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.jobs[taskID]
}

// List returns all registered jobs.
func (s *Scheduler) List() []*Job {
	s.mu.RLock()
	defer s.mu.RUnlock()

	list := make([]*Job, 0, len(s.jobs))
	for _, j := range s.jobs {
		list = append(list, j)
	}
	return list
}
