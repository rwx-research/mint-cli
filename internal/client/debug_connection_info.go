package client

type DebugConnectionInfo struct {
	Address        string
	PublicHostKey  string `json:"public_host_key"`
	PrivateUserKey string `json:"private_user_key"`
}
