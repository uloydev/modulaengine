package query

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var filterRegex = regexp.MustCompile(`^filter\[([a-z0-9_]+)\]\[(_[a-z0-9_]+)\]$`)

type SortOrder string

const (
	SortAsc  SortOrder = "ASC"
	SortDesc SortOrder = "DESC"
)

type SortField struct {
	Field string
	Order SortOrder
}

type QueryParams struct {
	Filter  Node
	Sort    []SortField
	Fields  []string
	Expand  []string
	Limit   int
	Offset  int
}

func ParseQueryParams(values url.Values) (*QueryParams, error) {
	qp := &QueryParams{
		Limit:  25,
		Offset: 0,
	}

	if l := values.Get("limit"); l != "" {
		parsed, err := strconv.Atoi(l)
		if err != nil || parsed < 1 {
			return nil, fmt.Errorf("%w: invalid limit '%s'", ErrInvalidFilterNode, l)
		}
		if parsed > 100 {
			parsed = 100
		}
		qp.Limit = parsed
	}

	if o := values.Get("offset"); o != "" {
		parsed, err := strconv.Atoi(o)
		if err != nil || parsed < 0 {
			return nil, fmt.Errorf("%w: invalid offset '%s'", ErrInvalidFilterNode, o)
		}
		qp.Offset = parsed
	}

	if s := values.Get("sort"); s != "" {
		parts := strings.Split(s, ",")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			order := SortAsc
			field := part
			if strings.HasPrefix(part, "-") {
				order = SortDesc
				field = strings.TrimPrefix(part, "-")
			}
			qp.Sort = append(qp.Sort, SortField{
				Field: field,
				Order: order,
			})
		}
	}

	if f := values.Get("fields"); f != "" {
		parts := strings.Split(f, ",")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part != "" {
				qp.Fields = append(qp.Fields, part)
			}
		}
	}

	if e := values.Get("expand"); e != "" {
		parts := strings.Split(e, ",")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part != "" {
				qp.Expand = append(qp.Expand, part)
			}
		}
	}

	var filterNodes []Node
	for key, valSlice := range values {
		matches := filterRegex.FindStringSubmatch(key)
		if len(matches) != 3 {
			continue
		}
		fieldName := matches[1]
		opStr := Op(matches[2])
		rawVal := ""
		if len(valSlice) > 0 {
			rawVal = valSlice[0]
		}

		if opStr == OpNull {
			isNull := strings.ToLower(rawVal) == "true" || rawVal == "1"
			filterNodes = append(filterNodes, &NullNode{
				Field:  fieldName,
				IsNull: isNull,
			})
			continue
		}

		if opStr == OpIn || opStr == OpNin {
			rawItems := strings.Split(rawVal, ",")
			cleanItems := make([]any, 0, len(rawItems))
			for _, item := range rawItems {
				item = strings.TrimSpace(item)
				if item != "" {
					cleanItems = append(cleanItems, item)
				}
			}
			filterNodes = append(filterNodes, &ComparisonNode{
				Field: fieldName,
				Op:    opStr,
				Value: cleanItems,
			})
			continue
		}

		filterNodes = append(filterNodes, &ComparisonNode{
			Field: fieldName,
			Op:    opStr,
			Value: rawVal,
		})
	}

	if len(filterNodes) == 1 {
		qp.Filter = filterNodes[0]
	} else if len(filterNodes) > 1 {
		qp.Filter = &LogicalNode{
			Op:       LogicalAnd,
			Children: filterNodes,
		}
	}

	if qp.Filter != nil {
		if err := qp.Filter.Validate(1); err != nil {
			return nil, err
		}
	}

	return qp, nil
}
