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

type NewIPSpec struct {
	// Region slug.
	Region string

	ProjectID int
	Tags      map[string]string
}

// CreateIP creates a floating IP address on Cherry Servers.
func (c Client) CreateIP(spec NewIPSpec) (IP, error) {
	ip, _, err := c.ip.Create(spec.ProjectID, &cherrygo.CreateIPAddress{
		Region: spec.Region, Tags: &spec.Tags,
	})
	if err != nil {
		return IP{}, err
	}
	return ipFrom(ip), nil
}

// Pseudo-constant for the IP fields we want to get from the API.
var ipGetFields = []string{"id", "address", "targeted_to", "hostname", "tags"}

// GetIP gets an IP from Cherry Servers.
func (c Client) GetIP(id string) (IP, error) {
	ip, _, err := c.ip.Get(id, &cherrygo.GetOptions{Fields: ipGetFields})
	if err != nil {
		return IP{}, fmt.Errorf("couldn't get IP %q", id)
	}
	return ipFrom(ip), nil
}

// ListIPs retrieves a list of project IP addresses.
func (c Client) ListIPs(projectID int) ([]IP, error) {
	r, _, err := c.ip.List(projectID, &cherrygo.GetOptions{Fields: ipGetFields})
	if err != nil {
		return nil, fmt.Errorf("couldn't list ips for project %d: %w", projectID, err)
	}

	ips := make([]IP, len(r))
	for i := range r {
		ips[i] = ipFrom(r[i])
	}

	return ips, nil
}

// AssignIP assigns an IP to a server.
func (c Client) AssignIP(ipID string, serverID int) (IP, error) {
	ip, _, err := c.ip.Assign(ipID, &cherrygo.AssignIPAddress{ServerID: serverID})
	if err != nil {
		return IP{}, fmt.Errorf("couldn't assign IP %q to server %d: %w", ipID, serverID, err)
	}
	return IP{ID: ipID, Address: ip.Address, targetHostname: ip.TargetedTo.Hostname, Tags: *ip.Tags}, nil
}
