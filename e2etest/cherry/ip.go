package cherry

import (
	"fmt"

	"github.com/cherryservers/cherrygo/v3"
)

// Cherry Servers IP address.
type IP struct {
	ID             string
	Address        string
	Tags           map[string]string
	targetHostname string
}

// TargetHostname returns the hostname of the server the IP is assigned to,
// if the IP is assigned to a server.
func (i IP) TargetHostname() (string, bool) {
	return i.targetHostname, i.targetHostname != ""
}

// ipFrom builds an IP from a cherrygo IPAddress.
func ipFrom(ip cherrygo.IPAddress) IP {
	return IP{
		ID:             ip.ID,
		Address:        ip.Address,
		targetHostname: ip.TargetedTo.Hostname,
		Tags:           *ip.Tags}
}

type ipClient interface {
	Create(id int, request *cherrygo.CreateIPAddress) (cherrygo.IPAddress, *cherrygo.Response, error)
	Get(id string, opts *cherrygo.GetOptions) (cherrygo.IPAddress, *cherrygo.Response, error)
	List(projectID int, opts *cherrygo.GetOptions) ([]cherrygo.IPAddress, *cherrygo.Response, error)
	Assign(id string, request *cherrygo.AssignIPAddress) (cherrygo.IPAddress, *cherrygo.Response, error)
}

type IPClient struct {
	c ipClient
}

func NewIPClient(c ipClient) IPClient {
	return IPClient{c: c}
}

type NewIPSpec struct {
	// Region slug.
	Region string

	ProjectID int
	Tags      map[string]string
}

// Create creates a floating IP address on Cherry Servers.
func (c IPClient) Create(spec NewIPSpec) (IP, error) {
	ip, _, err := c.c.Create(spec.ProjectID, &cherrygo.CreateIPAddress{
		Region: spec.Region, Tags: &spec.Tags,
	})
	if err != nil {
		return IP{}, err
	}
	return ipFrom(ip), nil
}

// Pseudo-constant for the IP fields we want to get from the API.
var ipGetFields = []string{"id", "address", "targeted_to", "hostname", "tags"}

// Get gets an IP from Cherry Servers.
func (c IPClient) Get(id string) (IP, error) {
	ip, _, err := c.c.Get(id, &cherrygo.GetOptions{Fields: ipGetFields})
	if err != nil {
		return IP{}, fmt.Errorf("couldn't get IP %q", id)
	}
	return ipFrom(ip), nil
}

// List retrieves a list of project IP addresses.
func (c IPClient) List(projectID int) ([]IP, error) {
	r, _, err := c.c.List(projectID, &cherrygo.GetOptions{Fields: ipGetFields})
	if err != nil {
		return nil, fmt.Errorf("couldn't list ips for project %d: %w", projectID, err)
	}

	ips := make([]IP, len(r))
	for i := range r {
		ips[i] = ipFrom(r[i])
	}

	return ips, nil
}

// Assign assigns an IP to a server.
func (c IPClient) Assign(ipID string, serverID int) (IP, error) {
	ip, _, err := c.c.Assign(ipID, &cherrygo.AssignIPAddress{ServerID: serverID})
	if err != nil {
		return IP{}, fmt.Errorf("couldn't assign IP %q to server %d: %w", ipID, serverID, err)
	}
	return IP{ID: ipID, Address: ip.Address, targetHostname: ip.TargetedTo.Hostname, Tags: *ip.Tags}, nil
}
