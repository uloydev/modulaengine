package query

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/uloydev/modulaengine/internal/core"
)

type rawRLSNode struct {
	Op       string          `json:"op"`
	Field    string          `json:"field,omitempty"`
	Value    any             `json:"value,omitempty"`
	Children []rawRLSNode    `json:"children,omitempty"`
}

func ParseRLSFilter(rawJSON string, secCtx *core.SecurityContext) (Node, error) {
	if strings.TrimSpace(rawJSON) == "" {
		return nil, nil
	}

	var raw rawRLSNode
	if err := json.Unmarshal([]byte(rawJSON), &raw); err != nil {
		return nil, fmt.Errorf("%w: invalid RLS JSON: %w", ErrInvalidFilterNode, err)
	}

	node, err := buildRLSNode(raw, secCtx, 1)
	if err != nil {
		return nil, err
	}

	return node, nil
}

func buildRLSNode(raw rawRLSNode, secCtx *core.SecurityContext, depth int) (Node, error) {
	if depth > MaxASTDepth {
		return nil, ErrDepthExceeded
	}

	op := strings.ToLower(raw.Op)
	switch op {
	case "and", "or":
		logicalOp := LogicalAnd
		if op == "or" {
			logicalOp = LogicalOr
		}
		children := make([]Node, 0, len(raw.Children))
		for _, childRaw := range raw.Children {
			child, err := buildRLSNode(childRaw, secCtx, depth+1)
			if err != nil {
				return nil, err
			}
			if child != nil {
				children = append(children, child)
			}
		}
		if len(children) == 0 {
			return nil, nil
		}
		if len(children) == 1 {
			return children[0], nil
		}
		return &LogicalNode{
			Op:       logicalOp,
			Children: children,
		}, nil

	case "null":
		valBool, _ := raw.Value.(bool)
		return &NullNode{
			Field:  raw.Field,
			IsNull: valBool,
		}, nil

	default:
		opStr := Op("_" + strings.TrimPrefix(op, "_"))
		resolvedVal := substituteContextVar(raw.Value, secCtx)
		return &ComparisonNode{
			Field: raw.Field,
			Op:    opStr,
			Value: resolvedVal,
		}, nil
	}
}

func substituteContextVar(val any, secCtx *core.SecurityContext) any {
	if secCtx == nil {
		return val
	}
	strVal, ok := val.(string)
	if !ok {
		return val
	}

	trimmed := strings.TrimSpace(strVal)
	switch trimmed {
	case "{{user.id}}":
		return secCtx.UserID
	case "{{user.role}}":
		return secCtx.RoleName
	case "{{tenant.id}}":
		return secCtx.TenantID
	default:
		return val
	}
}
