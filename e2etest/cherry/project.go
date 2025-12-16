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

type projectClient interface {
	Create(teamID int, request *cherrygo.CreateProject) (cherrygo.Project, *cherrygo.Response, error)
	Update(id int, request *cherrygo.UpdateProject) (cherrygo.Project, *cherrygo.Response, error)
	Get(id int, opts *cherrygo.GetOptions) (cherrygo.Project, *cherrygo.Response, error)
	Delete(id int) (*cherrygo.Response, error)
}

type serverListerUpdater interface {
	List(projectID int, opts *cherrygo.GetOptions) ([]cherrygo.Server, *cherrygo.Response, error)
	Update(id int, request *cherrygo.UpdateServer) (cherrygo.Server, *cherrygo.Response, error)
}

type ProjectClient struct {
	c      projectClient
	server serverListerUpdater
	ticker tickerFactory
}

func NewProjectClient(c projectClient, s serverListerUpdater, t tickerFactory) ProjectClient {
	return ProjectClient{c: c, server: s, ticker: t}
}

// Pseudo-constant for the project fields we want to get from the API.
var projectGetFields = []string{"id", "bgp", "name"}

// Get gets a project from Cherry Servers.
func (c ProjectClient) Get(id int) (Project, error) {
	p, _, err := c.c.Get(id, &cherrygo.GetOptions{Fields: projectGetFields})
	if err != nil {
		return Project{}, fmt.Errorf("couldn't get project %d: %w", id, err)
	}
	return projectFrom(p), nil
}

type NewProjectSpec struct {
	TeamID int
	Name   string
}

// Create creates a project on Cherry Servers.
func (c ProjectClient) Create(spec NewProjectSpec) (Project, error) {
	p, _, err := c.c.Create(spec.TeamID, &cherrygo.CreateProject{Name: spec.Name})
	if err != nil {
		return Project{}, fmt.Errorf("couldn't create project with spec %v: %w", spec, err)
	}
	return projectFrom(p), nil
}

// Delete deletes a project from Cherry Servers.
// This also deletes all resources associated with that project.
func (c ProjectClient) Delete(id int) error {
	_, err := c.c.Delete(id)
	if err != nil {
		return fmt.Errorf("failed to delete project %d: %w", id, err)
	}
	return nil
}

type ProjectUpdateSpec struct {
	BGPEnabled bool
}

// Update updates a project on Cherry Servers.
func (c ProjectClient) Update(id int, spec ProjectUpdateSpec) (Project, error) {
	p, _, err := c.c.Update(id, &cherrygo.UpdateProject{Bgp: &spec.BGPEnabled})
	if err != nil {
		return Project{}, fmt.Errorf("couldn't update project %d: %w", id, err)
	}
	return projectFrom(p), nil
}

// ForceASN works around API limitations to ensure that
// a project has a local ASN.
//
// As of this writing, enabling BGP for a project does not
// create a local ASN for it. Only after BGP is enabled for a server
// from that project, does the project get a local ASN.
//
// To work around this, ForceASN picks a project's server,
// enables BGP for it, waits for the project to get a local ASN and
// then disables BGP for that server. The project then retains that ASN.
//
// If the project already has a local ASN, does nothing.
func (c ProjectClient) ForceASN(ctx context.Context, p Project) (Project, error) {
	if _, ok := p.LocalASN(); ok {
		return p, nil
	}

	servers, _, err := c.server.List(p.ID, &cherrygo.GetOptions{Fields: serverGetFields})
	if err != nil {
		return Project{}, err
	}

	if len(servers) < 1 {
		return Project{}, fmt.Errorf("to force an asn, project %d must have at least one server", p.ID)
	}
	s := servers[0]

	return c.forceASN(ctx, p, s.ID)
}

func (c ProjectClient) forceASN(ctx context.Context, p Project, serverID int) (Project, error) {
	if !p.BGPEnabled {
		_, err := c.Update(p.ID, ProjectUpdateSpec{BGPEnabled: true})
		if err != nil {
			return Project{}, fmt.Errorf("couldn't enable BGP for project: %w", err)
		}
	}

	_, _, err := c.server.Update(serverID, &cherrygo.UpdateServer{Bgp: true})
	if err != nil {
		return Project{}, fmt.Errorf("couldn't enable BGP for server %d: %w", serverID, err)
	}

	p, err = c.untilHasASN(ctx, p)
	if err != nil {
		return Project{}, err
	}

	_, _, err = c.server.Update(serverID, &cherrygo.UpdateServer{Bgp: false})
	if err != nil {
		return Project{}, fmt.Errorf("couldn't disable BGP for server %d: %w", serverID, err)
	}

	return p, nil
}

func (c ProjectClient) untilHasASN(ctx context.Context, p Project) (Project, error) {
	t := c.ticker.newTicker()
	asnSet := false
	var err error

	for {
		select {
		case <-ctx.Done():
			return Project{}, ctx.Err()
		case <-t.C:
			p, err = c.Get(p.ID)
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
