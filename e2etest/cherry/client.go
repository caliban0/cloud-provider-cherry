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

type sshKeyClient interface {
	Create(request *cherrygo.CreateSSHKey) (cherrygo.SSHKey, *cherrygo.Response, error)
	Delete(sshKeyID int) (cherrygo.SSHKey, *cherrygo.Response, error)
}

type projectClient interface {
	Delete(projectID int) (*cherrygo.Response, error)
}

// Client is an abstraction over the [github.com/cherryservers/cherrygo/v3]
// Cherry Servers API client library.
// It isolates Cherry Servers resource management from test logic.
type Client struct {
	server  serverClient
	sshKey  sshKeyClient
	project projectClient

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
		sshKey:       c.SSHKeys,
		project:      c.Projects,
		maxJitter:    defaultMaxJitter,
		pollInterval: defaultPollInterval}, nil
}
