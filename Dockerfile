FROM golang:latest

WORKDIR /go/src/app
COPY . .

RUN go install -v ./...

VOLUME /var/lib/default.etcd

CMD ["app"]