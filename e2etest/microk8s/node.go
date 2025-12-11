package microk8s

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/cherryservers/cherrygo/v3"
	"k8s.io/client-go/kubernetes"

	"github.com/cherryservers/cloud-provider-cherry-tests/backoff"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	apiwatch "k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/watch"
)

const (
	APIPort               = 16443 // default microk8s kube-api port
	ControlPlaneNodeLabel = "node-role.kubernetes.io/control-plane"
	informerTimeout       = 300 * time.Second
	joinTimeout           = 360 * time.Second
)

type Node struct {
	Server    cherrygo.Server
	cmdRunner sshCmdRunner
}

// runCmd runs a shell command on the node via SSH.
// Passing nil stdin is fine.
func (n *Node) runCmd(cmd string, stdin io.Reader) (resp string, err error) {
	ip, err := serverPublicIP(n.Server)
	if err != nil {
		return "", err
	}
	return n.cmdRunner.run(ip, cmd, stdin)
}

type nodeConditionFunc func(node *corev1.Node) bool

func (n *Node) untilNodeCondition(ctx context.Context, k8sClient kubernetes.Interface, f nodeConditionFunc) error {
	ctx, cancel := context.WithTimeout(ctx, informerTimeout)
	defer cancel()

	lw := cache.NewListWatchFromClient(k8sClient.CoreV1().RESTClient(), "nodes", "", fields.Everything())

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

		return f(nn), nil
	}

	_, err := watch.UntilWithSync(ctx, lw, &corev1.Node{}, precon, func(event apiwatch.Event) (bool, error) {
		nn, ok := event.Object.(*corev1.Node)
		if !ok {
			return false, fmt.Errorf("unexpected object type: %T", event.Object)
		}

		return f(nn), nil
	})

	return err
}

func (n *Node) UntilHasProviderID(ctx context.Context, k8sClient kubernetes.Interface) error {
	return n.untilNodeCondition(ctx, k8sClient, func(node *corev1.Node) bool {
		return node.Name == n.Server.Hostname && node.Spec.ProviderID != ""
	})
}

func (n *Node) untilReady(ctx context.Context, k8sClient kubernetes.Interface) error {
	return n.untilNodeCondition(ctx, k8sClient, func(node *corev1.Node) bool {
		if n.Server.Hostname != node.Name {
			return false
		}

		for _, con := range node.Status.Conditions {
			if con.Type == corev1.NodeReady && con.Status == corev1.ConditionTrue {
				return true
			}
		}

		return false
	})
}

// LoadImage side-loads a OCI image tarball onto the node.
func (n *Node) LoadImage(ociPath string) error {
	oci, err := os.Open(ociPath)
	if err != nil {
		return fmt.Errorf("failed to open oci tar file: %w", err)
	}
	defer oci.Close()

	addr, err := serverPublicIP(n.Server)
	if err != nil {
		return fmt.Errorf("server %q has no public ip", n.Server.Hostname)
	}
	r, err := n.cmdRunner.run(addr, "microk8s ctr image import - ", oci)
	if err != nil {
		return fmt.Errorf("failed to load image: %w, with stderr: %s", err, r)
	}
	return nil
}

// AssignIP assigns IP to the node's loopback interface.
func (n *Node) AssignIP(ip string) error {
	r, err := n.runCmd(fmt.Sprintf("ip a add %s dev lo", ip), nil)
	if err != nil {
		return fmt.Errorf("failed to assign ip to lo: %s", r)
	}
	return nil
}

// DeleteIP deletes IP from the node's loopback interface.
func (n *Node) DeleteIP(ip string) error {
	r, err := n.runCmd(fmt.Sprintf("ip a delete %s dev lo", ip), nil)
	if err != nil {
		return fmt.Errorf("failed to delete ip from lo: %s", r)
	}
	return nil
}

// Shutdown shuts down the node.
func (n *Node) Shutdown() error {
	resp, err := n.runCmd("shutdown now", nil)
	if err != nil {
		return fmt.Errorf("node %q failed \"shutdown now\" command, resp: %q, with error: %v ",
			n.Server.Hostname, resp, err)
	}

	return nil
}

type WorkerNode struct {
	Node
}

type ControlPlaneNode struct {
	Node
	K8sclient kubernetes.Interface
}

func (n *ControlPlaneNode) Deploy(manifest io.Reader) error {
	r, err := n.runCmd("microk8s kubectl apply -f - ", manifest)
	if err != nil {
		return fmt.Errorf("failed to apply manifest: %s", r)
	}
	return nil
}

func (n *ControlPlaneNode) getJoinCmd(worker bool) (string, error) {
	r, err := n.runCmd("microk8s add-node", nil)
	if err != nil {
		return "", fmt.Errorf("couldn't get join URL from control plane node: %w", err)
	}
	ip, err := serverPublicIP(n.Server)
	if err != nil {
		return "", err
	}

	// parse the microk8s join invitation response message
	// looking for public ip
	joinCmd := ""
	for line := range strings.Lines(r) {
		if strings.Contains(line, ip) {
			joinCmd = line[:len(line)-1] // strip newline
		}
	}
	if joinCmd == "" {
		return "", fmt.Errorf("no ip address in join cmd: %q", r)
	}

	if worker {
		joinCmd += " --worker"
	}

	return joinCmd, nil
}

func (n *ControlPlaneNode) join(nn Node, worker bool) error {
	joinCmd, err := n.getJoinCmd(worker)
	if err != nil {
		return fmt.Errorf("couldn't get join cmd from control plane: %w", err)
	}

	_, err = nn.runCmd(joinCmd, nil)
	if err != nil {
		return fmt.Errorf("couldn't execute join cmd: %w", err)
	}

	return nil
}

