FROM golang:alpine as builder

RUN apk update \
 && apk add git curl \
 && apk add ca-certificates

RUN adduser -D -g '' user

COPY . $GOPATH/src/github.com/tomaszkiewicz/docker-copy-docker-image
WORKDIR $GOPATH/src/github.com/tomaszkiewicz/docker-copy-docker-image

RUN go get -u github.com/kardianos/govendor \
 && export GO_VERSION=$(go version | cut -d' ' -f3 | grep -Eo '[0-9]+\.[0-9]+\.[0-9]+') \
 && govendor sync

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -installsuffix cgo -ldflags="-w -s" -o /main

FROM alpine

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /etc/passwd /etc/passwd

COPY --from=builder /main /main

USER user
ENTRYPOINT ["/main"]