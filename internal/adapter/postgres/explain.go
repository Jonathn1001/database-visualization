package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/elgnas/dbviz/internal/adapter"
	"github.com/elgnas/dbviz/internal/model"
)

// Only SELECT / WITH statements are allowed through EXPLAIN (§10.3).
var leadingKeyword = regexp.MustCompile(`^\s*(?:--[^\n]*\n|\s)*([a-zA-Z]+)`)

// ExplainQuery runs EXPLAIN (FORMAT JSON) and reduces the plan to the ordered
// list of relations touched (§10.3). It rejects any non-read statement.
func (a *Adapter) ExplainQuery(ctx context.Context, query string) (*model.QueryPlan, error) {
	if a.pool == nil {
		return nil, adapter.ErrNotOpen
	}
	if err := validateReadOnlyQuery(query); err != nil {
		return nil, err
	}

	// VERBOSE adds the "Schema" field to scan nodes so relation names match the
	// schema-qualified node IDs used in the GraphModel.
	sql := "EXPLAIN (FORMAT JSON, VERBOSE TRUE, ANALYZE FALSE, BUFFERS FALSE) " + query
	var raw []byte
	if err := a.pool.QueryRow(ctx, sql).Scan(&raw); err != nil {
		if isContextErr(ctx) {
			return nil, model.NewAPIError(model.ErrExplainTimeout, "EXPLAIN timed out")
		}
		return nil, model.NewAPIError(model.ErrExplainInvalidSQL, err.Error())
	}

	plan := &model.QueryPlan{Query: query}
	plan.Nodes = parseExplainJSON(raw)
	return plan, nil
}

func validateReadOnlyQuery(query string) error {
	m := leadingKeyword.FindStringSubmatch(query)
	if m == nil {
		return model.NewAPIError(model.ErrExplainInvalidSQL, "could not parse query")
	}
	switch strings.ToUpper(m[1]) {
	case "SELECT", "WITH":
		return nil
	default:
		return model.NewAPIError(model.ErrExplainInvalidSQL,
			"only SELECT/WITH statements may be explained").
			WithHint("write operations are not permitted")
	}
}

// parseExplainJSON walks the nested EXPLAIN output depth-first, collecting every
// plan node that reads a relation, in execution order.
func parseExplainJSON(raw []byte) []model.ExplainNode {
	var top []struct {
		Plan map[string]any `json:"Plan"`
	}
	if err := json.Unmarshal(raw, &top); err != nil || len(top) == 0 {
		return nil
	}
	var out []model.ExplainNode
	walkPlan(top[0].Plan, &out)
	return out
}

func walkPlan(plan map[string]any, out *[]model.ExplainNode) {
	if plan == nil {
		return
	}
	// Children first is "bottom-up" execution order, which matches how data
	// flows through the plan toward the root.
	if children, ok := plan["Plans"].([]any); ok {
		for _, c := range children {
			if cm, ok := c.(map[string]any); ok {
				walkPlan(cm, out)
			}
		}
	}
	if rel, ok := plan["Relation Name"].(string); ok {
		node := model.ExplainNode{
			Table: rel,
			Op:    asString(plan["Node Type"]),
			Cost:  asFloat(plan["Total Cost"]),
		}
		if schema, ok := plan["Schema"].(string); ok && schema != "" {
			node.Table = fmt.Sprintf("%s.%s", schema, rel)
		}
		*out = append(*out, node)
	}
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func asFloat(v any) float64 {
	if f, ok := v.(float64); ok {
		return f
	}
	return 0
}

func isContextErr(ctx context.Context) bool {
	return ctx.Err() != nil
}
