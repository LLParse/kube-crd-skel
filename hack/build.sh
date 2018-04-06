#!/bin/bash -e

REPO=${REPO:-llparse}

if [ "$IMAGE" == "" ]; then
  go build -o bin/vm-controller cmd/vm-controller/main.go
else
  GOOS=linux GOARCH=amd64 go build -o bin/image/vm-controller cmd/vm-controller/main.go
  cp -f hack/Dockerfile bin/image/

  image_tag="$REPO/ranchervm-controller:dev"

  docker build -t ${image_tag} bin/image
  echo
  read -p "Push ${image_tag} (y/n)? " choice
  case "$choice" in 
    y|Y ) docker push ${image_tag} ;;
    * ) ;;
  esac
#  rm -r bin/image
fi
