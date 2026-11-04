package query

import (
	"fmt"
	"sync"

	"github.com/uloydev/modulaengine/internal/core"
)

type Registry struct {
	mu          sync.RWMutex
	collections map[string]*core.Collection
	byID        map[int64]*core.Collection
	permissions map[string]*core.Permission
}

func NewRegistry() *Registry {
	return &Registry{
		collections: make(map[string]*core.Collection),
		byID:        make(map[int64]*core.Collection),
		permissions: make(map[string]*core.Permission),
	}
}

func collectionKey(tenantID int64, name string) string {
	return fmt.Sprintf("%d:%s", tenantID, name)
}

func permissionKey(tenantID, roleID, collectionID int64) string {
	return fmt.Sprintf("%d:%d:%d", tenantID, roleID, collectionID)
}

func (r *Registry) RegisterCollection(col *core.Collection) error {
	if col == nil {
		return fmt.Errorf("%w: collection cannot be nil", core.ErrValidation)
	}
	if err := core.ValidateIdentifier(col.Name); err != nil {
		return fmt.Errorf("%w: %s", core.ErrInvalidIdentifier, col.Name)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	key := collectionKey(col.TenantID, col.Name)
	r.collections[key] = col
	if col.ID != 0 {
		r.byID[col.ID] = col
	}
	return nil
}

func (r *Registry) UnregisterCollection(tenantID int64, name string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := collectionKey(tenantID, name)
	if col, exists := r.collections[key]; exists {
		delete(r.collections, key)
		if col.ID != 0 {
			delete(r.byID, col.ID)
		}
	}
}

func (r *Registry) GetCollection(tenantID int64, name string) (*core.Collection, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := collectionKey(tenantID, name)
	col, exists := r.collections[key]
	return col, exists
}

func (r *Registry) GetCollectionByID(id int64) (*core.Collection, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	col, exists := r.byID[id]
	return col, exists
}

func (r *Registry) RegisterPermission(perm *core.Permission) error {
	if perm == nil {
		return fmt.Errorf("%w: permission cannot be nil", core.ErrValidation)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	key := permissionKey(perm.TenantID, perm.RoleID, perm.CollectionID)
	r.permissions[key] = perm
	return nil
}

func (r *Registry) GetPermission(tenantID, roleID, collectionID int64) (*core.Permission, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := permissionKey(tenantID, roleID, collectionID)
	perm, exists := r.permissions[key]
	return perm, exists
}
