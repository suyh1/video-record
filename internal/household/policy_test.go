package household

import (
	"testing"

	"github.com/stretchr/testify/require"

	"video-record/internal/auth"
)

func TestPolicyKeepsPersonalRecordsPrivateWithoutAdminOverride(t *testing.T) {
	policy := Policy{}
	require.True(t, policy.CanManageMembers(auth.RoleAdmin))
	require.False(t, policy.CanManageMembers(auth.RoleMember))
	require.True(t, policy.CanMutatePersonalRecord("member-1", "member-1"))
	require.False(t, policy.CanMutatePersonalRecord("admin", "member-1"))
	require.True(t, policy.CanReadPrivateNote("member-1", "member-1"))
	require.False(t, policy.CanReadPrivateNote("admin", "member-1"))
}
