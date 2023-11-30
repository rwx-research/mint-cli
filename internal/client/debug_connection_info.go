package client

import (
	"crypto/ed25519"
)

type DebugConnectionInfo struct {
	Address        string
	PublicHostKey  ed25519.PublicKey
	PrivateUserKey []byte
}