// JoinAsControlPlane joins nn to the base node's cluster as a CP node.
// Blocks until the node is ready.
// The base node's cluster MUST have the CCM running.
func (n *ControlPlaneNode) JoinAsControlPlane(ctx context.Context, nn Node) (*ControlPlaneNode, error) {
	ctx, cancel := context.WithTimeoutCause(ctx, joinTimeout, errors.New("node join timeout"))
	defer cancel()

	err := n.join(nn, false)
	if err != nil {
		return nil, fmt.Errorf("failed to execute join cmd: %w", err)
	}

	kubeconfig, err := untilKubeAPIReady(ctx, nn)
	if err != nil {
		return nil, fmt.Errorf("kube-api not ready on node %q: %w", nn.Server.Hostname, err)
	}

	k8sClient, err := newK8sClient(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create k8s client: %w", err)
	}

	newCp := ControlPlaneNode{Node: nn, K8sclient: k8sClient}

	err = newCp.untilReady(ctx, newCp.K8sclient)
	if err != nil {
		return nil, fmt.Errorf("added node didn't get ready status: %w", err)
	}

	err = newCp.addCpLabel(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to add control plane label: %w", err)
	}

	return &newCp, nil
}

// JoinAsWorker joins nn to the base node's cluster as a worker.
// Blocks until the node is ready.
// The base node's cluster MUST have the CCM running.
func (n *ControlPlaneNode) JoinAsWorker(ctx context.Context, nn Node) (WorkerNode, error) {
	ctx, cancel := context.WithTimeoutCause(ctx, joinTimeout, errors.New("node join timeout"))
	defer cancel()

	err := n.join(nn, true)
	if err != nil {
		return WorkerNode{}, fmt.Errorf("failed to execute join cmd: %w", err)
	}
	newNode := WorkerNode{Node: nn}

	err = newNode.UntilHasProviderID(ctx, n.K8sclient)
	if err != nil {
		return WorkerNode{}, fmt.Errorf("added node didn't get provider ID: %w", err)
	}

	return newNode, nil
}

// JoinControlPlanesBatch wraps join to join multiple nodes to the base node
// in a concurrent manner.
func (n *ControlPlaneNode) JoinControlPlanesBatch(ctx context.Context, nodes []Node) ([]*ControlPlaneNode, []error) {
	type s struct {
		err  error
		node *ControlPlaneNode
	}

	errs := make([]error, len(nodes))
	newNodes := make([]*ControlPlaneNode, len(nodes))
	c := make(chan s, len(nodes))

	for i := range len(nodes) {
		go func() {
			node, err := n.JoinAsControlPlane(ctx, nodes[i])
			c <- s{err, node}
		}()
	}

	for i := range len(nodes) {
		r := <-c
		errs[i] = r.err
		newNodes[i] = r.node
	}
	return newNodes, errs
}

// addCpLabel adds the well-known control plane label
// to the node, since microk8s doesn't use it,
// but we need it for fip reconciliation.
func (n *ControlPlaneNode) addCpLabel(ctx context.Context) error {
	ctx, cancel := context.WithTimeoutCause(ctx, 64*time.Second, fmt.Errorf("timed out on label apply for %s", n.Server.Hostname))
	defer cancel()

	_, err := n.K8sclient.CoreV1().Nodes().Patch(
		ctx,
		n.Server.Hostname,
		types.MergePatchType,
		fmt.Appendf(nil, `{"metadata":{"labels":{"%s":""}}}`, ControlPlaneNodeLabel),
		metav1.PatchOptions{},
	)

	return err
}

// check for kubectl success through ssh
// returns kubeconfig
func untilKubeAPIReady(ctx context.Context, n Node) (string, error) {
	const timeout = time.Minute * 10
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ip, err := serverPublicIP(n.Server)
	if err != nil {
		return "", fmt.Errorf("couldn't get server ip: %w", err)
	}

	err = backoff.ExpBackoffWithContext(func() (bool, error) {
		// Check if kube-api is reachable. Non-zero exit code will be returned if not.
		_, err := n.cmdRunner.run(ip, "microk8s kubectl get --raw='/readyz'", nil)
		if err != nil {
			return false, nil
		}
		return true, nil
	}, backoff.DefaultExpBackoffConfigWithContext(ctx))
	if err != nil {
		return "", fmt.Errorf("node kube-api didn't become reachable: %w", err)
	}

	kubeconfig, err := n.cmdRunner.run(ip, "microk8s config", nil)
	if err != nil {
		return "", fmt.Errorf("failed to get kubeconfig %w", err)
	}
	return kubeconfig, nil
}

func serverPublicIP(srv cherrygo.Server) (string, error) {
	for _, ip := range srv.IPAddresses {
		if ip.Type == "primary-ip" {
			return ip.Address, nil
		}
	}
	return "", fmt.Errorf("server %d has no public ip", srv.ID)
}

func newK8sClient(kubeconfig string) (*kubernetes.Clientset, error) {
	cfg, err := clientcmd.RESTConfigFromKubeConfig([]byte(kubeconfig))
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(cfg)
}

// ControlPlanesToNodes extracts embedded Node objects from
// the ControlPlaneNode
func ControlPlanesToNodes(cps []*ControlPlaneNode) []Node {
	nodes := make([]Node, len(cps))
	for i, n := range cps {
		nodes[i] = n.Node
	}
	return nodes
}
