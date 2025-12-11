package microk8s

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"time"

	"github.com/cherryservers/cherrygo/v3"
	"github.com/cherryservers/cloud-provider-cherry-tests/backoff"
	"golang.org/x/crypto/ssh"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	apiwatch "k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/watch"
)

type NodeProvisioner struct {
	cherryClient cherrygo.Client
	projectID    int
	sshKeyID     string
	serverPlan   string
	region       string
	cmdRunner    sshCmdRunner
	k8sVersion   string
}

// Provision creates a Cherry Servers server and waits for k8s to be running.
func (np NodeProvisioner) Provision(ctx context.Context) (*ControlPlaneNode, error) {
	const (
		userDataPath  = "./testdata/init-microk8s.yaml"
		k8sVersionVar = "K8S_VERSION"
		timeout       = 30 * time.Minute
	)

	ctx, cancel := context.WithTimeoutCause(ctx, timeout, errors.New("node provision timeout"))
	defer cancel()

	userDataRaw, err := os.ReadFile(userDataPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read user data file: %w", err)
	}
	userDataRaw = bytes.ReplaceAll(userDataRaw, []byte(k8sVersionVar), []byte(np.k8sVersion))
	userdata := base64.StdEncoding.EncodeToString(userDataRaw)

	srv, err := provisionServer(ctx, np.cherryClient, np.projectID, userdata, np.sshKeyID, np.serverPlan, np.region)
	if err != nil {
		return nil, fmt.Errorf("failed to provision server: %w", err)
	}

	n := Node{Server: srv, cmdRunner: np.cmdRunner}

	kubeconfig, err := untilKubeAPIReady(ctx, n)
	if err != nil {
		return nil, fmt.Errorf("kube-api not ready on %q: %w", srv.Hostname, err)
	}

	k8sclient, err := newK8sClient(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create k8s client for node %q: %w", srv.Hostname, err)
	}

	cp := ControlPlaneNode{Node: n, K8sclient: k8sclient}

	err = np.untilProvisioned(ctx, &cp)
	if err != nil {
		return nil, fmt.Errorf("node didn't reach provisioned state: %w", err)
	}

	err = cp.addCpLabel(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to add control plane label: %w", err)
	}

	return &cp, nil
}

// ProvisionBatch wraps provision to create n Cherry Servers servers
// in a concurrent manner.
func (np NodeProvisioner) ProvisionBatch(ctx context.Context, n int) ([]*ControlPlaneNode, []error) {
	type p struct {
		nn  *ControlPlaneNode
		err error
	}

	nodes := make([]*ControlPlaneNode, n)
	errs := make([]error, n)
	c := make(chan p, n)

	for range n {
		go func() {
			nn, err := np.Provision(ctx)
			c <- p{nn: nn, err: err}
		}()
	}
	for i := range n {
		provisioned := <-c
		nodes[i] = provisioned.nn
		errs[i] = provisioned.err

	}
	return nodes, errs
}

// wait until node has provider ID or is tainted with
// 'node.cloudprovider.kubernetes.io/uninitialized'
func (np NodeProvisioner) untilProvisioned(ctx context.Context, n *ControlPlaneNode) error {
	const uninitTaint = "node.cloudprovider.kubernetes.io/uninitialized"
	ctx, cancel := context.WithTimeout(ctx, informerTimeout)
	defer cancel()

	isProvisioned := func(n *corev1.Node) bool {
		if n.Spec.ProviderID != "" {
			return true
		}

		return slices.ContainsFunc(n.Spec.Taints, func(t corev1.Taint) bool {
			return t.Key == uninitTaint
		})
	}

	lw := cache.NewListWatchFromClient(n.K8sclient.CoreV1().RESTClient(), "nodes", "", fields.Everything())

	var precon watch.PreconditionFunc = func(store cache.Store) (bool, error) {
		o, ok, err := store.GetByKey(n.Server.Hostname)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}

		nn, ok := o.(*corev1.Node)
		if !ok {
			return false, fmt.Errorf("unexpected type for %s object: %T", n.Server.Hostname, o)
		}

		return isProvisioned(nn), nil
	}

	_, err := watch.UntilWithSync(ctx, lw, &corev1.Node{}, precon, func(event apiwatch.Event) (bool, error) {
		nn, ok := event.Object.(*corev1.Node)
		if !ok {
			return false, fmt.Errorf("unexpected object type: %T", event.Object)
		}

		return isProvisioned(nn), nil
	})

	return err
}

func (np NodeProvisioner) Cleanup() error {
	_, projectErr := np.cherryClient.Projects.Delete(np.projectID)
	sshID, convErr := strconv.Atoi(np.sshKeyID)
	_, _, sshErr := np.cherryClient.SSHKeys.Delete(sshID)
	return errors.Join(projectErr, convErr, sshErr)
}

func NewNodeProvisioner(testName, k8sVersion, serverPlan, region string, projectID int, cc cherrygo.Client) (NodeProvisioner, error) {
	// Create a SSH key signer:
	sshRunner, err := newSSHCmdRunner()
	if err != nil {
		return NodeProvisioner{}, fmt.Errorf("failed to create SSH runner: %v", err)
	}

	// Create SSH key on Cherry servers:
	pub := ssh.MarshalAuthorizedKey(sshRunner.signer.PublicKey())
	pub = pub[:len(pub)-1] // strip newline
	sshKey, _, err := cc.SSHKeys.Create(&cherrygo.CreateSSHKey{
		Label: testName,
		Key:   string(pub),
	})
	if err != nil {
		return NodeProvisioner{}, fmt.Errorf("failed to create SSH key on cherry servers: %v", err)
	}

	return NodeProvisioner{
		cherryClient: cc,
		projectID:    projectID,
		sshKeyID:     strconv.Itoa(sshKey.ID),
		serverPlan:   serverPlan,
		region:       region,
		cmdRunner:    *sshRunner,
		k8sVersion:   k8sVersion,
	}, nil
}

func provisionServer(ctx context.Context, cc cherrygo.Client, projectID int, userdata, sshkeyID, serverPlan, region string) (cherrygo.Server, error) {
	const (
		serverImage = "ubuntu_24_04_64bit"
		timeout     = time.Minute * 15
	)

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	srv, _, err := cc.Servers.Create(&cherrygo.CreateServer{
		ProjectID: projectID,
		Plan:      serverPlan,
		Region:    region,
		Image:     serverImage,
		UserData:  userdata,
		SSHKeys:   []string{sshkeyID},
	})

	if err != nil {
		return cherrygo.Server{}, fmt.Errorf("failed to create server: %w", err)
	}

	err = backoff.ExpBackoffWithContext(func() (bool, error) {
		srv, _, err = cc.Servers.Get(srv.ID, nil)
		if err != nil {
			return false, fmt.Errorf("failed to get server: %w", err)
		}
		if srv.State == "active" {
			return true, nil
		}
		return false, nil
	}, backoff.DefaultExpBackoffConfigWithContext(ctx))
	if err != nil {
		return cherrygo.Server{}, fmt.Errorf("server didn't reach active state: %w", err)
	}

	return srv, nil
}
