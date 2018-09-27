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

build:
	CGO_ENABLED=0 go build -a -ldflags '-extldflags "-static"' github.com/kubevirt/cluster-api-provider-external/cmd/external-controller

all: generate install images

.PHONY: depend
depend:
	dep version || go get -u github.com/golang/dep/cmd/dep
	dep ensure -v

	# go libraries often ship BUILD and BUILD.bazel files, but they often don't work.
	# We delete them and regenerate them
	find vendor -name "BUILD" -delete
	find vendor -name "BUILD.bazel" -delete

	bazel run //:gazelle

depend-update: work
	dep ensure -update

generate: gendeepcopy

gendeepcopy:
	go build -o $$GOPATH/bin/deepcopy-gen github.com/kubevirt/cluster-api-provider-external/vendor/k8s.io/code-generator/cmd/deepcopy-gen
	deepcopy-gen \
	  -i ./cloud/external/providerconfig/v1alpha1 \
	  -O zz_generated.deepcopy \
	  -h boilerplate.go.txt
	 #--logtostderr -v 9

install: depend
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
