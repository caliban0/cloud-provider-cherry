# E2E tests

This directory contains the end-to-end test sub-module of the CCM.
The tests are implemented using Go's standard `testing` package.

Each test is performed in a separate Cherry Servers project.
The general testing process is as follows:
1. Create a project.
2. Create an SSH key that we will use to connect to the server.
3. Create a server and deploy `microk8s` on it, through cloud-config.
4. Side-load a user provided CCM image on the node and deploy it.
5. Perform the test.
6. Cleanup - delete the SSH key and the project, along with all resources that belong to it.

The tests can be also be run with `make test-e2e`.

If a timeout is set for the tests, two minutes of that timeout will be reserved for teardown.
So if `timeout=20m` is set, the actual test execution time will be limited to 18 minutes.

## Configuration

All configuration is done with environment variables:

- `CHERRY_TEST_API_TOKEN` - Cherry Servers API token, required.
- `CHERRY_TEST_TEAM_ID` - the team for which all resources (projects, SSH keys, IPs, etc.) will be created, required.
- `CCM_IMG_PATH` - path to the CCM image tarball, required.
- `NO_CLEANUP` - if set to `true`, post-test cleanup will be disabled, optional.
- `K8S_VERSION` - what version of k8s to deploy on the cluster, optional. Defaults to `1.34`
- `METALLB_VERSION` - what version of `metallb` to deploy for the `metallb` test, optional. Defaults to `0.15.2`
- `KUBE_VIP_VERSION` - what version of `kube-vip` to deploy for the `kube-vip` test, optional. Defaults to `1.0.1`

The `make` targets `build-test-image` and `buildx-builder` are available for building the test image and creating a `buildx` builder, if required.

## Test details

The following controllers are tested: instance, load balancer (with `kube-vip` and `metallb`), FIP control plane load balancer.

### Instance controller

1. Add a worker node to the cluster, check that:
   - It got provider ID and that it matches the server ID from the Cherry Servers API.
   - Metadata labels for a region, type and addresses.
2. Delete a node from the cluster via Cherry Servers API, check that :
   - There is an event for node deletion in the cluster.


### Load Balancer controller

1. Add a worker node to the cluster.
2. Deploy `metallb`/`kube-vip`.
3. Deploy a test deployment (`nginx`) and two `LoadBalancer` type services for it.
4. Check that a FIP is assigned to both services as an Ingress IP and that the FIPs get correct tags for:
   - Cluster ID
   - Service (SHA256 of namespace and name)
   - Usage (Cherry Servers cloud provider)
5. Check that the project and the nodes that host the services have BGP enabled (BGP enabling functionality).
6. Check that each service has a distinct Ingress IP.
7. Check that a service is reachable (with an HTTP request).
8. Delete the first service and check that:
   - The FIP for that service is deleted.
   - The second service is unaffected and still has it's FIP set as Ingress IP.
9. Delete the second service and check that it's FIP is deleted as well.

Also, as part of the `kube-vip` test, we check the worker node get's annotations for:
- Local ASN
- Peer ASN
- Peer IP 

### FIP control plane load balancer controller:

1. Create a FIP with a tag that matches the CCM's `fipTag`.
2. Assign the FIP to the control plane node's loopback interface.
3. Check that the FIP gets assigned to the control plane node.
4. Add three additional control plane nodes to the cluster. Two are enough for the test, but `microk8s` automatically enables it's own HA when there are three control planes in the cluster, which leads two quorum issues later when we remove nodes, if we don't have at least two.
5. Assign the FIP to each of the new control plane node loopback interfaces.
6. Delete one of the nodes that the FIP isn't attached to through the Cherry Servers API.
7. Check that the FIP is still attached to the same node.
8. Assign the FIP to another node. This is done because the CCM image is only side-loaded to one node, so we can't delete it.
9. Disable the node that the FIP is attached to, but don't delete it through the Cherry Servers API.
10. Check that the FIP is attached to one of the remaining nodes.
11. Assign the FIP to the loopback interface on those nodes.
12. Check that the `kube-api` service is reachable through the FIP (with an HTTP request).
