package cherry

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/cherryservers/cherrygo/v3"
)

type Server struct {
	ID       int
	Hostname string
	PublicIP string
}

// Pseudo-constant for the server fields we want to get from the API.
var serverGetFields = []string{"id", "hostname", "ip_addresses", "address", "type", "state"}

// GetServer gets a server from Cherry Servers.
func (c Client) GetServer(id int) (Server, error) {
	srv, _, err := c.server.Get(id, &cherrygo.GetOptions{
		Fields: serverGetFields,
	})
	if err != nil {
		return Server{}, fmt.Errorf("couldn't get %d server: %w", id, err)
	}

	ip, ok := publicIP(srv.IPAddresses)
	if !ok {
		return Server{}, fmt.Errorf("server %d doesn't have a public IP", id)
	}

	s := Server{ID: srv.ID, Hostname: srv.Hostname, PublicIP: ip}

	return s, nil
}

type NewServerSpec struct {
	ProjectID int

	// ID of the public SSH key that should be added to this server.
	SSHKeyID string

	// Plan is a plan slug.
	Plan string

	// Region is a region slug.
	Region string

	// UserData is a base64 encoded cloud-config.
	UserData string
}

// ProvisionServer creates a server on Cherry Servers and waits for it to become active.
func (c Client) ProvisionServer(ctx context.Context, spec NewServerSpec) (Server, error) {
	sid, err := c.createServer(spec)
	if err != nil {
		return Server{}, err
	}

	s, err := c.untilServerActive(ctx, sid)
	if err != nil {
		return Server{}, fmt.Errorf("server %d didn't become active: %w", sid, err)
	}

	return s, nil
}

// createServer creates a server on Cherry Servers.
func (c Client) createServer(spec NewServerSpec) (int, error) {
	s, _, err := c.server.Create(&cherrygo.CreateServer{
		ProjectID: spec.ProjectID,
		SSHKeys:   []string{spec.SSHKeyID},
		Plan:      spec.Plan,
		Region:    spec.Region,
		UserData:  spec.UserData,
	})
	if err != nil {
		return 0, fmt.Errorf("couldn't create server: %w, with spec: %v", err, spec)
	}

	return s.ID, nil
}

// untilServerActive waits for a server to become active.
func (c Client) untilServerActive(ctx context.Context, id int) (Server, error) {
	jitter := time.Duration(rand.Intn(int(c.maxJitter.Milliseconds()))+1) * time.Millisecond
	t := time.NewTicker(c.pollInterval + jitter)

	for {
		select {
		case <-ctx.Done():
			t.Stop()
			return Server{}, ctx.Err()
		case <-t.C:
			// Server might not have all fields set yet, so we can't use GetServer.
			s, _, err := c.server.Get(id, &cherrygo.GetOptions{Fields: serverGetFields})
			if err != nil {
				return Server{}, fmt.Errorf("couldn't get server %d: %w", id, err)
			}

			if s.State == "active" {
				ip, ok := publicIP(s.IPAddresses)
				if !ok {
					return Server{}, fmt.Errorf("server %d doesn't have a public IP", id)
				}
				return Server{ID: s.ID, Hostname: s.Hostname, PublicIP: ip}, nil
			}
		}
	}
}

// publicIP finds any public IP, if one exists in the slice.
func publicIP(a []cherrygo.IPAddress) (string, bool) {
	for i := range a {
		if a[i].Type == "primary-ip" {
			return a[i].Address, true
		}
	}
	return "", false
}
