package viewer

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/report"
)

func snapshotAt(finished time.Time, details bool) DashboardSnapshot {
	return DashboardSnapshot{RunID: uuid.New(), SourceFinishedAt: &finished, DetailsAvailable: details}
}

// A viewer who reopens a report should see what they or the schedule collected
// most recently, and their own run when both are equally recent because it has the
// rows. Neither being found stays "not found", which the page turns into a fetch.
func TestNewerSnapshotPrefersTheMoreRecentAndTheViewersOwnOnATie(t *testing.T) {
	now := time.Date(2026, 10, 1, 5, 0, 0, 0, time.UTC)
	tenant, own := snapshotAt(now, false), snapshotAt(now.Add(-10*time.Minute), true)
	if got, err := newerSnapshot(tenant, nil, own, nil); err != nil || got.RunID != tenant.RunID {
		t.Fatalf("a more recent tenant snapshot should win, got %+v %v", got, err)
	}
	own = snapshotAt(now.Add(5*time.Minute), true)
	if got, err := newerSnapshot(tenant, nil, own, nil); err != nil || got.RunID != own.RunID {
		t.Fatalf("a more recent own run should win, got %+v %v", got, err)
	}
	tied := snapshotAt(now, true)
	if got, err := newerSnapshot(tenant, nil, tied, nil); err != nil || got.RunID != tied.RunID || !got.DetailsAvailable {
		t.Fatalf("on a tie the own run, which has rows, should win, got %+v %v", got, err)
	}
}

func TestNewerSnapshotFallsBackToWhicheverExists(t *testing.T) {
	now := time.Date(2026, 10, 1, 5, 0, 0, 0, time.UTC)
	only := snapshotAt(now, true)
	missing := report.ErrRunNotFound
	if got, err := newerSnapshot(DashboardSnapshot{}, missing, only, nil); err != nil || got.RunID != only.RunID {
		t.Fatalf("own only: %+v %v", got, err)
	}
	if got, err := newerSnapshot(only, nil, DashboardSnapshot{}, missing); err != nil || got.RunID != only.RunID {
		t.Fatalf("tenant only: %+v %v", got, err)
	}
	if _, err := newerSnapshot(DashboardSnapshot{}, missing, DashboardSnapshot{}, missing); !errors.Is(err, report.ErrRunNotFound) {
		t.Fatalf("neither should be not found, got %v", err)
	}
}
