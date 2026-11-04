package core

import "errors"

var (
	ErrNotFound           = errors.New("record not found")
	ErrVersionConflict     = errors.New("optimistic concurrency conflict")
	ErrTenantMismatch     = errors.New("tenant authorization mismatch")
	ErrInvalidIdentifier  = errors.New("invalid identifier")
	ErrValidation         = errors.New("validation failed")
	ErrCollectionExists   = errors.New("collection already exists")
	ErrCollectionNotFound = errors.New("collection not found")
	ErrFieldExists        = errors.New("field already exists")
	ErrFieldNotFound      = errors.New("field not found")
	ErrReservedName       = errors.New("identifier uses reserved system name")
)
