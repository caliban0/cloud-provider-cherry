// Package cherry implements the Cherry Servers infrastructure
// provisioning functionality that is required by the tests.
package cherry

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/cherryservers/cherrygo/v3"
)

// Client is an abstraction over the [github.com/cherryservers/cherrygo/v3]
// Cherry Servers API client library.
// It isolates Cherry Servers resource management from the test logic.
type Client struct {
	IP      IPClient
	Project ProjectClient
	Server  ServerClient
	SSHKey  SSHKeyClient
	Plan    PlanClient
}

// NewClient creates a new Cherry Servers API client.
// Request polling interval is set to a default of 10 seconds
// and jitter of up to 1 second is added.
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
		Server:  newServerClient(c.Servers, tf),
		SSHKey:  newSSHKeyClient(c.SSHKeys),
		Project: newProjectClient(c.Projects, c.Servers, tf),
		IP:      newIPClient(c.IPAddresses),
		Plan:    newPlanClient(c.Plans),
	}, nil
}

type tickerFactory struct {
	maxJitter    time.Duration
	pollInterval time.Duration
}

func (t tickerFactory) newTicker() *time.Ticker {
	jitter := time.Duration(rand.Intn(int(t.maxJitter.Milliseconds()))+1) * time.Millisecond
	return time.NewTicker(t.pollInterval + jitter)
}
