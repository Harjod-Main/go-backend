# syntax=docker/dockerfile:1.7
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS build

ARG TARGETOS
ARG TARGETARCH
ARG GIT_COMMIT

RUN apk add --no-cache ca-certificates

WORKDIR /src

COPY go.mod go.sum ./

# Modules are public (proxy.golang.org). No private Git credentials in the image.
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -a -installsuffix cgo \
      -ldflags "-s -w -extldflags -static -X main.commit=${GIT_COMMIT}" \
      -o /out/api .

FROM alpine:3.23

RUN addgroup -S app && adduser -S -G app -u 10001 app \
  && apk add --no-cache tini ca-certificates tzdata \
  && cp /usr/share/zoneinfo/Asia/Bangkok /etc/localtime \
  && echo "Asia/Bangkok" > /etc/timezone \
  && update-ca-certificates

WORKDIR /home/app

COPY --from=build /out/api /home/app/api

USER app

ENTRYPOINT ["/sbin/tini", "--"]
CMD ["/home/app/api"]
