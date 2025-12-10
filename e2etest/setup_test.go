package e2etest

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"time"

	"testing"

	"github.com/cherryservers/cherrygo/v3"
	"k8s.io/client-go/kubernetes"

	"github.com/cherryservers/cloud-provider-cherry-tests/microk8s"
	ccm "github.com/cherryservers/cloud-provider-cherry/cherry"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	metallbSetting = "metallb:///"
	kubeVipSetting = "kube-vip://"
)

func setupProject(t testing.TB, name string) cherrygo.Project {
	t.Helper()

	project, _, err := cherryClient.Projects.Create(*teamID, &cherrygo.CreateProject{
		Name: name})
	if err != nil {
		t.Fatalf("failed to setup cherry servers project: %v", err)
	}
	return project
}

type testEnv struct {
	project         cherrygo.Project
	mainNode        *microk8s.ControlPlaneNode
	nodeProvisioner nodeProvisioner
	k8sClient       kubernetes.Interface
	ctx             context.Context
}

type testEnvConfig struct {
	name         string
	loadBalancer string // optional
	fipTag       string // optional
}

type nodeProvisioner interface {
	Provision(context.Context) (*microk8s.ControlPlaneNode, error)
	ProvisionBatch(context.Context, int) ([]*microk8s.ControlPlaneNode, []error)
}

func setupTestEnv(t *testing.T, cfg testEnvConfig) *testEnv {
	t.Helper()
	ctx, _ := beforeTimeoutCtx(t)

	// Setup project:
	project := setupProject(t, cfg.name)

	// Setup node provisioner:
	np, err := microk8s.NewMicrok8sNodeProvisioner(cfg.name, *k8sVersion, *serverPlan, *region, project.ID, *cherryClient)
	if err != nil {
		t.Fatalf("failed to setup node provisioner: %v", err)
	}

	if *cleanup {
		t.Cleanup(func() {
			np.Cleanup()
		})
	}

	// Create a node (server with k8s running):
	n, err := np.Provision(ctx)
	if err != nil {
		t.Fatalf("failed to provision test node: %v", err)
	}

	err = n.LoadImage(*ccmImagePath)
	if err != nil {
		t.Fatalf("failed to load image to node: %v, from path: %q", err, *ccmImagePath)
	}

	deployCcm(ctx, t, n, ccm.Config{
		AuthToken:               cherryClient.AuthToken,
		Region:                  *region,
		LoadBalancerSetting:     cfg.loadBalancer,
		FIPTag:                  cfg.fipTag,
		ProjectID:               project.ID,
		FIPHealthCheckUseHostIP: true})

	return &testEnv{
		project:         project,
		mainNode:        n,
		k8sClient:       n.K8sclient,
		nodeProvisioner: np,
		ctx:             ctx,
	}
}

func deployCcm(ctx context.Context, t testing.TB, n *microk8s.ControlPlaneNode, cfg ccm.Config) {
	const (
		imgTag       = "ghcr.io/cherryservers/cloud-provider-cherry:test"
		manifestPath = "../deploy/template/deployment.yaml"
	)

	configJSON, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("failed to marshall ccm config: %v", err)
	}

	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "cherry-cloud-config", Namespace: metav1.NamespaceSystem},
		StringData: map[string]string{"cloud-sa.json": string(configJSON)},
	}

	n.K8sclient.CoreV1().Secrets(metav1.NamespaceSystem).Create(ctx, &secret, metav1.CreateOptions{})

	manifest, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("failed to read deployment manifest file: %v", err)
	}
	manifest = bytes.Replace(manifest, []byte("RELEASE_IMG"), []byte(imgTag), 1)
	manifest = bytes.Replace(
		manifest,
		[]byte("imagePullPolicy: Always"),
		[]byte("imagePullPolicy: Never"), 1)

	err = n.Deploy(bytes.NewReader(manifest))
	if err != nil {
		t.Fatalf("failed to deploy ccm: %v", err)
	}

	// when node.cloudprovider.kubernetes.io/uninitialized
	// is gone, the ccm is running.
	err = n.UntilHasProviderID(ctx, n.K8sclient)
	if err != nil {
		t.Fatalf("node %q didn't get provider ID: %v", n.Server.Hostname, err)
	}
}

func beforeTimeoutCtx(t *testing.T) (context.Context, context.CancelFunc) {
	const cleanupPadding = time.Minute * 2
	deadline, ok := t.Deadline()
	if ok {
		return context.WithDeadline(t.Context(), deadline.Add(-cleanupPadding))
	}
	return context.WithCancel(t.Context())

}
