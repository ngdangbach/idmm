package engine

import "sync"

// SegmentManager handles dividing file ranges and dynamic re-splitting.
type SegmentManager struct {
	mu             sync.Mutex
	segments       []*Segment
	nextID         int
	minSegmentSize int64
}

// NewSegmentManager creates a new segment manager.
func NewSegmentManager(minSegmentSize int64) *SegmentManager {
	if minSegmentSize <= 0 {
		minSegmentSize = 512 * 1024 // 512 KB default
	}
	return &SegmentManager{
		minSegmentSize: minSegmentSize,
	}
}

// InitializeSegments computes initial segments for a known file size.
func (sm *SegmentManager) InitializeSegments(totalSize int64, desiredConnections int) []*Segment {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.segments = make([]*Segment, 0, desiredConnections)
	sm.nextID = 1

	if totalSize <= 0 {
		seg := &Segment{
			ID:         sm.nextID,
			Start:      0,
			End:        -1, // Unknown end
			Downloaded: 0,
			Status:     SegmentPending,
		}
		sm.nextID++
		sm.segments = append(sm.segments, seg)
		return sm.segments
	}

	if desiredConnections <= 0 {
		desiredConnections = 1
	}

	// Adjust connections if totalSize is small
	if totalSize < sm.minSegmentSize {
		desiredConnections = 1
	} else {
		maxPossible := int(totalSize / sm.minSegmentSize)
		if maxPossible < 1 {
			maxPossible = 1
		}
		if desiredConnections > maxPossible {
			desiredConnections = maxPossible
		}
	}

	chunkSize := totalSize / int64(desiredConnections)
	for i := 0; i < desiredConnections; i++ {
		start := int64(i) * chunkSize
		end := start + chunkSize - 1
		if i == desiredConnections-1 {
			end = totalSize - 1 // Last chunk absorbs remainder
		}

		seg := &Segment{
			ID:         sm.nextID,
			Start:      start,
			End:        end,
			Downloaded: 0,
			Status:     SegmentPending,
		}
		sm.nextID++
		sm.segments = append(sm.segments, seg)
	}

	return sm.segments
}

// RestoreSegments sets the existing segments (e.g. from resume state).
func (sm *SegmentManager) RestoreSegments(segs []*Segment) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.segments = segs
	maxID := 0
	for _, s := range segs {
		if s.ID > maxID {
			maxID = s.ID
		}
		// Reset any unfinished segments to SegmentPending so workers resume them
		if s.Status != SegmentCompleted && s.Remaining() > 0 {
			s.Status = SegmentPending
		}
	}
	sm.nextID = maxID + 1
}

// GetAllSegments returns a shallow copy of current segments.
func (sm *SegmentManager) GetAllSegments() []*Segment {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	res := make([]*Segment, len(sm.segments))
	copy(res, sm.segments)
	return res
}

// GetPendingSegment picks a pending segment that needs downloading.
func (sm *SegmentManager) GetPendingSegment() *Segment {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	for _, s := range sm.segments {
		if s.Status == SegmentPending && s.Remaining() > 0 {
			s.Status = SegmentDownloading
			return s
		}
	}
	return nil
}

// PrioritizeOffset finds or creates a segment starting near the requested offset.
func (sm *SegmentManager) PrioritizeOffset(offset int64) *Segment {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Look for a segment that covers this offset
	for _, s := range sm.segments {
		if s.Start <= offset && offset <= s.End {
			// If already downloaded past this offset, no action needed
			if offset < s.CurrentOffset() {
				return nil
			}
			// If pending, mark as next priority
			if s.Status == SegmentPending {
				s.Status = SegmentDownloading
				return s
			}
			// If already downloading, it is being fetched
			return s
		}
	}
	return nil
}

// IsRangeAvailable checks if bytes in [start, end] have already been downloaded.
func (sm *SegmentManager) IsRangeAvailable(start, end int64) bool {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if start > end {
		return false
	}

	for _, s := range sm.segments {
		if s.Start <= start && start <= s.End {
			availEnd := s.Start + s.Downloaded - 1
			if availEnd >= end {
				return true
			}
			if s.Status == SegmentCompleted && s.End >= end {
				return true
			}
		}
	}
	return false
}

// AvailableBytesAt returns how many contiguous bytes are ready starting from offset.
func (sm *SegmentManager) AvailableBytesAt(offset int64) int64 {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	for _, s := range sm.segments {
		if s.Start <= offset && offset <= s.End {
			availEnd := s.Start + s.Downloaded - 1
			if s.Status == SegmentCompleted {
				availEnd = s.End
			}
			if availEnd >= offset {
				return availEnd - offset + 1
			}
		}
	}
	return 0
}

// TrySplitLargest finds the active segment with the largest remaining work
// and splits its unfinished tail into a new segment for an idle worker.
func (sm *SegmentManager) TrySplitLargest() *Segment {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	var candidate *Segment
	var maxRemaining int64 = 0

	for _, s := range sm.segments {
		if s.Status == SegmentDownloading {
			rem := s.Remaining()
			if rem > maxRemaining {
				maxRemaining = rem
				candidate = s
			}
		}
	}

	// Only split if remaining bytes is at least double the minimum segment size
	if candidate == nil || maxRemaining < (2*sm.minSegmentSize) {
		return nil
	}

	// Split remaining bytes in half
	splitBytes := maxRemaining / 2
	originalEnd := candidate.End

	// Old segment now ends at split point
	candidate.End = candidate.CurrentOffset() + splitBytes - 1

	// New segment covers the remainder
	newSeg := &Segment{
		ID:         sm.nextID,
		Start:      candidate.End + 1,
		End:        originalEnd,
		Downloaded: 0,
		Status:     SegmentDownloading,
	}
	sm.nextID++
	sm.segments = append(sm.segments, newSeg)

	return newSeg
}

// IsCompleted checks if all segments have completed.
func (sm *SegmentManager) IsCompleted() bool {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if len(sm.segments) == 0 {
		return false
	}
	for _, s := range sm.segments {
		if s.Status != SegmentCompleted || s.Remaining() > 0 {
			return false
		}
	}
	return true
}

// TotalDownloaded sums downloaded bytes across all segments.
func (sm *SegmentManager) TotalDownloaded() int64 {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	var total int64
	for _, s := range sm.segments {
		total += s.Downloaded
	}
	return total
}
