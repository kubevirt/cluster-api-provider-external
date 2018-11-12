#!/bin/bash
set -ex

docker build . -t docker.io/kubevirt/fence-agents:28
docker push docker.io/kubevirt/fence-agents:28
