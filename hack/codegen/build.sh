#!/bin/bash -e

REPO=${REPO:-llparse}
NAME=k8s-codegen
VERSION=1.8

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
TAG=${REPO}/${NAME}:${VERSION}

docker build -t ${TAG} ${DIR}
docker push ${TAG}
