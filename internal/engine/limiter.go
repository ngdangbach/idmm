package engine

import (
	"context"
	"io"
	"sync"
	"time"
)

// SpeedLimiter controls total download bandwidth across all concurrent workers.
type SpeedLimiter struct {
	mu         sync.Mutex
	limit      int64 // Bytes per second (0 = unlimited)
	tokens     int64 // Available tokens (bytes)
	lastRefill time.Time
}

// NewSpeedLimiter creates a new rate limiter.
func NewSpeedLimiter(bytesPerSec int64) *SpeedLimiter {
	return &SpeedLimiter{
		limit:      bytesPerSec,
		tokens:     bytesPerSec,
		lastRefill: time.Now(),
	}
}

// SetLimit dynamically updates the bandwidth limit.
func (sl *SpeedLimiter) SetLimit(bytesPerSec int64) {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	sl.limit = bytesPerSec
	if sl.tokens > bytesPerSec {
		sl.tokens = bytesPerSec
	}
}

// Wait allocates up to n bytes from the bucket, sleeping if necessary.
func (sl *SpeedLimiter) Wait(ctx context.Context, n int64) error {
	sl.mu.Lock()
	if sl.limit <= 0 {
		sl.mu.Unlock()
		return nil
	}

	// Refill tokens based on elapsed time
	now := time.Now()
	elapsed := now.Sub(sl.lastRefill).Seconds()
	if elapsed > 0 {
		added := int64(elapsed * float64(sl.limit))
		sl.tokens += added
		if sl.tokens > sl.limit {
			sl.tokens = sl.limit
		}
		sl.lastRefill = now
	}

	// If enough tokens, consume and return
	if sl.tokens >= n {
		sl.tokens -= n
		sl.mu.Unlock()
		return nil
	}

	// Calculate wait time needed for n tokens
	deficit := n - sl.tokens
	sl.tokens = 0
	sl.mu.Unlock()

	sleepDuration := time.Duration(float64(deficit) / float64(sl.limit) * float64(time.Second))
	select {
	case <-time.After(sleepDuration):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// LimitedReader wraps an io.Reader and throttles read throughput.
type LimitedReader struct {
	reader  io.Reader
	limiter *SpeedLimiter
	ctx     context.Context
}

// NewLimitedReader creates a rate-limited reader.
func NewLimitedReader(ctx context.Context, r io.Reader, limiter *SpeedLimiter) io.Reader {
	if limiter == nil {
		return r
	}
	return &LimitedReader{
		reader:  r,
		limiter: limiter,
		ctx:     ctx,
	}
}

func (lr *LimitedReader) Read(p []byte) (int, error) {
	n, err := lr.reader.Read(p)
	if n > 0 && lr.limiter != nil {
		if waitErr := lr.limiter.Wait(lr.ctx, int64(n)); waitErr != nil {
			return n, waitErr
		}
	}
	return n, err
}
