FROM --platform=linux/x86_64 golang:1.22 AS base

WORKDIR /src

COPY ./  .
ENV GOPROXY=https://proxy.golang.org
RUN --mount=type=cache,id=gomodcache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go mod download

FROM base AS builder

ENV CGO_ENABLED=0
ARG VERSION_VAR

RUN mkdir /app
COPY ./  /app/
WORKDIR /app
RUN --mount=type=cache,id=gomodcache,target=/go/pkg/mod \
    --mount=type=cache,id=gomodbuild,target=/root/.cache/go-build \
    --mount=type=bind,id=bind,target=. \
    DOCKER_BUILDKIT=1 go build -ldflags "-s -w -X 'main.version=${VERSION_VAR}'" -o /out/server .
FROM --platform=linux/x86_64 gcr.io/distroless/static-debian12:nonroot
LABEL maintainer="Jon Hadfield jon@lessknown.co.uk"
COPY public /app/public
WORKDIR /app
COPY --from=builder /out/server /app/server
EXPOSE 1323
ENTRYPOINT ["/app/server"]
