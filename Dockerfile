FROM --platform=linux/x86_64 golang:1.22 AS base

WORKDIR /src

COPY ./  .
ENV GOPROXY=https://proxy.golang.org
RUN --mount=type=cache,id=s-b9528c46-a1b7-4e62-853b-d7b0307531e5-go-pkg-mod,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go mod download

FROM base AS builder

ENV CGO_ENABLED=0
ARG VERSION_VAR

RUN mkdir /app
COPY ./  /app/
WORKDIR /app
RUN --mount=type=cache,id=s-b9528c46-a1b7-4e62-853b-d7b0307531e5-go-pkg-mod,target=/go/pkg/mod \
    --mount=type=cache,id=s-b9528c46-a1b7-4e62-853b-d7b0307531e5-go-build,target=/root/.cache/go-build \
    --mount=type=bind,target=. \
    go build -ldflags "-s -w -X 'main.version=${VERSION_VAR}'" -o /out/server .
LABEL maintainer="Jon Hadfield jon@lessknown.co.uk"
COPY public /app/public
WORKDIR /app
COPY --from=builder /out/server /app/server
EXPOSE 1323
ENTRYPOINT ["/app/server"]
