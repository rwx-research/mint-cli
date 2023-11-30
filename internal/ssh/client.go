package ssh

import (
	"os"

	tsize "github.com/kopoli/go-terminal-size"
	"github.com/pkg/errors"
	"golang.org/x/crypto/ssh"
)

type Client struct {
	*ssh.Client
}

func (c *Client) Connect(address string, config ssh.ClientConfig) (err error) {
	c.Client, err = ssh.Dial("tcp", address, &config)
	return
}

func (c *Client) Close() error {
	return c.Client.Close()
}

func (c *Client) InteractiveSession() error {
	session, err := c.Client.NewSession()
	if err != nil {
		return errors.Wrapf(err, "unable to start interactive debug session in Mint")
	}
	defer session.Close()

	session.Stdin = os.Stdin
	session.Stdout = os.Stdout
	session.Stderr = os.Stderr

	terminalSize, err := tsize.GetSize()
	if err != nil {
		return errors.Wrapf(err, "unable to determine terminal size")
	}

	if err := session.RequestPty(os.Getenv("TERM"), terminalSize.Height, terminalSize.Width, nil); err != nil {
		return errors.Wrapf(err, "unable to start PTY")
	}

	sizeChangeNotification, err := tsize.NewSizeListener()
	if err != nil {
		return errors.Wrapf(err, "unable to listen to terminal size changes")
	}
	defer sizeChangeNotification.Close()

	go func() {
		for size := range sizeChangeNotification.Change {
			_ = session.WindowChange(size.Height, size.Width)
		}
	}()

	if err := session.Shell(); err != nil {
		return errors.Wrapf(err, "unable to start shell")
	}

	// This is blocking
	if err := session.Wait(); err != nil {
		return errors.Wrapf(err, "connection was unexpectedly closed")
	}

	return nil
}
