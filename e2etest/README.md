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