package insights

import (
	"testing"

	"github.com/elgnas/dbviz/internal/model"
)

func TestHealthFindingsDeadTupleRatio(t *testing.T) {
	cases := []struct {
		name    string
		live    int64
		dead    int64
		wantSev string // "" = no dead-ratio finding expected
	}{
		{"critical", 400, 600, SeverityCritical}, // 60% dead
		{"warn", 750, 250, SeverityWarn},         // 25% dead
		{"below threshold", 950, 50, ""},         // 5% dead
		{"too small to matter", 40, 60, ""},      // 60% dead but 100 rows total
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fs := HealthFindings([]model.TableStats{{
				NodeID: "public.t", RowCount: tc.live, DeadTuples: tc.dead,
				SizeBytes: 1 << 20, LastVacuum: "2026-07-01T00:00:00Z",
			}})
			var got string
			for _, f := range fs {
				if f.Title == "High dead-tuple ratio" {
					got = f.Severity
					if f.Meta["estBloatBytes"] == nil {
						t.Error("dead-ratio finding missing estBloatBytes meta")
					}
				}
			}
			if got != tc.wantSev {
				t.Errorf("severity = %q, want %q (findings: %+v)", got, tc.wantSev, fs)
			}
		})
	}
}

func TestHealthFindingsNeverVacuumed(t *testing.T) {
	fs := HealthFindings([]model.TableStats{{
		NodeID: "public.t", RowCount: 5000, DeadTuples: 100,
	}})
	found := false
	for _, f := range fs {
		if f.Title == "Never vacuumed" && f.Severity == SeverityInfo {
			found = true
		}
	}
	if !found {
		t.Errorf("expected never-vacuumed finding, got %+v", fs)
	}

	// Autovacuum counts as vacuumed.
	fs = HealthFindings([]model.TableStats{{
		NodeID: "public.t", RowCount: 5000, DeadTuples: 100,
		LastAutovacuum: "2026-07-01T00:00:00Z",
	}})
	for _, f := range fs {
		if f.Title == "Never vacuumed" {
			t.Errorf("autovacuumed table reported never-vacuumed: %+v", f)
		}
	}
}

func TestHealthFindingsSeqScanHeavy(t *testing.T) {
	fs := HealthFindings([]model.TableStats{{
		NodeID: "public.t", RowCount: 50_000, SeqScans: 500, IdxScans: 10,
		LastVacuum: "2026-07-01T00:00:00Z",
	}})
	found := false
	for _, f := range fs {
		if f.Title == "Sequential-scan heavy" && f.Severity == SeverityWarn {
			found = true
		}
	}
	if !found {
		t.Errorf("expected seq-scan-heavy finding, got %+v", fs)
	}

	// Healthy index usage -> no finding.
	fs = HealthFindings([]model.TableStats{{
		NodeID: "public.t", RowCount: 50_000, SeqScans: 500, IdxScans: 400,
		LastVacuum: "2026-07-01T00:00:00Z",
	}})
	for _, f := range fs {
		if f.Title == "Sequential-scan heavy" {
			t.Errorf("healthy table reported seq-scan heavy: %+v", f)
		}
	}

	// Small tables are exempt.
	fs = HealthFindings([]model.TableStats{{
		NodeID: "public.t", RowCount: 500, SeqScans: 5000,
		LastVacuum: "2026-07-01T00:00:00Z",
	}})
	for _, f := range fs {
		if f.Title == "Sequential-scan heavy" {
			t.Errorf("small table reported seq-scan heavy: %+v", f)
		}
	}
}
