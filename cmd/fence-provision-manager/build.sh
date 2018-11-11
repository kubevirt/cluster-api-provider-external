#!/bin/bash
set -ex

ROOT_DIR="$(pwd)"
DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
TAR=$1

(
    cd $DIR
    cp Dockerfile Dockerfile.fence-provision-manager
    docker build -f Dockerfile.fence-provision-manager . -t fence-provision-manager:bazel
    rm -rf Dockerfile.fence-provision-manager
    docker save fence-provision-manager:bazel -o $ROOT_DIR/$TAR
)
