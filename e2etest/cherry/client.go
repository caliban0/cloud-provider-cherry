// Package cherry implements the Cherry Servers infrastructure
// provisioning functionality that is required by the tests.
package cherry

import (
	"fmt"
	"time"

	"github.com/cherryservers/cherrygo/v3"
)

type serverClient interface {
	Create(*cherrygo.CreateServer) (cherrygo.Server, *cherrygo.Response, error)
	Get(int, *cherrygo.GetOptions) (cherrygo.Server, *cherrygo.Response, error)
}

type Client struct {
	server       serverClient
	maxJitter    time.Duration
	pollInterval time.Duration
}

func NewClient(authToken string) (Client, error) {
	const (
		defaultMaxJitter    = time.Second * 1
		defaultPollInterval = time.Second * 10
	)

	c, err := cherrygo.NewClient(cherrygo.WithAuthToken(authToken))
	if err != nil {
		return Client{}, fmt.Errorf("couldn't create cherrygo client: %w", err)
	}

	return Client{server: c.Servers,
		maxJitter:    defaultMaxJitter,
		pollInterval: defaultPollInterval}, nil
}
