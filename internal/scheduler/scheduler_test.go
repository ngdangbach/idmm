package scheduler

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestSchedulerExecution(t *testing.T) {
	s := NewScheduler()
	var executed int32

	targetTime := time.Now().Add(60 * time.Millisecond)
	job, err := s.Schedule("task-1", targetTime, func() {
		atomic.StoreInt32(&executed, 1)
	})

	if err != nil {
		t.Fatalf("Schedule failed: %v", err)
	}

	if job.Status != StatusScheduled {
		t.Errorf("expected StatusScheduled, got %s", job.Status)
	}

	// Should not have executed immediately
	if atomic.LoadInt32(&executed) != 0 {
		t.Error("job executed prematurely")
	}

	// Wait for execution
	time.Sleep(120 * time.Millisecond)

	if atomic.LoadInt32(&executed) != 1 {
		t.Error("job was not executed after scheduled duration")
	}

	if job.Status != StatusCompleted {
		t.Errorf("expected StatusCompleted, got %s", job.Status)
	}
}

func TestSchedulerCancellation(t *testing.T) {
	s := NewScheduler()
	var executed int32

	targetTime := time.Now().Add(80 * time.Millisecond)
	_, err := s.Schedule("task-2", targetTime, func() {
		atomic.StoreInt32(&executed, 1)
	})
	if err != nil {
		t.Fatalf("Schedule failed: %v", err)
	}

	// Cancel before execution
	cancelled := s.Cancel("task-2")
	if !cancelled {
		t.Error("expected Cancel to return true")
	}

	time.Sleep(120 * time.Millisecond)

	if atomic.LoadInt32(&executed) != 0 {
		t.Error("cancelled job should not have executed")
	}
}
