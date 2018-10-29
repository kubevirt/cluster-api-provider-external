bazel-generate:
	SYNC_VENDOR=true hack/dockerized "bazel run :gazelle"

bazel-generate-manifests-dev:
	SYNC_MANIFESTS=true hack/dockerized "bazel build //manifests:generate_manifests --define dev=true"

bazel-generate-manifests-release:
	SYNC_MANIFESTS=true hack/dockerized "bazel build //manifests:generate_manifests --define release=true"

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

fmt:
	hack/verify-gofmt.sh

vet:
	go vet ./...

.PHONY: bazel-generate \
	bazel-generate-manifests-dev \
	bazel-generate-manifests-release \
	bazel-push-images-release \
	deps-install \
	deps-update \
	distclean \
	generate
