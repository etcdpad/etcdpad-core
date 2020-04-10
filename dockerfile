FROM golang:1.14.1-alpine3.11 as builder

RUN mkdir -p /go/src/github.com/etcdpad/etcdpad-core
COPY . /go/src/github.com/etcdpad/etcdpad-core
WORKDIR /go/src/github.com/etcdpad/etcdpad-core

RUN apk add --update bash curl tzdata \
    && cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime \
    && echo 'Asia/Shanghai' > /etc/timezone \
    && GOPROXY=https://goproxy.cn GO111MODULE=on go build -o app app.go

FROM golang:1.14.1-alpine3.11
RUN apk add --update bash curl tzdata \
    && cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime \
    && echo 'Asia/Shanghai' > /etc/timezone

COPY --from=builder /go/src/github.com/etcdpad/etcdpad-core/app /usr/local/bin/etcdpad-core

WORKDIR /usr/local/bin
ENTRYPOINT ["/usr/local/bin/etcdpad-core"]
CMD ["-port", "8989", "-stdout", "true"]