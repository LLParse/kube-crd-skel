#!/bin/bash

if [ "$IMAGE" == "" ]; then
  go build -o bin/vm-controller cmd/vm-controller/vm-controller.go
fi
