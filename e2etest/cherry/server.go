package cherry

import (
	"context"
	"fmt"

	"github.com/cherryservers/cherrygo/v3"
)

type Server struct {
	ID       int
	Hostname string

	// Region slug.
	Region string

	// Plan slug.
	Plan      string
	PublicIP  string
	PrivateIP string
	ServerBGP
}

type ServerBGP struct {
	PeerASN       int
	PeerAddresses []string
	Enabled       bool
}

// serverFrom builds a Server from a [github.com/cherryservers/cherrygo/v3] server.
func serverFrom(sg cherrygo.Server) (Server, error) {
	pub, priv, err := serverIPs(sg)
	if err != nil {
		return Server{}, err
	}

	s := Server{ID: sg.ID,
		Hostname:  sg.Hostname,
		Region:    sg.Region.Slug,
		Plan:      sg.Plan.Slug,
		PublicIP:  pub,
		PrivateIP: priv,
		ServerBGP: ServerBGP{
			PeerASN:       sg.Region.BGP.Asn,
			PeerAddresses: sg.Region.BGP.Hosts,
			Enabled:       sg.BGP.Enabled,
		}}

	return s, nil
}

// Pseudo-constant for the server fields we want to get from the API.
var serverGetFields = []string{"id", "hostname", "ip_addresses",
	"address", "type", "state", "region", "plan", "bgp"}

// GetServer gets a server from Cherry Servers.
func (c Client) GetServer(id int) (Server, error) {
	srv, _, err := c.server.Get(id, &cherrygo.GetOptions{
		Fields: serverGetFields,
	})
	if err != nil {
		return Server{}, fmt.Errorf("couldn't get %d server: %w", id, err)
	}

	s, err := serverFrom(srv)
	if err != nil {
		return s, fmt.Errorf("couldn't create a server object from a cherrygo server: %w", err)
	}

	return s, nil
}

// ListServers lists all servers that belong to a Cherry Servers project.
func (c Client) ListServers(projectID int) ([]Server, error) {
	srv, _, err := c.server.List(projectID, &cherrygo.GetOptions{Fields: serverGetFields})
	if err != nil {
		return nil, fmt.Errorf("couldn't list servers for project %d: %w", projectID, err)
	}
	s := make([]Server, len(srv))
	for i := range srv {
		fs, err := serverFrom(srv[i])
		if err != nil {
			return nil, fmt.Errorf("couldn't create a server object from a cherrygo server: %w", err)
		}
		s[i] = fs
	}
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
	t := c.newTicker()

	for {
		select {
		case <-ctx.Done():
			return Server{}, ctx.Err()
		case <-t.C:
			// Server might not have all fields set yet, so we can't use GetServer.
			srv, _, err := c.server.Get(id, &cherrygo.GetOptions{Fields: serverGetFields})
			if err != nil {
				return Server{}, fmt.Errorf("couldn't get server %d: %w", id, err)
			}

			if srv.State == "active" {
				s, err := serverFrom(srv)
				if err != nil {
					return s, fmt.Errorf("couldn't create a server object from a cherrygo server: %w", err)
				}
				return s, nil
			}
		}
	}
}

// DeleteServer deletes a server on Cherry Servers.
func (c Client) DeleteServer(id int) error {
	_, _, err := c.server.Delete(id)
	if err != nil {
		return fmt.Errorf("couldn't delete server %d", id)
	}
	return nil
}

type ServerUpdateSpec struct {
	BGPEnabled bool
}

// UpdateServer updates a server on Cherry Servers.
// Does a GET request after the update, because the response
// from an update request doesn't contain all the expected fields.
func (c Client) UpdateServer(id int, spec ServerUpdateSpec) (Server, error) {
	_, _, err := c.server.Update(id, &cherrygo.UpdateServer{Bgp: spec.BGPEnabled})
	if err != nil {
		return Server{}, fmt.Errorf("couldn't update server %d, with update spec %v: %w", id, spec, err)
	}

	return c.GetServer(id)
}

// serverIPs gets a server's public and private IP,
// if it has them.
func serverIPs(s cherrygo.Server) (pub, priv string, err error) {
	pub, priv, ok := publicPrivate(s.IPAddresses)
	if !ok {
		if pub == "" && priv == "" {
			return pub, priv, fmt.Errorf("server %d has no public or private ip", s.ID)
		}
		if pub == "" {
			return pub, priv, fmt.Errorf("server %d has no public ip", s.ID)
		}
		if priv == "" {
			return pub, priv, fmt.Errorf("server %d has no private ip", s.ID)
		}
	}
	return pub, priv, nil
}

// publicPrivate finds any tuple of public and private IP addresses,
// if they exist in the slice.
func publicPrivate(a []cherrygo.IPAddress) (pub, priv string, ok bool) {
	for i := range a {
		switch a[i].Type {
		case "primary-ip":
			pub = a[i].Address
		case "private-ip":
			priv = a[i].Address
		}
	}
	return pub, priv, pub != "" && priv != ""
}
