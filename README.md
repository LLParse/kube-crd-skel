kube-crd-skel
=============

This project provides a skeleton for writing Kubernetes controllers that interact with Kubernetes CustomResourceDefinitions.

## Usage

1. Start a Kubernetes v1.7.0+ cluster and configure `kubectl` CLI with a valid kube config.
2. Run `kubectl create -f hack/deploy.yaml` to start an in-cluster controller.
3. Run `kubectl create -f hack/example/vm.yaml` to create an example object.

## Building

To build the controller for your machine's OS/architecture, run:

`hack/build.sh`

To build the Docker image `mydockerhubacct/ranchervm-controller:dev`, run:

`IMAGE=yes REPO=mydockerhubacct hack/build.sh`

The script will ask you if you want to publish the image to Dockerhub.

### Code Generation

Changing CRD schema will require regenerating client code. To do so, run:

`hack/update-codegen.sh`

### Updating dependencies

This project uses [glide](https://github.com/Masterminds/glide) for package management.
To update the project dependencies, run:

`hack/update-deps.sh` 

## Validation

In Kubernetes v1.9, CRD validation beta feature is enabled by default.
In Kubernetes v1.8, CRD validation alpha feature must be explicitly enabled.
To do so, start `kube-apiserver` with the following flag appended:

`--feature-gates=CustomResourceValidation=true`

If the feature remains disabled, any validation definitions will be ignored.
