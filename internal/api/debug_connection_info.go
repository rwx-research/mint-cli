package api

type DebugConnectionInfo struct {
	Debuggable     bool
	Address        string
	PublicHostKey  string `json:"public_host_key"`
	PrivateUserKey string `json:"private_user_key"`
}

type DebugConnectionInfoError struct {
	Error string
}
