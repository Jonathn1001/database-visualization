package insights

import (
	"context"
	"errors"
	"testing"

	"github.com/elgnas/dbviz/internal/model"
)

// stubStatser implements both capabilities.
type stubStatser struct {
	idx []IndexStat
	tbl []model.TableStats
	err error
}

func (s stubStatser) IndexStats(context.Context) ([]IndexStat, error)           { return s.idx, s.err }
func (s stubStatser) AllTableStats(context.Context) ([]model.TableStats, error) { return s.tbl, s.err }

// bare implements neither capability.
type bare struct{}

func catByName(r Result, name string) *CategoryResult {
	for i := range r.Categories {
		if r.Categories[i].Category == name {
			return &r.Categories[i]
		}
	}
	return nil
}

func TestCollectUnsupportedCapabilities(t *testing.T) {
	r, err := Collect(context.Background(), gapsGraph(), bare{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Categories) != 3 {
		t.Fatalf("expected 3 categories, got %+v", r.Categories)
	}
	if got := catByName(r, CategoryIndex); got == nil || got.Status != StatusUnsupported {
		t.Errorf("index category = %+v, want unsupported", got)
	}
	if got := catByName(r, CategoryHealth); got == nil || got.Status != StatusUnsupported {
		t.Errorf("health category = %+v, want unsupported", got)
	}
	gaps := catByName(r, CategoryGaps)
	if gaps == nil || gaps.Status != StatusOK {
		t.Fatalf("gaps category = %+v, want ok", gaps)
	}
	if len(gaps.Findings) == 0 || len(gaps.ImpliedLinks) != len(gaps.Findings) {
		t.Errorf("gaps findings/links = %d/%d", len(gaps.Findings), len(gaps.ImpliedLinks))
	}
	// Unsupported categories still carry an empty (non-nil) findings array.
	if catByName(r, CategoryIndex).Findings == nil {
		t.Error("unsupported category has nil findings; must serialize as []")
	}
}

func TestCollectWithCapabilities(t *testing.T) {
	src := stubStatser{
		idx: []IndexStat{
			{NodeID: "public.users", Index: "idx_a", Columns: []string{"email"}, Scans: 0},
		},
		tbl: []model.TableStats{
			{NodeID: "public.users", RowCount: 900, DeadTuples: 600},    // in graph -> analyzed
			{NodeID: "other.ghost", RowCount: 900, DeadTuples: 900_000}, // not in graph -> dropped
		},
	}
	r, err := Collect(context.Background(), gapsGraph(), src, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := catByName(r, CategoryIndex); got.Status != StatusOK || len(got.Findings) == 0 {
		t.Errorf("index category = %+v", got)
	}
	health := catByName(r, CategoryHealth)
	if health.Status != StatusOK {
		t.Fatalf("health category = %+v", health)
	}
	for _, f := range health.Findings {
		if f.NodeID == "other.ghost" {
			t.Errorf("stats for node absent from graph were not prefiltered: %+v", f)
		}
	}
}

func TestCollectCategoryFilter(t *testing.T) {
	r, err := Collect(context.Background(), gapsGraph(), bare{}, CategoryGaps)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Categories) != 1 || r.Categories[0].Category != CategoryGaps {
		t.Errorf("filtered result = %+v", r.Categories)
	}
}

func TestCollectInvalidCategory(t *testing.T) {
	_, err := Collect(context.Background(), gapsGraph(), bare{}, "bogus")
	var apiErr *model.APIError
	if !errors.As(err, &apiErr) || apiErr.Code != model.ErrBadRequest {
		t.Errorf("err = %v, want APIError BAD_REQUEST", err)
	}
}

func TestCollectCapabilityErrorPropagates(t *testing.T) {
	boom := errors.New("connection reset")
	_, err := Collect(context.Background(), gapsGraph(), stubStatser{err: boom}, CategoryIndex)
	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want wrapped %v", err, boom)
	}
}
