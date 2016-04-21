FROM golang:1.6.2-wheezy
ENV GOPATH /work
ENV PATH $PATH:$GOPATH/bin

WORKDIR /work/src/github.com/hashicorp
RUN git clone -b hooklift https://github.com/hooklift/nomad.git
RUN go get -u \
	github.com/ugorji/go/codec/codecgen \
	github.com/mitchellh/gox
RUN cd nomad && \
	make dev
