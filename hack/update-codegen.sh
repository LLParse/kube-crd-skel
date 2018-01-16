#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

SCRIPT_ROOT=$(dirname ${BASH_SOURCE})/..
DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
CODEGEN_PKG=${CODEGEN_PKG:-$(cd ${SCRIPT_ROOT}; ls -d -1 ./vendor/k8s.io/code-generator 2>/dev/null || echo $GOPATH/src/k8s.io/code-generator)}

cd ${CODEGEN_PKG}
./generate-groups.sh "deepcopy,client,informer,lister" \
  github.com/llparse/kube-crd-skel/pkg/client github.com/llparse/kube-crd-skel/pkg/apis \
  virtualmachine:v1alpha1 \
  --go-header-file ${DIR}/boilerplate.txt
cd -
