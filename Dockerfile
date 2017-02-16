FROM golang:1.8-alpine

ENV WORKING_DIR /go/src/github.com/marcusmartins/reg

RUN apk add --no-cache bash

WORKDIR $WORKING_DIR
COPY . $WORKING_DIR

RUN go install ./...

ENTRYPOINT ["/bin/bash"]
