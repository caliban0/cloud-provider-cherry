package e2etest

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/cherryservers/cherrygo/v3"
	"github.com/cherryservers/cloud-provider-cherry-tests/backoff"
	"github.com/cherryservers/cloud-provider-cherry-tests/microk8s"
)

func untilIPHasTarget(ctx context.Context, ip cherrygo.IPAddress, target ...string) error {
	const timeout = 300 * time.Second
	ctx, cancel := context.WithTimeoutCause(
		ctx, timeout, errors.New("timeout out waiting for ip to get target"))
	defer cancel()

	return backoff.ExpBackoffWithContext(func() (bool, error) {
		fip, _, err := cherryClient.IPAddresses.Get(ip.ID, nil)
		if err != nil {
			return false, fmt.Errorf("failed to get fip: %w", err)
		}
		if slices.Contains(target, fip.TargetedTo.Hostname) {
			return true, nil
		}
		return false, nil
	}, backoff.DefaultExpBackoffConfigWithContext(ctx))
}

func TestFipControlPlaneReconciliation(t *testing.T) {
	t.Parallel()
	const fipTag = "kubernetes-ccm-test"

	cfg := testEnvConfig{name: "kubernetes-ccm-test-fip-controlplane", fipTag: fipTag}
	env := setupTestEnv(t, cfg)
	ctx := env.ctx

	fip, _, err := cherryClient.IPAddresses.Create(
		env.project.ID, &cherrygo.CreateIPAddress{
			Region: env.mainNode.Server.Region.Slug,
			Tags:   &map[string]string{fipTag: ""}})

	if err != nil {
		t.Fatalf("failed to create cherry servers fip: %v", err)
	}

	if err = env.mainNode.AssignIP(fip.Address); err != nil {
		t.Fatalf("failed to assign ip %s to %s", fip.Address, env.mainNode.Server.Hostname)
	}

	err = untilIPHasTarget(ctx, fip, env.mainNode.Server.Hostname)
	if err != nil {
		t.Fatalf("fip %s didn't get attached to cp node: %v", fip.ID, err)
	}

	// Provision enough nodes, so that we don't fall below two for the cluster,
	// otherwise dqlite quorum breaks.
	nodes, errs := env.nodeProvisioner.ProvisionBatch(ctx, 3)
	for _, err := range errs {
		if err != nil {
			t.Fatalf("failed to provision node: %v", err)
		}
	}

	nodes, errs = env.mainNode.JoinControlPlanesBatch(ctx, microk8s.ControlPlanesToNodes(nodes))
	for _, err := range errs {
		if err != nil {
			t.Fatalf("failed to join node: %v", err)
		}
	}

	cp1 := nodes[0]
	cp2 := nodes[1]
	cp3 := nodes[2]

	for _, cp := range nodes {
		if err = cp.AssignIP(fip.Address); err != nil {
			t.Fatalf("failed to assign ip %s to %s", fip.Address, cp.Server.Hostname)
		}
	}

	wantTarget := env.mainNode.Server.Hostname

	// test that fip remains attached to node after deleting another node
	_, _, err = cherryClient.Servers.Delete(cp1.Server.ID)
	if err != nil {
		t.Fatalf("failed to delete server %q: %v", cp1.Server.Hostname, err)
	}

	k8sn, err := env.k8sClient.CoreV1().Nodes().Get(ctx, cp1.Server.Hostname, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to get node: %v", err)
	}

	err = untilNodeDeleted(ctx, *k8sn, env.k8sClient)
	if err != nil {
		t.Fatalf("node %q didn't get deleted: %v", k8sn.Name, err)
	}

	fip, _, err = cherryClient.IPAddresses.Get(fip.ID, nil)
	if err != nil {
		t.Fatalf("failed to get fip %s: %v", fip.ID, err)
	}
	if fip.TargetedTo.Hostname != wantTarget {
		t.Fatalf("fip %s target: %q, want %q", fip.ID, fip.TargetedTo.Hostname, wantTarget)
	}

	// test that fip is reattached when a cp node is disabled

	// Reassign the FIP, so that we don't have to shut down the main node,
	// since the main node is the one that has the CCM image side-loaded.
	_, _, err = cherryClient.IPAddresses.Assign(fip.ID, &cherrygo.AssignIPAddress{ServerID: cp2.Server.ID})
	if err != nil {
		t.Fatalf("failed to re-assign ip %s: %v", fip.ID, err)
	}

	err = cp2.Shutdown()
	if err != nil {
		t.Fatalf("couldn't shut down node: %v", err)
	}

	wantTargets := []string{wantTarget, cp3.Server.Hostname}

	err = untilIPHasTarget(ctx, fip, wantTargets...)
	if err != nil {
		t.Fatalf("fip %s didn't get attached to any of cp nodes %v: %v", fip.ID, wantTargets, err)
	}

	t.Run("fip reachable", func(t *testing.T) {
		resp, err := http.Get(fmt.Sprintf("http://%s:%d", fip.Address, microk8s.APIPort))
		if err != nil {
			t.Fatalf("failed get request to %s:%d:%v ", fip.Address, microk8s.APIPort, err)
		}

		if got, want := resp.StatusCode, http.StatusBadRequest; got != want {
			t.Errorf("response status %d, want %d", got, want)
		}
	})
}
