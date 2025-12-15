package cherry

import (
	"fmt"

	"github.com/cherryservers/cherrygo/v3"
)

// Cherry Servers floating IP address.
type FIP struct {
	ID             string
	Address        string
	targetHostname string
}

// TargetHostname returns the hostname of the server the FIP is assigned to,
// if the FIP is assigned to a server.
func (i FIP) TargetHostname() (string, bool) {
	return i.targetHostname, i.targetHostname != ""
}

type NewFIPSpec struct {
	// Region slug.
	Region string

	ProjectID int
	Tags      map[string]string
}

// CreateFIP creates an FIP address on Cherry Servers.
func (c Client) CreateFIP(spec NewFIPSpec) (FIP, error) {
	ip, _, err := c.ip.Create(spec.ProjectID, &cherrygo.CreateIPAddress{
		Region: spec.Region, Tags: &spec.Tags,
	})
	if err != nil {
		return FIP{}, err
	}
	return FIP{ID: ip.ID, Address: ip.Address}, nil
}

// Pseudo-constant for the IP fields we want to get from the API.
var ipGetFields = []string{"id", "address", "targeted_to", "hostname"}

// GetFIP gets a FIP from Cherry Servers.
func (c Client) GetFIP(id string) (FIP, error) {
	ip, _, err := c.ip.Get(id, &cherrygo.GetOptions{Fields: ipGetFields})
	if err != nil {
		return FIP{}, fmt.Errorf("couldn't get FIP %q", id)
	}
	return FIP{ID: ip.ID, Address: ip.Address, targetHostname: ip.TargetedTo.Hostname}, nil
}

// AssignFIP assigns a FIP to a server.
func (c Client) AssignFIP(ipID string, serverID int) (FIP, error) {
	ip, _, err := c.ip.Assign(ipID, &cherrygo.AssignIPAddress{ServerID: serverID})
	if err != nil {
		return FIP{}, fmt.Errorf("couldn't assign IP %q to server %d: %w", ipID, serverID, err)
	}
	return FIP{ID: ipID, Address: ip.Address, targetHostname: ip.TargetedTo.Hostname}, nil
}
