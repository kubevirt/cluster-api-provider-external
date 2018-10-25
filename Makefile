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

NS=fenced
KUBECTL=kubectl
YAML=examples/crd.yaml examples/storage.yaml examples/demo.yaml 

bazel-generate:
	SYNC_VENDOR=true hack/dockerized "bazel run :gazelle"

bazel-push-images-release:
	hack/dockerized "bazel run //:push_images --define release=true"

deps-install:
	SYNC_VENDOR=true hack/dockerized "dep ensure -v"

deps-update:
	SYNC_VENDOR=true hack/dockerized "dep ensure -v -update"

distclean: clean
	hack/dockerized "rm -rf vendor/ && rm -f Gopkg.lock"
	rm -rf vendor/

generate:
	hack/dockerized "hack/update-codegen.sh"

check: fmt vet

test:
	go test -race -cover ./cmd/... ./clusterctl/... ./cloud/...

fmt:
	hack/verify-gofmt.sh

vet:
	go vet ./...

.PHONY: bazel-generate \
	bazel-push-images-release \
	deps-install \
	deps-update \
	distclean \
	generate
