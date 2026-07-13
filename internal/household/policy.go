package household

import "video-record/internal/auth"

type Policy struct{}

func (Policy) CanManageMembers(role auth.Role) bool {
	return role == auth.RoleAdmin
}

func (Policy) CanMutatePersonalRecord(actorID, ownerID string) bool {
	return actorID != "" && actorID == ownerID
}

func (Policy) CanReadPrivateNote(viewerID, ownerID string) bool {
	return viewerID != "" && viewerID == ownerID
}
