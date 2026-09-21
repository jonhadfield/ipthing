FROM --platform=linux/x86_64 golang:1.24.4 AS base

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

### ✅ This was missing before!
FROM --platform=linux/x86_64 gcr.io/distroless/static-debian12:nonroot

LABEL maintainer="Jon Hadfield jon@lessknown.co.uk"

COPY public /app/public
WORKDIR /app
COPY --from=builder /out/server /app/server

EXPOSE 1323
ENTRYPOINT ["/app/server"]
