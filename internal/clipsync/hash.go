package clipsync

import (
	"crypto/sha256"
	"encoding/hex"
)

// hashText identifies clipboard content without keeping a second copy of it.
func hashText(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

// HashFor identifies clipboard text, so the PC can ignore the change its own
// clipboard write caused.
func HashFor(text string) string { return hashText(text) }
