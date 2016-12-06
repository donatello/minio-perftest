FROM golang:alpine

RUN ["apk", "add", "--no-cache", "git"]

RUN ["go", "get", "-u", "github.com/minio/minio-go"]

COPY ./uploadsperftest.go /root/

WORKDIR /root

ENTRYPOINT ["go", "run", "uploadsperftest.go"]
