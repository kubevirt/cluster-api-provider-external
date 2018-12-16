#!/bin/bash

set -e

source hack/common.sh
source cluster/$CLUSTER_PROVIDER/provider.sh

echo "Cleaning up ..."

# Remove finalizers from all machines, to not block the cleanup
_kubectl -n cluster-api-provider-baremetal get machines -o=custom-columns=NAME:.metadata.name,FINALIZERS:.metadata.finalizers --no-headers | grep "machine.cluster.k8s.io" | while read p; do
    arr=($p)
    name="${arr[0]}"
    _kubectl -n cluster-api-provider-baremetal delete machine $name --wait=false
    _kubectl -n cluster-api-provider-baremetal patch machine $name --type=json -p '[{ "op": "remove", "path": "/metadata/finalizers" }]'
done

# Remove finalizers from all cluster, to not block the cleanup
_kubectl -n cluster-api-provider-baremetal get clusters -o=custom-columns=NAME:.metadata.name,FINALIZERS:.metadata.finalizers --no-headers | grep "cluster.cluster.k8s.io" | while read p; do
    arr=($p)
    name="${arr[0]}"
    _kubectl -n cluster-api-provider-baremetal delete cluster $name --wait=false
    _kubectl -n cluster-api-provider-baremetal patch cluster $name --type=json -p '[{ "op": "remove", "path": "/metadata/finalizers" }]'
done

_kubectl -n cluster-api-provider-baremetal delete deployment -l 'cluster-api-provider-baremetal.kubevirt.io'
_kubectl -n cluster-api-provider-baremetal delete configmap -l 'cluster-api-provider-baremetal.kubevirt.io'
_kubectl -n cluster-api-provider-baremetal delete rs -l 'cluster-api-provider-baremetal.kubevirt.io'
_kubectl -n cluster-api-provider-baremetal delete pods -l 'cluster-api-provider-baremetal.kubevirt.io'
_kubectl -n cluster-api-provider-baremetal delete clusterrolebinding -l 'cluster-api-provider-baremetal.kubevirt.io'
_kubectl -n cluster-api-provider-baremetal delete clusterroles -l 'cluster-api-provider-baremetal.kubevirt.io'
_kubectl -n cluster-api-provider-baremetal delete serviceaccounts -l 'cluster-api-provider-baremetal.kubevirt.io'
_kubectl -n cluster-api-provider-baremetal delete customresourcedefinitions -l 'cluster-api-provider-baremetal.kubevirt.io'

# Remove cluster-api-provider-baremetal namespace
if [ "$(_kubectl get ns | grep cluster-api-provider-baremetal)" ]; then
    echo "Clean cluster-api-provider-baremetal namespace"
    _kubectl delete ns cluster-api-provider-baremetal

    current_time=0
    sample=10
    timeout=120
    echo "Waiting for cluster-api-provider-baremetal namespace to dissappear ..."
    while  [ "$(_kubectl get ns | grep cluster-api-provider-baremetal)" ]; do
        sleep $sample
        current_time=$((current_time + sample))
        if [ $current_time -gt $timeout ]; then
            exit 1
        fi
    done
fi
