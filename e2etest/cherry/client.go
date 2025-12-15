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
	Get(id int, opt *cherrygo.GetOptions) (cherrygo.Server, *cherrygo.Response, error)
	Delete(id int) (cherrygo.Server, *cherrygo.Response, error)
}

type sshKeyClient interface {
	Create(*cherrygo.CreateSSHKey) (cherrygo.SSHKey, *cherrygo.Response, error)
	Delete(id int) (cherrygo.SSHKey, *cherrygo.Response, error)
}

type projectClient interface {
	Delete(id int) (*cherrygo.Response, error)
}

type ipClient interface {
	Create(projectID int, request *cherrygo.CreateIPAddress) (cherrygo.IPAddress, *cherrygo.Response, error)
	Get(id string, opts *cherrygo.GetOptions) (cherrygo.IPAddress, *cherrygo.Response, error)
	Assign(id string, request *cherrygo.AssignIPAddress) (cherrygo.IPAddress, *cherrygo.Response, error)
}

// Client is an abstraction over the [github.com/cherryservers/cherrygo/v3]
// Cherry Servers API client library.
// It isolates Cherry Servers resource management from test logic.
type Client struct {
	server  serverClient
	sshKey  sshKeyClient
	project projectClient
	ip      ipClient

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
		ip:           c.IPAddresses,
		maxJitter:    defaultMaxJitter,
		pollInterval: defaultPollInterval}, nil
}
