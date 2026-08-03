package room

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// NormalizeParticipantIdentity validates client-supplied join identifiers before state changes.
func NormalizeParticipantIdentity(playerID, playerName string) (string, string, error) {
	playerID = strings.TrimSpace(playerID)
	if playerID == "" {
		return "", "", ErrPlayerIDRequired
	}
	if len(playerID) > MaxPlayerIDBytes {
		return "", "", ErrInvalidPlayerID
	}
	for _, character := range playerID {
		if !((character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			character == '-' || character == '_') {
			return "", "", ErrInvalidPlayerID
		}
	}

	playerName = strings.TrimSpace(playerName)
	if playerName == "" {
		return "", "", ErrPlayerNameRequired
	}
	if !utf8.ValidString(playerName) {
		return "", "", ErrInvalidPlayerName
	}
	if len(playerName) > MaxPlayerNameBytes || utf8.RuneCountInString(playerName) > MaxPlayerNameRunes {
		return "", "", ErrPlayerNameTooLong
	}
	for _, character := range playerName {
		if unicode.IsControl(character) {
			return "", "", ErrInvalidPlayerName
		}
	}

	return playerID, playerName, nil
}
