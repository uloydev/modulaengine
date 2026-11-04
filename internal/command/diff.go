package command

import (
	"reflect"
)

type FieldDiff struct {
	Old any `json:"old"`
	New any `json:"new"`
}

type AuditDiff map[string]FieldDiff

func ComputeDiff(oldValues, newValues map[string]any) AuditDiff {
	diff := make(AuditDiff)

	for k, newVal := range newValues {
		oldVal, exists := oldValues[k]
		if !exists {
			diff[k] = FieldDiff{
				Old: nil,
				New: newVal,
			}
			continue
		}

		if !reflect.DeepEqual(oldVal, newVal) {
			diff[k] = FieldDiff{
				Old: oldVal,
				New: newVal,
			}
		}
	}

	for k, oldVal := range oldValues {
		if _, exists := newValues[k]; !exists {
			diff[k] = FieldDiff{
				Old: oldVal,
				New: nil,
			}
		}
	}

	return diff
}
