package settings

import (
	"context"
	"testing"
	"time"
)

func TestDecisionsRetentionDefaultsAndZeroAreDistinct(t *testing.T) {
	dr, err := decodeDataRetention(map[string]any{"messages_retention_days": 90})
	if err != nil || dr.DecisionsRetentionDays != 30 {
		t.Fatalf("missing new field must default to 30 days: %+v %v", dr, err)
	}
	dr, err = decodeDataRetention(map[string]any{"decisions_retention_days": 0})
	if err != nil || dr.DecisionsRetentionDays != 0 {
		t.Fatalf("explicit zero must disable cleanup: %+v %v", dr, err)
	}
}

func TestDecisionCleanupUsesSavedPolicyAndBoundedBatches(t *testing.T) {
	s := NewService(newFakeRepo(), fileCfg())
	now := time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	_, err := s.UpdateDataRetention(context.Background(), DataRetentionSettings{DecisionsRetentionDays: 7})
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	s.SetDecisionsCleaner(func(_ context.Context, before time.Time, limit int) (int64, error) {
		calls++
		if !before.Equal(now.Add(-7*24*time.Hour)) || limit != archiveBatchSize {
			t.Fatal("incorrect cleanup bounds")
		}
		return 2, nil
	})
	r, err := s.RunDataCleanup(context.Background(), DataCleanupTargets{Decisions: true})
	if err != nil || r.DeletedDecisions != 2 || calls != 1 {
		t.Fatalf("%+v %v calls=%d", r, err, calls)
	}
}
