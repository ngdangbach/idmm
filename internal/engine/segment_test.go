package engine

import (
	"testing"
)

func TestInitializeSegments(t *testing.T) {
	sm := NewSegmentManager(1024) // 1 KB min size

	// 10 KB file with 4 connections
	var totalSize int64 = 10 * 1024
	segs := sm.InitializeSegments(totalSize, 4)

	if len(segs) != 4 {
		t.Fatalf("expected 4 segments, got %d", len(segs))
	}

	// Verify continuous ranges from 0 to totalSize-1
	var expectedStart int64 = 0
	for i, s := range segs {
		if s.Start != expectedStart {
			t.Errorf("segment %d start: expected %d, got %d", i, expectedStart, s.Start)
		}
		if s.End < s.Start {
			t.Errorf("segment %d end %d is before start %d", i, s.End, s.Start)
		}
		expectedStart = s.End + 1
	}

	if segs[len(segs)-1].End != totalSize-1 {
		t.Errorf("last segment end: expected %d, got %d", totalSize-1, segs[len(segs)-1].End)
	}
}

func TestDynamicReSplit(t *testing.T) {
	sm := NewSegmentManager(1024) // 1 KB min chunk size

	// Start with 1 segment of 100 KB
	var totalSize int64 = 100 * 1024
	segs := sm.InitializeSegments(totalSize, 1)

	if len(segs) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(segs))
	}

	// Mark segment as downloading
	s0 := sm.GetPendingSegment()
	if s0 == nil {
		t.Fatal("expected pending segment")
	}
	s0.Downloaded = 20 * 1024 // 20 KB downloaded, 80 KB remaining

	// Another worker arrives looking for work -> should split s0
	s1 := sm.TrySplitLargest()
	if s1 == nil {
		t.Fatal("expected dynamic split to succeed")
	}

	// s0 remaining was 80KB. Split in half = 40KB for s0, 40KB for s1.
	// s0 current offset is Start(0) + Downloaded(20KB) = 20KB.
	// s0 new End should be 20KB + 40KB - 1 = 61439.
	// s1 Start should be 61440, End should be 100KB - 1 = 102399.
	if s1.Start != s0.End+1 {
		t.Errorf("gap or overlap: s0.End=%d, s1.Start=%d", s0.End, s1.Start)
	}
	if s1.End != totalSize-1 {
		t.Errorf("s1.End: expected %d, got %d", totalSize-1, s1.End)
	}

	all := sm.GetAllSegments()
	if len(all) != 2 {
		t.Errorf("expected 2 total segments after split, got %d", len(all))
	}
}

func TestCompletion(t *testing.T) {
	sm := NewSegmentManager(512)
	segs := sm.InitializeSegments(1000, 2)

	if sm.IsCompleted() {
		t.Error("should not be completed initially")
	}

	for _, s := range segs {
		s.Downloaded = s.End - s.Start + 1
		s.Status = SegmentCompleted
	}

	if !sm.IsCompleted() {
		t.Error("should be completed when all segments downloaded")
	}

	if sm.TotalDownloaded() != 1000 {
		t.Errorf("expected 1000 downloaded bytes, got %d", sm.TotalDownloaded())
	}
}
