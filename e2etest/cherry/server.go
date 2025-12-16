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

type serverClient interface {
	Create(*cherrygo.CreateServer) (cherrygo.Server, *cherrygo.Response, error)
	Get(id int, opts *cherrygo.GetOptions) (cherrygo.Server, *cherrygo.Response, error)
	List(projectID int, opts *cherrygo.GetOptions) ([]cherrygo.Server, *cherrygo.Response, error)
	Update(id int, request *cherrygo.UpdateServer) (cherrygo.Server, *cherrygo.Response, error)
	Delete(id int) (cherrygo.Server, *cherrygo.Response, error)
}

type ServerClient struct {
	c      serverClient
	ticker tickerFactory
}

func NewServerClient(c serverClient, t tickerFactory) ServerClient {
	return ServerClient{c: c, ticker: t}
}

// Pseudo-constant for the server fields we want to get from the API.
var serverGetFields = []string{"id", "hostname", "ip_addresses",
	"address", "type", "state", "region", "plan", "bgp"}

// Get gets a server from Cherry Servers.
func (c ServerClient) Get(id int) (Server, error) {
	srv, _, err := c.c.Get(id, &cherrygo.GetOptions{
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

// List lists all servers that belong to a Cherry Servers project.
func (c ServerClient) List(projectID int) ([]Server, error) {
	srv, _, err := c.c.List(projectID, &cherrygo.GetOptions{Fields: serverGetFields})
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

// Provision creates a server on Cherry Servers and waits for it to become active.
func (c ServerClient) Provision(ctx context.Context, spec NewServerSpec) (Server, error) {
	sid, err := c.create(spec)
	if err != nil {
		return Server{}, err
	}

	s, err := c.untilActive(ctx, sid)
	if err != nil {
		return Server{}, fmt.Errorf("server %d didn't become active: %w", sid, err)
	}

	return s, nil
}

// create creates a server on Cherry Servers.
func (c ServerClient) create(spec NewServerSpec) (int, error) {
	s, _, err := c.c.Create(&cherrygo.CreateServer{
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

// untilActive waits for a server to become active.
func (c ServerClient) untilActive(ctx context.Context, id int) (Server, error) {
	t := c.ticker.newTicker()

	for {
		select {
		case <-ctx.Done():
			return Server{}, ctx.Err()
		case <-t.C:
			// Server might not have all fields set yet, so we can't use GetServer.
			srv, _, err := c.c.Get(id, &cherrygo.GetOptions{Fields: serverGetFields})
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

// Delete deletes a server on Cherry Servers.
func (c ServerClient) Delete(id int) error {
	_, _, err := c.c.Delete(id)
	if err != nil {
		return fmt.Errorf("couldn't delete server %d", id)
	}
	return nil
}

type ServerUpdateSpec struct {
	BGPEnabled bool
}

// Update updates a server on Cherry Servers.
// Does a GET request after the update, because the response
// from an update request doesn't contain all the expected fields.
func (c ServerClient) Update(id int, spec ServerUpdateSpec) (Server, error) {
	_, _, err := c.c.Update(id, &cherrygo.UpdateServer{Bgp: spec.BGPEnabled})
	if err != nil {
		return Server{}, fmt.Errorf("couldn't update server %d, with update spec %v: %w", id, spec, err)
	}

	return c.Get(id)
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
