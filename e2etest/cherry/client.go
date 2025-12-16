// Package cherry implements the Cherry Servers infrastructure
// provisioning functionality that is required by the tests.
package cherry

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/cherryservers/cherrygo/v3"
)

type sshKeyClient interface {
	Create(*cherrygo.CreateSSHKey) (cherrygo.SSHKey, *cherrygo.Response, error)
	Delete(id int) (cherrygo.SSHKey, *cherrygo.Response, error)
}

// Client is an abstraction over the [github.com/cherryservers/cherrygo/v3]
// Cherry Servers API client library.
// It isolates Cherry Servers resource management from the test logic.
type Client struct {
	IP      IPClient
	Project ProjectClient
	Server  ServerClient

	sshKey sshKeyClient

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

	tf := tickerFactory{maxJitter: defaultMaxJitter, pollInterval: defaultPollInterval}

	return Client{
		Server:       NewServerClient(c.Servers, tf),
		sshKey:       c.SSHKeys,
		Project:      NewProjectClient(c.Projects, c.Servers, tf),
		IP:           NewIPClient(c.IPAddresses),
		maxJitter:    defaultMaxJitter,
		pollInterval: defaultPollInterval}, nil
}

type tickerFactory struct {
	maxJitter    time.Duration
	pollInterval time.Duration
}

func (t tickerFactory) newTicker() *time.Ticker {
	jitter := time.Duration(rand.Intn(int(t.maxJitter.Milliseconds()))+1) * time.Millisecond
	return time.NewTicker(t.pollInterval + jitter)
}

func (c Client) newTicker() *time.Ticker {
	jitter := time.Duration(rand.Intn(int(c.maxJitter.Milliseconds()))+1) * time.Millisecond
	return time.NewTicker(c.pollInterval + jitter)
}
