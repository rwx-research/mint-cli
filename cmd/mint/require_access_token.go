package main

import (
	"github.com/pkg/errors"
	"github.com/rwx-research/mint-cli/internal/accesstoken"
)

func requireAccessToken() error {
	token, err := accesstoken.Get(accessTokenBackend, AccessToken)
	if err == nil && token != "" {
		return nil
	}

	return errors.New(
		"You're trying to use a command which requires authentication with RWX Cloud, " +
			"but you do not have an access token configured.\n\n" +
			"To use this command, you can authenticate with RWX Cloud via the `mint login` command, or " +
			"you can supply the `--access-token` option or `RWX_ACCESS_TOKEN` environment variable.\n\n" +
			"Once you do so, go ahead and run the command again.",
	)
}
