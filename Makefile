# Copyright 2018 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

.PHONY: gendeepcopy

NS=fenced
KUBECTL=kubectl
YAML=examples/crd.yaml examples/storage.yaml examples/demo.yaml 

bazel-generate:
	SYNC_VENDOR=true hack/dockerized "bazel run :gazelle"

build:
	CGO_ENABLED=0 go build -a -ldflags '-extldflags "-static"' github.com/kubevirt/cluster-api-provider-external/cmd/external-controller

all: generate install images

depend-update: work
	dep ensure -update

deps-install:
	SYNC_VENDOR=true hack/dockerized "dep ensure -v"

deps-update:
	SYNC_VENDOR=true hack/dockerized "dep ensure -v -update"

distclean: clean
	hack/dockerized "rm -rf vendor/ && rm -f Gopkg.lock"
	rm -rf vendor/

generate: gendeepcopy

gendeepcopy:
	go build -o $$GOPATH/bin/deepcopy-gen github.com/kubevirt/cluster-api-provider-external/vendor/k8s.io/code-generator/cmd/deepcopy-gen
	deepcopy-gen \
	  -i ./cloud/external/providerconfig/v1alpha1 \
	  -O zz_generated.deepcopy \
	  -h boilerplate.go.txt
	 #--logtostderr -v 9

nstall: depend
	CGO_ENABLED=0 go install -a -ldflags '-extldflags "-static"' github.com/kubevirt/cluster-api-provider-external/cmd/external-controller

images:
	$(MAKE) -C cmd/external-controller image
	$(MAKE) -C examples/agents image

push:
	$(MAKE) -C cmd/external-controller push
	$(MAKE) -C examples/agents push

check: depend fmt vet

test:
	go test -race -cover ./cmd/... ./clusterctl/... ./cloud/...

fmt:
	hack/verify-gofmt.sh

vet:
	go vet ./...

ns:
	echo KUBECTL=$(KUBECTL)
	-$(KUBECTL) create ns $(NS)
	examples/rbac/create_role.sh  --namespace $(NS) --role-name $(NS)-actuator --role-binding-name $(NS)-actuator

e2e: ns
	for yaml in $(YAML); do \
		echo "Applying $$yaml";\
		$(KUBECTL) -n $(NS) create -f $$yaml ;\
	done

clean:
	# Delete stuff, wait for the pods to die, then delete the entire namespace
	-$(KUBECTL) -n $(NS) delete deploy,jobs,rs,pods --all
	-$(KUBECTL) -n $(NS) delete machine,cluster --all
	-$(KUBECTL) -n $(NS) delete crd --all
	while [ "x$$($(KUBECTL) -n $(NS) get po 2>/dev/null)" != "x" ]; do sleep 5; /bin/echo -n .; done
	-$(KUBECTL) delete ns/$(NS) clusterrole/$(NS)-actuator clusterrolebinding/$(NS)-actuator
	while [ "x$$($(KUBECTL) get ns $(NS) 2>/dev/null)" != "x" ]; do sleep 5; /bin/echo -n .; done

.PHONY: bazel-generate deps-install deps-update distclean
