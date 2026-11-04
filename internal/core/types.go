package core

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

var identifierRegex = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

var reservedIdentifiers = map[string]struct{}{
	"id":         {},
	"public_id":  {},
	"tenant_id":  {},
	"created_at": {},
	"created_by": {},
	"updated_at": {},
	"updated_by": {},
	"deleted_at": {},
	"deleted_by": {},
	"version":    {},
}

func ValidateIdentifier(name string) error {
	if !identifierRegex.MatchString(name) {
		return fmt.Errorf("%w: '%s' must match [a-z][a-z0-9_]{0,62}", ErrInvalidIdentifier, name)
	}
	return nil
}

func IsReservedIdentifier(name string) bool {
	_, exists := reservedIdentifiers[strings.ToLower(name)]
	return exists
}

type Tenant struct {
	ID        int64     `json:"-"`
	PublicID  string    `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type FieldType string

const (
	FieldTypeText     FieldType = "text"
	FieldTypeNumber   FieldType = "number"
	FieldTypeBoolean  FieldType = "boolean"
	FieldTypeDateTime FieldType = "datetime"
	FieldTypeEnum     FieldType = "enum"
	FieldTypeJSON     FieldType = "json"
	FieldTypeRelation FieldType = "relation"
)

func (ft FieldType) IsValid() bool {
	switch ft {
	case FieldTypeText, FieldTypeNumber, FieldTypeBoolean, FieldTypeDateTime,
		FieldTypeEnum, FieldTypeJSON, FieldTypeRelation:
		return true
	default:
		return false
	}
}

type RelationType string

const (
	RelationTypeM2O RelationType = "M2O"
	RelationTypeO2M RelationType = "O2M"
)

func (rt RelationType) IsValid() bool {
	return rt == RelationTypeM2O || rt == RelationTypeO2M
}

type OnDeleteAction string

const (
	OnDeleteCascade  OnDeleteAction = "CASCADE"
	OnDeleteSetNull  OnDeleteAction = "SET_NULL"
	OnDeleteRestrict OnDeleteAction = "RESTRICT"
)

func (a OnDeleteAction) IsValid() bool {
	switch a {
	case OnDeleteCascade, OnDeleteSetNull, OnDeleteRestrict:
		return true
	default:
		return false
	}
}

func (a OnDeleteAction) SQL() string {
	switch a {
	case OnDeleteSetNull:
		return "SET NULL"
	case OnDeleteCascade:
		return "CASCADE"
	case OnDeleteRestrict:
		return "RESTRICT"
	default:
		return "RESTRICT"
	}
}

type RelationDef struct {
	RelationType     RelationType   `json:"relation_type"`
	TargetCollection string         `json:"target_collection"`
	OnDelete         OnDeleteAction `json:"on_delete"`
}

type Field struct {
	ID           int64        `json:"-"`
	CollectionID int64        `json:"-"`
	Name         string       `json:"name"`
	Type         FieldType    `json:"type"`
	IsRequired   bool         `json:"required"`
	IsUnique     bool         `json:"unique"`
	IsSearchable bool         `json:"searchable"`
	IsEncrypted  bool         `json:"is_encrypted"`
	DefaultValue *string      `json:"default,omitempty"`
	Options      []string     `json:"options,omitempty"`
	Precision    *int         `json:"precision,omitempty"`
	Scale        *int         `json:"scale,omitempty"`
	Relation     *RelationDef `json:"relation,omitempty"`
	IsActive     bool         `json:"is_active"`
	CreatedAt    time.Time    `json:"created_at"`
	UpdatedAt    time.Time    `json:"updated_at"`
}

func (f *Field) Validate() error {
	if err := ValidateIdentifier(f.Name); err != nil {
		return err
	}
	if IsReservedIdentifier(f.Name) {
		return fmt.Errorf("%w: field name '%s'", ErrReservedName, f.Name)
	}
	if !f.Type.IsValid() {
		return fmt.Errorf("%w: invalid field type '%s'", ErrValidation, f.Type)
	}
	if f.IsEncrypted && f.IsSearchable {
		return fmt.Errorf("%w: encrypted field '%s' cannot be searchable", ErrValidation, f.Name)
	}
	if f.Type == FieldTypeNumber {
		if f.Precision != nil {
			if *f.Precision < 1 || *f.Precision > 1000 {
				return fmt.Errorf("%w: numeric precision must be between 1 and 1000", ErrValidation)
			}
		}
		if f.Scale != nil {
			if *f.Scale < 0 {
				return fmt.Errorf("%w: numeric scale cannot be negative", ErrValidation)
			}
			if f.Precision != nil && *f.Scale > *f.Precision {
				return fmt.Errorf("%w: numeric scale cannot exceed precision", ErrValidation)
			}
		}
	}
	if f.Type == FieldTypeEnum && len(f.Options) == 0 {
		return fmt.Errorf("%w: enum field '%s' requires at least one option", ErrValidation, f.Name)
	}
	if f.Type == FieldTypeRelation {
		if f.Relation == nil {
			return fmt.Errorf("%w: relation field '%s' missing relation definition", ErrValidation, f.Name)
		}
		if f.Relation.RelationType == "" {
			f.Relation.RelationType = RelationTypeM2O
		}
		if !f.Relation.RelationType.IsValid() {
			return fmt.Errorf("%w: invalid relation type '%s'", ErrValidation, f.Relation.RelationType)
		}
		if err := ValidateIdentifier(f.Relation.TargetCollection); err != nil {
			return fmt.Errorf("%w: target collection '%s'", err, f.Relation.TargetCollection)
		}
		if f.Relation.OnDelete == "" {
			f.Relation.OnDelete = OnDeleteRestrict
		}
		if !f.Relation.OnDelete.IsValid() {
			return fmt.Errorf("%w: invalid on_delete action '%s'", ErrValidation, f.Relation.OnDelete)
		}
	}
	return nil
}

type Collection struct {
	ID        int64              `json:"-"`
	TenantID  int64              `json:"-"`
	Name      string             `json:"name"`
	TableName string             `json:"table_name"`
	Label     string             `json:"label"`
	IsSystem  bool               `json:"is_system"`
	Fields    map[string]*Field  `json:"fields"`
	FieldList []*Field           `json:"field_list"`
	CreatedAt time.Time          `json:"created_at"`
	UpdatedAt time.Time          `json:"updated_at"`
}

func NewCollection(tenantID int64, name, label string) (*Collection, error) {
	if err := ValidateIdentifier(name); err != nil {
		return nil, fmt.Errorf("collection name validation: %w", err)
	}
	return &Collection{
		TenantID:  tenantID,
		Name:      name,
		TableName: "usr_" + name,
		Label:     label,
		Fields:    make(map[string]*Field),
		FieldList: make([]*Field, 0),
	}, nil
}

func (c *Collection) AddField(f *Field) error {
	if err := f.Validate(); err != nil {
		return fmt.Errorf("field '%s': %w", f.Name, err)
	}
	if _, exists := c.Fields[f.Name]; exists {
		return fmt.Errorf("%w: field '%s' on collection '%s'", ErrFieldExists, f.Name, c.Name)
	}
	c.Fields[f.Name] = f
	c.FieldList = append(c.FieldList, f)
	return nil
}

type Record struct {
	ID        int64          `json:"-"`
	PublicID  string         `json:"id"`
	TenantID  int64          `json:"-"`
	Version   int            `json:"version"`
	Values    map[string]any `json:"values"`
	CreatedAt time.Time      `json:"created_at"`
	CreatedBy int64          `json:"-"`
	UpdatedAt time.Time      `json:"updated_at"`
	UpdatedBy int64          `json:"-"`
	DeletedAt *time.Time     `json:"-"`
	DeletedBy *int64         `json:"-"`
}

func NewRecord(tenantID int64, publicID string, version int) *Record {
	return &Record{
		TenantID: tenantID,
		PublicID: publicID,
		Version:  version,
		Values:   make(map[string]any),
	}
}
