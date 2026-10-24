package game

// Identifies a participant's function in a session. Could include roles like
// 'player', 'spectator', 'dealer', 'banker', etc. Games declare supported
// roles with Game.Roles.
type Role string

// Common reusable roles
const (
	RolePlayer    Role = "player"
	RoleSpectator Role = "spectator"
)

type RoleSpec struct {
	Name Role
	// Bound how many members may hold this role. Max == 0 means unlimited
	Min, Max int
}
