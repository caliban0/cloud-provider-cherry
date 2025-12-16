package cherry

import (
	"fmt"

	"github.com/cherryservers/cherrygo/v3"
)

type SSHKey struct {
	ID int
}

type sshKeyClient interface {
	Create(*cherrygo.CreateSSHKey) (cherrygo.SSHKey, *cherrygo.Response, error)
	Delete(id int) (cherrygo.SSHKey, *cherrygo.Response, error)
}

type SSHKeyClient struct {
	c sshKeyClient
}

func newSSHKeyClient(c sshKeyClient) SSHKeyClient {
	return SSHKeyClient{c: c}
}

type NewSSHKeySpec struct {
	// Label is a name for the key.
	Label     string
	PublicKey string
}

// Create creates a new SSH key on Cherry Servers.
func (c SSHKeyClient) Create(spec NewSSHKeySpec) (SSHKey, error) {
	k, _, err := c.c.Create(&cherrygo.CreateSSHKey{
		Label: spec.Label,
		Key:   spec.PublicKey,
	})
	return SSHKey{ID: k.ID}, err
}

// Delete deletes an SSH key from Cherry Servers.
func (c SSHKeyClient) Delete(id int) error {
	_, _, err := c.c.Delete(id)
	if err != nil {
		return fmt.Errorf("failed to delete ssh key %d: %w", id, err)
	}
	return nil
}
