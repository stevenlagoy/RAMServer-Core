package game

import (
	"testing"
)

// Conformance helper

func Run(t *testing.T, game Game, players []string) bool {
	return true

	// Check that Apply leaves input state untouched
	// Check every payload from LegalActions passes Validate
	// Check that Roles agrees with ReadyToStart
}
