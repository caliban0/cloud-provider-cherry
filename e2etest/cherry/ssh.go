package cherry

import (
	"fmt"

	"github.com/cherryservers/cherrygo/v3"
)

type SSHKey struct {
	ID int
}

type NewSSHKeySpec struct {
	// Label is a unique human-readable identifier for the key.
	Label     string
	PublicKey string
}

// CreateSSHKey creates a new SSH key on Cherry Servers.
func (c Client) CreateSSHKey(spec NewSSHKeySpec) (SSHKey, error) {
	k, _, err := c.sshKey.Create(&cherrygo.CreateSSHKey{
		Label: spec.Label,
		Key:   spec.PublicKey,
	})
	return SSHKey{ID: k.ID}, err
}

// DeleteSSHKey deletes an SSH key from Cherry Servers.
func (c Client) DeleteSSHKey(id int) error {
	_, _, err := c.sshKey.Delete(id)
	if err != nil {
		return fmt.Errorf("failed to delete ssh key %d: %w", id, err)
	}
	return nil
}
