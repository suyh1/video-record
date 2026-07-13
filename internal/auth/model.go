package auth

import "time"

type Role string

const (
	RoleAdmin  Role = "admin"
	RoleMember Role = "member"
)

type User struct {
	ID        string
	Username  string
	Role      Role
	Active    bool
	CreatedAt time.Time
}

type Session struct {
	ID         string
	UserID     string
	Token      string
	CSRFToken  string
	ExpiresAt  time.Time
	LastSeenAt time.Time
}

type Identity struct {
	SessionID     string
	User          User
	CSRFTokenHash []byte
	ExpiresAt     time.Time
	LastSeenAt    time.Time
}
