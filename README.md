# nova-operator

A golang operator for openstack nova lifecycle management

## Description

This operator is built using the operator-sdk framework to provide day one and day two
lifecycle managment of the OpenStack nova service on an OpenShift cluster.

## Getting Started

You’ll need a Kubernetes cluster to run against.
You can use [openshift-local](https://developers.redhat.com/products/openshift-local/overview) to get a local cluster for testing, or run against a remote cluster.
**Note:** Your controller will automatically use the current context in your kubeconfig file (i.e. whatever cluster `kubectl cluster-info` shows).

### golang

this repo currently uses go 1.18

### pre-commit

This repo uses pre-commit to automate basic checks that should be run before pushing a PR for review.
pre-commit is optional but recommend to ensure good git hygiene.

#### install go if required

```sh
sudo dnf install -y golang
```

#### installing in a virutal env

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
