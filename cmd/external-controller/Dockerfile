# Reproducible builder image
FROM golang:1.10.0 as builder

WORKDIR /go/src/github.com/kubevirt/cluster-api-provider-external

# This expects that the context passed to the docker build command is
# the cluster-api-provider-gcp directory.
# e.g. docker build -t <tag> -f <this_Dockerfile> <path_to_cluster-api-provider-gcp>
COPY . . 
RUN CGO_ENABLED=0 GOOS=linux go install -a -ldflags '-extldflags "-static"' github.com/kubevirt/cluster-api-provider-external/cmd/external-controller
#RUN find / -name external-controller

# Final container
FROM centos:centos7

COPY --from=builder /go/bin/external-controller /bin/
#CMD ["/go/bin/external-controller", "-alsologtostderr"]

