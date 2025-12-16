package cherry

import (
	"context"
	"fmt"

	"github.com/cherryservers/cherrygo/v3"
)

type Project struct {
	ID         int
	BGPEnabled bool
	Name       string
	localASN   int
}

// LocalASN returns the project network ASN, if the project has one.
func (p Project) LocalASN() (int, bool) {
	return p.localASN, p.localASN != 0
}

// projectFrom builds a Project from a cherrygo Project.
func projectFrom(p cherrygo.Project) Project {
	return Project{
		ID:         p.ID,
		BGPEnabled: p.Bgp.Enabled,
		localASN:   p.Bgp.LocalASN,
		Name:       p.Name}
}

// Pseudo-constant for the project fields we want to get from the API.
var projectGetFields = []string{"id", "bgp", "name"}

// GetProject gets a project from Cherry Servers.
func (c Client) GetProject(id int) (Project, error) {
	p, _, err := c.project.Get(id, &cherrygo.GetOptions{Fields: projectGetFields})
	if err != nil {
		return Project{}, fmt.Errorf("couldn't get project %d: %w", id, err)
	}
	return projectFrom(p), nil
}

type NewProjectSpec struct {
	TeamID int
	Name   string
}

func (c Client) CreateProject(spec NewProjectSpec) (Project, error) {
	p, _, err := c.project.Create(spec.TeamID, &cherrygo.CreateProject{Name: spec.Name})
	if err != nil {
		return Project{}, fmt.Errorf("couldn't create project with spec %v: %w", spec, err)
	}
	return projectFrom(p), nil
}

// DeleteProject deletes a project from Cherry Servers.
// This also deletes all resources associated with that project.
func (c Client) DeleteProject(id int) error {
	_, err := c.project.Delete(id)
	if err != nil {
		return fmt.Errorf("failed to delete project %d: %w", id, err)
	}
	return nil
}

type ProjectUpdateSpec struct {
	BGPEnabled bool
}

// UpdateProject updates a project on Cherry Servers.
func (c Client) UpdateProject(id int, spec ProjectUpdateSpec) (Project, error) {
	p, _, err := c.project.Update(id, &cherrygo.UpdateProject{Bgp: &spec.BGPEnabled})
	if err != nil {
		return Project{}, fmt.Errorf("couldn't update project %d: %w", id, err)
	}
	return projectFrom(p), nil
}

// ForceProjectASN works around API limitations to ensure that
// a project has a local ASN.
//
// As of this writing, enabling BGP for a project does not
// create a local ASN for it. Only after BGP is enabled for a server
// from that project, does the project get a local ASN.
//
// To work around this, ForceProjectASN picks a project's server,
// enables BGP for it, waits for the project to get a local ASN and
// then disables BGP for that server. The project then retains that ASN.
//
// If the project already has a local ASN, does nothing.
func (c Client) ForceProjectASN(ctx context.Context, p Project) (Project, error) {
	if _, ok := p.LocalASN(); ok {
		return p, nil
	}

	servers, err := c.ListServers(p.ID)
	if err != nil {
		return Project{}, err
	}

	if len(servers) < 1 {
		return Project{}, fmt.Errorf("to force an asn, project %d must have at least one server", p.ID)
	}
	s := servers[0]

	return c.forceProjectASN(ctx, p, s.ID)
}

func (c Client) forceProjectASN(ctx context.Context, p Project, serverID int) (Project, error) {
	if !p.BGPEnabled {
		_, err := c.UpdateProject(p.ID, ProjectUpdateSpec{BGPEnabled: true})
		if err != nil {
			return Project{}, fmt.Errorf("couldn't enable BGP for project: %w", err)
		}
	}

	_, err := c.UpdateServer(serverID, ServerUpdateSpec{BGPEnabled: true})
	if err != nil {
		return Project{}, fmt.Errorf("couldn't enable BGP for server %d: %w", serverID, err)
	}

	p, err = c.waitUntilProjectHasASN(ctx, p)
	if err != nil {
		return Project{}, err
	}

	_, err = c.UpdateServer(serverID, ServerUpdateSpec{BGPEnabled: false})
	if err != nil {
		return Project{}, fmt.Errorf("couldn't disable BGP for server %d: %w", serverID, err)
	}

	return p, nil
}

func (c Client) waitUntilProjectHasASN(ctx context.Context, p Project) (Project, error) {
	t := c.newTicker()
	asnSet := false
	var err error

	for {
		select {
		case <-ctx.Done():
			return Project{}, ctx.Err()
		case <-t.C:
			p, err = c.GetProject(p.ID)
			if err != nil {
				return Project{}, err
			}
			_, asnSet = p.LocalASN()
		}
		if asnSet {
			break
		}
	}

	return p, nil
}
