package insights

import (
	"testing"
)

func TestSortFindingsSeverityThenNodeThenTitle(t *testing.T) {
	fs := []Finding{
		{Severity: SeverityInfo, NodeID: "public.b", Title: "z"},
		{Severity: SeverityCritical, NodeID: "public.c", Title: "a"},
		{Severity: SeverityWarn, NodeID: "public.a", Title: "b"},
		{Severity: SeverityWarn, NodeID: "public.a", Title: "a"},
	}
	sortFindings(fs)

	want := []struct{ sev, node, title string }{
		{SeverityCritical, "public.c", "a"},
		{SeverityWarn, "public.a", "a"},
		{SeverityWarn, "public.a", "b"},
		{SeverityInfo, "public.b", "z"},
	}
	for i, w := range want {
		if fs[i].Severity != w.sev || fs[i].NodeID != w.node || fs[i].Title != w.title {
			t.Errorf("pos %d = {%s %s %s}, want {%s %s %s}",
				i, fs[i].Severity, fs[i].NodeID, fs[i].Title, w.sev, w.node, w.title)
		}
	}
}
