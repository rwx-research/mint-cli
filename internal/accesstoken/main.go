package accesstoken

func Set(backend Backend, token string) error {
	return backend.Set(token)
}

func Get(backend Backend, provided string) (string, error) {
	if provided != "" {
		return provided, nil
	}

	return backend.Get()
}
