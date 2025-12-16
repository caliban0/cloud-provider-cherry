// Package cherry implements the Cherry Servers infrastructure
// provisioning functionality that is required by the tests.
package cherry

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/cherryservers/cherrygo/v3"
)

type serverClient interface {
	Create(*cherrygo.CreateServer) (cherrygo.Server, *cherrygo.Response, error)
	Get(id int, opts *cherrygo.GetOptions) (cherrygo.Server, *cherrygo.Response, error)
	List(projectID int, opts *cherrygo.GetOptions) ([]cherrygo.Server, *cherrygo.Response, error)
	Update(id int, request *cherrygo.UpdateServer) (cherrygo.Server, *cherrygo.Response, error)
	Delete(id int) (cherrygo.Server, *cherrygo.Response, error)
}

type sshKeyClient interface {
	Create(*cherrygo.CreateSSHKey) (cherrygo.SSHKey, *cherrygo.Response, error)
	Delete(id int) (cherrygo.SSHKey, *cherrygo.Response, error)
}

type projectClient interface {
	Create(teamID int, request *cherrygo.CreateProject) (cherrygo.Project, *cherrygo.Response, error)
	Update(id int, request *cherrygo.UpdateProject) (cherrygo.Project, *cherrygo.Response, error)
	Get(id int, opts *cherrygo.GetOptions) (cherrygo.Project, *cherrygo.Response, error)
	Delete(id int) (*cherrygo.Response, error)
}

type ipClient interface {
	Create(id int, request *cherrygo.CreateIPAddress) (cherrygo.IPAddress, *cherrygo.Response, error)
	Get(id string, opts *cherrygo.GetOptions) (cherrygo.IPAddress, *cherrygo.Response, error)
	List(projectID int, opts *cherrygo.GetOptions) ([]cherrygo.IPAddress, *cherrygo.Response, error)
	Assign(id string, request *cherrygo.AssignIPAddress) (cherrygo.IPAddress, *cherrygo.Response, error)
}

// Client is an abstraction over the [github.com/cherryservers/cherrygo/v3]
// Cherry Servers API client library.
// It isolates Cherry Servers resource management from the test logic.
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

func (c Client) newTicker() *time.Ticker {
	jitter := time.Duration(rand.Intn(int(c.maxJitter.Milliseconds()))+1) * time.Millisecond
	return time.NewTicker(c.pollInterval + jitter)
}
