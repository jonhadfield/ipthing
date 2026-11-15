FROM --platform=linux/x86_64 golang:1.26.11 AS base

WORKDIR /src

COPY ./ .
ENV GOPROXY=https://proxy.golang.org
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go mod download

FROM base AS builder

ARG VERSION_VAR=dev
ARG BUILD_TAG=dev
ARG BUILD_SHA=unknown
ENV CGO_ENABLED=0

RUN mkdir /app
COPY ./ /app/
WORKDIR /app

RUN go build -ldflags "-s -w -X 'main.version=${VERSION_VAR}' -X 'main.buildTag=${BUILD_TAG}' -X 'main.buildSHA=${BUILD_SHA}'" -o /out/server .

FROM --platform=linux/x86_64 gcr.io/distroless/static-debian12:nonroot

LABEL maintainer="Jon Hadfield jon@lessknown.co.uk"

WORKDIR /app
COPY --from=builder /out/server /app/server

# Defaults match config (HTTP 8080 / HTTPS 443). UDP 443 is HTTP/3 when AutoTLS is enabled.
EXPOSE 8080 443/tcp 443/udp
ENTRYPOINT ["/app/server"]
