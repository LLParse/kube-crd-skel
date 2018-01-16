kube-crd-skel
=============

This project provides a skeleton for writing Kubernetes controllers that interact with Kubernetes CustomResourceDefinitions.

## Building (optional)

The controller is pre-compiled and stored in Dockerhub. To build it, run:

`hack/build`

## Usage

1. Start a Kubernetes v1.7.0+ cluster and configure `kubectl` CLI with a valid kubeconfig.
2. Run `kubectl create -f hack/deploy.yaml` to create the CRD and start the controller.
3. Run `kubectl create -f hack/example.yaml` to create some example objects.

## Code Generation

Changing CRD specifications will require regenerating client code. To do so,
run the following:

`hack/update-codegen.sh`

You may check if generated code is out of date at any time by running:

`hack/verify-codegen.sh`

## Validation

In v1.9, CRD validation is a beta feature and is enabled, by default. In v1.8,
it is an alpha feature and must be explicitly enabled. To do so, start
`kube-apiserver` with the following flag appended:

`--feature-gates CustomResourceValidation=true`

If the feature is left disabled, no validation will occur at the apiserver. It
is up to the CRD creator / controller to validate the specification and react
accordingly.
