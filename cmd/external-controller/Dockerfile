FROM centos:centos7
RUN yum install -y which docker golang git make

ARG GOPATH
ARG HOME
ENV GOPATH=${GOPATH:-/root/go} HOME=${HOME:-/root}

WORKDIR $GOPATH/src/github.com/kubevirt/cluster-api-provider-external
COPY . .

RUN CGO_ENABLED=0 GOOS=linux go install -a -ldflags '-extldflags "-static"' github.com/kubevirt/cluster-api-provider-external/cmd/external-controller
#RUN make install

WORKDIR ..
RUN rm -rf cluster-api-provider-external

#CMD ["/go/bin/external-controller", "-alsologtostderr"]
