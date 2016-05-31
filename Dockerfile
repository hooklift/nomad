FROM golang:1.6.2-wheezy
ENV GOPATH /work
ENV PATH $PATH:$GOPATH/bin
ARG VERSION

WORKDIR /work/src/github.com/hashicorp
RUN git clone -b hooklift https://github.com/hooklift/nomad.git
RUN go get -u \
	github.com/ugorji/go/codec/codecgen \
	github.com/mitchellh/gox
RUN sed -i -E "s/(const Version ).+/\1=\"${VERSION}\"/" nomad/version.go
RUN cd nomad && \
	make dev
