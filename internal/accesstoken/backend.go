package accesstoken

type Backend interface {
	Get() (string, error)
	Set(token string) error
}
