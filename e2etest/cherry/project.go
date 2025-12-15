package cherry

import "fmt"

// DeleteProject deletes a project from Cherry Servers.
// This also deletes all resources associated with that project.
func (c Client) DeleteProject(id int) error {
	_, err := c.project.Delete(id)
	if err != nil {
		return fmt.Errorf("failed to delete project %d: %w", id, err)
	}
	return nil
}
