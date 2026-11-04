package query

import (
	"errors"
	"fmt"
)

const (
	MaxASTDepth    = 5
	MaxArrayParams = 500
)

var (
	ErrDepthExceeded      = errors.New("filter tree exceeds maximum nesting depth of 5")
	ErrArraySizeExceeded  = errors.New("array parameter exceeds maximum size of 500")
	ErrUnsupportedOp      = errors.New("unsupported filter operator")
	ErrInvalidFilterNode  = errors.New("invalid filter node")
	ErrUnknownField       = errors.New("unknown field identifier")
	ErrFieldNotPermitted  = errors.New("field access not permitted")
)

type Op string

const (
	OpEq    Op = "_eq"
	OpNeq   Op = "_neq"
	OpGt    Op = "_gt"
	OpGte   Op = "_gte"
	OpLt    Op = "_lt"
	OpLte   Op = "_lte"
	OpIn    Op = "_in"
	OpNin   Op = "_nin"
	OpLike  Op = "_like"
	OpILike Op = "_ilike"
	OpNull  Op = "_null"
)

func (o Op) SQL() (string, error) {
	switch o {
	case OpEq:
		return "=", nil
	case OpNeq:
		return "!=", nil
	case OpGt:
		return ">", nil
	case OpGte:
		return ">=", nil
	case OpLt:
		return "<", nil
	case OpLte:
		return "<=", nil
	case OpLike:
		return "LIKE", nil
	case OpILike:
		return "ILIKE", nil
	case OpIn:
		return "IN", nil
	case OpNin:
		return "NOT IN", nil
	default:
		return "", fmt.Errorf("%w: %s", ErrUnsupportedOp, o)
	}
}

type LogicalOp string

const (
	LogicalAnd LogicalOp = "and"
	LogicalOr  LogicalOp = "or"
)

type Node interface {
	Depth() int
	Validate(depth int) error
}

type ComparisonNode struct {
	Field string
	Op    Op
	Value any
}

func (n *ComparisonNode) Depth() int {
	return 1
}

func (n *ComparisonNode) Validate(depth int) error {
	if depth > MaxASTDepth {
		return ErrDepthExceeded
	}
	if n.Field == "" {
		return fmt.Errorf("%w: field cannot be empty", ErrInvalidFilterNode)
	}
	if n.Op == OpIn || n.Op == OpNin {
		switch v := n.Value.(type) {
		case []any:
			if len(v) > MaxArrayParams {
				return ErrArraySizeExceeded
			}
			if len(v) == 0 {
				return fmt.Errorf("%w: in/nin value array cannot be empty", ErrInvalidFilterNode)
			}
		case []string:
			if len(v) > MaxArrayParams {
				return ErrArraySizeExceeded
			}
			if len(v) == 0 {
				return fmt.Errorf("%w: in/nin value array cannot be empty", ErrInvalidFilterNode)
			}
		default:
			return fmt.Errorf("%w: %s requires array value", ErrInvalidFilterNode, n.Op)
		}
	}
	return nil
}

type LogicalNode struct {
	Op       LogicalOp
	Children []Node
}

func (n *LogicalNode) Depth() int {
	maxChild := 0
	for _, c := range n.Children {
		cd := c.Depth()
		if cd > maxChild {
			maxChild = cd
		}
	}
	return maxChild + 1
}

func (n *LogicalNode) Validate(depth int) error {
	if depth > MaxASTDepth {
		return ErrDepthExceeded
	}
	for _, c := range n.Children {
		if err := c.Validate(depth + 1); err != nil {
			return err
		}
	}
	return nil
}

type NullNode struct {
	Field  string
	IsNull bool
}

func (n *NullNode) Depth() int {
	return 1
}

func (n *NullNode) Validate(depth int) error {
	if depth > MaxASTDepth {
		return ErrDepthExceeded
	}
	if n.Field == "" {
		return fmt.Errorf("%w: null check field cannot be empty", ErrInvalidFilterNode)
	}
	return nil
}
