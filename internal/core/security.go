package core

type Role struct {
	ID          int64  `json:"-"`
	TenantID    int64  `json:"-"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type Action string

const (
	ActionCreate Action = "create"
	ActionRead   Action = "read"
	ActionUpdate Action = "update"
	ActionDelete Action = "delete"
)

type Permission struct {
	ID            int64    `json:"-"`
	TenantID      int64    `json:"-"`
	RoleID        int64    `json:"-"`
	CollectionID  int64    `json:"-"`
	Actions       []Action `json:"actions"`
	AllowedFields []string `json:"allowed_fields"`
	RLSFilter     string   `json:"rls_filter,omitempty"`
}

func (p *Permission) AllowsAction(act Action) bool {
	for _, a := range p.Actions {
		if a == act || a == "*" {
			return true
		}
	}
	return false
}

func (p *Permission) AllowsField(field string) bool {
	for _, f := range p.AllowedFields {
		if f == "*" || f == field {
			return true
		}
	}
	return false
}

type SecurityContext struct {
	TenantID      int64
	UserID        int64
	RoleID        int64
	RoleName      string
	IsSuperAdmin  bool
	AllowedFields []string
	RLSFilter     string
}
