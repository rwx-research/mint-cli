package mocks

import (
	"golang.org/x/crypto/ssh"

	"github.com/pkg/errors"
)

type SSH struct {
	MockConnect            func(addr string, cfg ssh.ClientConfig) error
	MockInteractiveSession func() error
}

func (s *SSH) Close() error {
	return nil
}

func (s *SSH) Connect(addr string, cfg ssh.ClientConfig) error {
	if s.MockConnect != nil {
		return s.MockConnect(addr, cfg)
	}

	return errors.New("MockConnect was not configured")
}

func (s *SSH) InteractiveSession() error {
	if s.MockInteractiveSession != nil {
		return s.MockInteractiveSession()
	}

	return errors.New("MockInteractiveSession was not configured")
}
