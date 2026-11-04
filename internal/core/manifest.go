package core

import (
	"encoding/json"
	"fmt"
	"os"
)

type ManifestField struct {
	Name             string         `json:"name"`
	Type             FieldType      `json:"type"`
	Required         bool           `json:"required"`
	Unique           bool           `json:"unique"`
	Searchable       bool           `json:"searchable"`
	Encrypted        bool           `json:"encrypted"`
	Default          *string        `json:"default,omitempty"`
	Options          []string       `json:"options,omitempty"`
	Precision        *int           `json:"precision,omitempty"`
	Scale            *int           `json:"scale,omitempty"`
	RelationType     RelationType   `json:"relation_type,omitempty"`
	TargetCollection string         `json:"target_collection,omitempty"`
	OnDelete         OnDeleteAction `json:"on_delete,omitempty"`
}

type ManifestCollection struct {
	Name   string          `json:"name"`
	Label  string          `json:"label"`
	Fields []ManifestField `json:"fields"`
}

type ManifestPermission struct {
	Collection    string          `json:"collection"`
	Actions       []Action        `json:"actions"`
	RLSFilter     json.RawMessage `json:"rls_filter,omitempty"`
	AllowedFields []string        `json:"allowed_fields"`
}

type ManifestRole struct {
	Name        string               `json:"name"`
	Permissions []ManifestPermission `json:"permissions"`
}

type Manifest struct {
	Name        string               `json:"name"`
	Version     string               `json:"version"`
	Collections []ManifestCollection `json:"collections"`
	Roles       []ManifestRole       `json:"roles"`
}

func ParseManifest(data []byte) (*Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("%w: invalid manifest json: %w", ErrValidation, err)
	}
	return &m, nil
}

func LoadManifestFile(filePath string) (*Manifest, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read manifest file: %w", err)
	}
	return ParseManifest(data)
}

func (m *Manifest) BuildCollections(tenantID int64) ([]*Collection, error) {
	collections := make([]*Collection, 0, len(m.Collections))

	for _, mc := range m.Collections {
		col, err := NewCollection(tenantID, mc.Name, mc.Label)
		if err != nil {
			return nil, fmt.Errorf("collection '%s': %w", mc.Name, err)
		}

		for _, mf := range mc.Fields {
			var rel *RelationDef
			if mf.Type == FieldTypeRelation {
				rel = &RelationDef{
					RelationType:     mf.RelationType,
					TargetCollection: mf.TargetCollection,
					OnDelete:         mf.OnDelete,
				}
			}

			field := &Field{
				Name:         mf.Name,
				Type:         mf.Type,
				IsRequired:   mf.Required,
				IsUnique:     mf.Unique,
				IsSearchable: mf.Searchable,
				IsEncrypted:  mf.Encrypted,
				DefaultValue: mf.Default,
				Options:      mf.Options,
				Precision:    mf.Precision,
				Scale:        mf.Scale,
				Relation:     rel,
				IsActive:     true,
			}

			if err := col.AddField(field); err != nil {
				return nil, fmt.Errorf("collection '%s' field '%s': %w", mc.Name, mf.Name, err)
			}
		}

		collections = append(collections, col)
	}

	return collections, nil
}

func (m *Manifest) BuildPermissions(tenantID int64, collections []*Collection) ([]*Permission, error) {
	colMap := make(map[string]*Collection, len(collections))
	for _, c := range collections {
		colMap[c.Name] = c
	}

	var perms []*Permission
	for roleIdx, role := range m.Roles {
		roleID := int64(roleIdx + 1)
		for _, mp := range role.Permissions {
			targetCol, exists := colMap[mp.Collection]
			if !exists {
				return nil, fmt.Errorf("role '%s' references unknown collection '%s'", role.Name, mp.Collection)
			}

			rlsStr := ""
			if len(mp.RLSFilter) > 0 {
				rlsStr = string(mp.RLSFilter)
			}

			perm := &Permission{
				TenantID:      tenantID,
				RoleID:        roleID,
				CollectionID:  targetCol.ID,
				Actions:       mp.Actions,
				AllowedFields: mp.AllowedFields,
				RLSFilter:     rlsStr,
			}
			perms = append(perms, perm)
		}
	}

	return perms, nil
}
