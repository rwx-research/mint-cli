package accesstoken

type MemoryBackend struct {
	Token string
}

func NewMemoryBackend() (*MemoryBackend, error) {
	return &MemoryBackend{}, nil
}

func (f *MemoryBackend) Get() (string, error) {
	return f.Token, nil
}

func (f *MemoryBackend) Set(value string) error {
	f.Token = value
	return nil
}
