# nova-operator

A golang operator for openstack nova lifecycle management

## nova-operator Goal

The goal of nova-operator is to manage custom resource that define a nova control plane (CRD).
Nova operator continuously monitors the state of Nova CR and takes actions to ensure that the desired state is applied and reflected in nova pods. These pods run actual nova services, nova-operators ensure they are deployed, scaled, and configured correctly.
**Note:** Nova CR is created by openstack-operator as per user initial deployment.

## Description

This operator is built using the operator-sdk framework to provide day one and day two
lifecycle management of the OpenStack nova service on an OpenShift cluster.

## Getting Started

You’ll need a Kubernetes cluster to run against.
You can use [openshift-local](https://developers.redhat.com/products/openshift-local/overview) to get a local cluster for testing, or run against a remote cluster.
**Note:** Your controller will automatically use the current context in your kubeconfig file (i.e. whatever cluster `kubectl cluster-info` shows).

### pre-commit

This repo uses pre-commit to automate basic checks that should be run before pushing a PR for review.
pre-commit is optional but recommend to ensure good git hygiene.

#### install go if required

```sh
sudo dnf install -y golang
```

#### installing in a virtual env

```sh
python3 -m venv .venv
. .venv/bin/activate
python3 -m pip install pre-commit
pre-commit install --install-hooks
```

#### golangci-lint

pre-commit is configured to run golangci-lint on each commit

```sh
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin v1.50.0
```

add "$(go env GOPATH)/bin" to your path via ~/.bashrc

```sh
if ! [[ "$PATH" =~ "$(go env GOPATH)/bin" ]]
then
    export PATH=${PATH}:$(go env GOPATH)/bin
fi
```

confirm golangci-lint is installed

```sh
[stack@crc nova-operator]$ golangci-lint --version
golangci-lint has version 1.50.0 built from 704109c6 on 2022-10-04T10:25:07Z
```

#### confirm pre-commit is working

**NOTE:** this might take some time on the first run as it need to build the operator

```sh
(.venv) [stack@crc nova-operator]$ pre-commit run -a
make-manifests...........................................................Passed
make-generate............................................................Passed
go fmt...................................................................Passed
go vet...................................................................Passed
go-mod-tidy..............................................................Passed
golangci-lint............................................................Passed
check for added large files..............................................Passed
fix utf-8 byte order marker..............................................Passed
check for case conflicts.................................................Passed
check that executables have shebangs.....................................Passed
check that scripts with shebangs are executable..........................Passed
check for merge conflicts................................................Passed
check for broken symlinks............................(no files to check)Skipped
detect destroyed symlinks................................................Passed
check yaml...............................................................Passed
check json...............................................................Passed
detect private key.......................................................Passed
fix end of files.........................................................Passed
don't commit to branch...................................................Passed
trim trailing whitespace.................................................Passed
```

### Running kuttl tests

Kuttl testing requires some initial setup on the host to enable running them.
first the kuttl oc/kubectl plugin need to be installed, that is done via using
the krew plugin manager. kuttl testing in this repo will never install or modify
operators in the test cloud. that means that you can run them in the openshift
cluster or locally (with or without a debugger). While these test should be able
to run on any openshift they have only been tested with crc.

1. install krew

```sh
bash hack/install-krew.sh
```

this will download the latest krew tar and unpack it in a temp dir
the use krew to install itself. This will result in the creation of ${HOME}/.krew

To make krew usable we then need to add it to the path.
add this to your ~/.bashrc and source it.

```sh
if [[ ! "${PATH}" =~ "$HOME/.krew/bin"  ]]; then
    export PATH="$HOME/.krew/bin:$PATH"
fi
```

this will make the krew kubectl/oc plug available
and any future plugins will be available automatically.

2. install kuttl

```sh
oc krew install kuttl
```

3. prepare crc

kuttl testing should be usable with any existing crc env and is
is intended to be runnable in parallel to a dev openstack deployment.
As such i will not describe this in detail but the hi level steps are as follows

```sh
crc setup
crc start
oc login ...
cd /path/to/install_yamls/devsetup
make crc_attach_default_interface
cd ..
make openstack
```
note: we will use the `crc-csi-hostpath-provisioner` storage class so "make crc_storage"
is not required but it won't break anything either.

4. prepare kuttl deps

The makefile supports specifying a kuttl test suite to use via `KUTTL_SUITE`
currently only one suite exist `multi-cell` and this is the default.

prepare the deps using

```sh
make kuttl-test-prep
```

This will use the openstack operator to deploy rabbitmq, galera, memcached,
keystone and placement into a dedicated kuttl namespace via the OpenStackControlplane CR.

5. run kuttl tests

```sh
make kuttl-test-run
```

Note: step 4 and 5 can be combined by using `make kuttl-test`

6. kuttl cleanup

if you want to reclaim resources form the CRC env when you are finished doing kuttl testing
locally you can do that with:

```sh
make kuttl-test-cleanup
```

This will clean up the kuttl deps including the kuttl namespace for the test suite.

### Running on the cluster

1. Install Instances of Custom Resources:

```sh
make install
```

2. Build and push your image to the location specified by `IMG`:

```sh
make docker-build docker-push IMG=<some-registry>/nova-operator:tag
```

3. Deploy the controller to the cluster with the image specified by `IMG`:

```sh
make deploy IMG=<some-registry>/nova-operator:tag
```

### Uninstall CRDs

To delete the CRDs from the cluster:

```sh
make uninstall
```

### Undeploy controller

UnDeploy the controller to the cluster:

```sh
make undeploy
```

## Contributing

// TODO(user): Add detailed information on how you would like others to contribute to this project

### How it works

This project aims to follow the Kubernetes [Operator pattern](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/)

It uses [Controllers](https://kubernetes.io/docs/concepts/architecture/controller/)
which provides a reconcile function responsible for synchronizing resources untile the desired state is reached on the cluster

### Test It Out

1. Install the CRDs into the cluster:

```sh
make install
```

2. Run your controller (this will run in the foreground, so switch to a new terminal if you want to leave it running):

```sh
make run
```

**NOTE:** You can also run this in one step by running: `make install run`

### Modifying the API definitions

If you are editing the API definitions, generate the manifests such as CRs or CRDs using:

```sh
make manifests
```

**NOTE:** Run `make --help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

### Running tempest locally

1. Attach default interface, assumes in [install_yamls/devsetup](https://github.com/openstack-k8s-operators/install_yamls/tree/main/devsetup) directory
```sh
make crc_attach_default_interface
```

2. Install operators, from install_yamls directory
```sh
make openstack
```

3. From nova-operator deploy openstack
```sh
ansible-playbook ci/nova-operator-compute-kit/playbooks/deploy-openstack.yaml -e @${HOME}/my-envs.yml
```

**NOTE:** The my-envs.yaml contains overrides pointing to base directory of nova operator and sets target host to localhost
```sh
cat ~/my-envs.yml
---
nova_operator_basedir: '/home/stack/nova-operator'
target_host: 'localhost'
```

4. Run tempest playbook
```sh
ansible-playbook ci/nova-operator-compute-kit/playbooks/tempest.yaml -e @${HOME}/my-envs.yml
```

## License

Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
