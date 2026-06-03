package domain

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
)

type EntryID string
type RunID string
type ActorRef string

func (id EntryID) String() string { return string(id) }
func (id RunID) String() string   { return string(id) }

func NewEntryID() (EntryID, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate schedule entry ID entropy: %w", err)
	}
	return EntryID("schedule-" + hex.EncodeToString(b[:])), nil
}

func NewRunID() (RunID, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate schedule run ID entropy: %w", err)
	}
	return RunID("schedule-run-" + hex.EncodeToString(b[:])), nil
}

func normalizeEntryID(id EntryID) EntryID {
	return EntryID(strings.TrimSpace(string(id)))
}

func normalizeRunID(id RunID) RunID {
	return RunID(strings.TrimSpace(string(id)))
}

func normalizeActorRef(ref ActorRef) ActorRef {
	return ActorRef(strings.TrimSpace(string(ref)))
}
