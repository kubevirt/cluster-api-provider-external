#!/bin/bash
set -ex

ROOT_DIR="$(pwd)"
DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
TAR=$1

(
    cd $DIR
    cp Dockerfile Dockerfile.fencing-agents
    docker build -f Dockerfile.fencing-agents . -t fencing-agents:bazel
    rm -rf Dockerfile.fencing-agents
    docker save fencing-agents:bazel -o $ROOT_DIR/$TAR
)
