SOURCE_FILES?=$$(go list ./...)
TEST_PATTERN?=.
TEST_OPTIONS?=-race -v

setup:
	go get -u github.com/go-critic/go-critic/...
	go get -u github.com/psampaz/go-mod-outdated
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sudo sh -s -- -b $(go env GOPATH)/bin v1.46.1
	go get -u golang.org/x/tools/cmd/cover
	go install mvdan.cc/gofumpt@latest

test:
	echo 'mode: atomic' > coverage.txt && go list ./... | xargs -n1 -I{} sh -c 'go test -v -timeout=600s -covermode=atomic -coverprofile=coverage.tmp {} && tail -n +2 coverage.tmp >> coverage.txt' && rm coverage.tmp

cover: test fmt
	go tool cover -html=coverage.txt

fmt:
	find . -name '*.go' | while read -r file; do gofumpt -w "$$file"; done
lint:
	golangci-lint run ./...

ci: lint test

BUILD_TAG := $(shell git describe --tags 2>/dev/null)
BUILD_SHA := $(shell git rev-parse --short HEAD)
BUILD_DATE := $(shell date -u '+%Y/%m/%d:%H:%M:%S')

build:
	go build -ldflags '-s -w -X "main.version=[$(BUILD_TAG)-$(BUILD_SHA)] $(BUILD_DATE) UTC"' -o ".local_dist/ipthing_darwin_amd64"

build-all: fmt
	GOOS=darwin  CGO_ENABLED=0 GOARCH=amd64 go build -ldflags '-s -w -X "main.version=[$(BUILD_TAG)-$(BUILD_SHA)] $(BUILD_DATE) UTC"' -o ".local_dist/ipthing_darwin_amd64"
	GOOS=linux   CGO_ENABLED=0 GOARCH=amd64 go build -ldflags '-s -w -X "main.version=[$(BUILD_TAG)-$(BUILD_SHA)] $(BUILD_DATE) UTC"' -o ".local_dist/ipthing_linux_amd64"
	GOOS=linux   CGO_ENABLED=0 GOARCH=386 go build -ldflags '-s -w -X "main.version=[$(BUILD_TAG)-$(BUILD_SHA)] $(BUILD_DATE) UTC"' -o ".local_dist/ipthing_linux_386"
	GOOS=linux   CGO_ENABLED=0 GOARCH=arm go build -ldflags '-s -w -X "main.version=[$(BUILD_TAG)-$(BUILD_SHA)] $(BUILD_DATE) UTC"' -o ".local_dist/ipthing_linux_arm"
	GOOS=linux   CGO_ENABLED=0 GOARCH=arm64 go build -ldflags '-s -w -X "main.version=[$(BUILD_TAG)-$(BUILD_SHA)] $(BUILD_DATE) UTC"' -o ".local_dist/ipthing_linux_arm64"
	GOOS=netbsd  CGO_ENABLED=0 GOARCH=amd64 go build -ldflags '-s -w -X "main.version=[$(BUILD_TAG)-$(BUILD_SHA)] $(BUILD_DATE) UTC"' -o ".local_dist/ipthing_netbsd_amd64"
	GOOS=openbsd CGO_ENABLED=0 GOARCH=amd64 go build -ldflags '-s -w -X "main.version=[$(BUILD_TAG)-$(BUILD_SHA)] $(BUILD_DATE) UTC"' -o ".local_dist/ipthing_openbsd_amd64"
	GOOS=freebsd CGO_ENABLED=0 GOARCH=amd64 go build -ldflags '-s -w -X "main.version=[$(BUILD_TAG)-$(BUILD_SHA)] $(BUILD_DATE) UTC"' -o ".local_dist/ipthing_freebsd_amd64"
	GOOS=windows CGO_ENABLED=0 GOARCH=amd64 go build -ldflags '-s -w -X "main.version=[$(BUILD_TAG)-$(BUILD_SHA)] $(BUILD_DATE) UTC"' -o ".local_dist/ipthing_windows_amd64.exe"

build-linux-amd64: fmt
	GOOS=linux CGO_ENABLED=0 GOARCH=amd64 go build -ldflags '-s -w -X "main.version=[$(BUILD_TAG)-$(BUILD_SHA)] $(BUILD_DATE) UTC"' -o ".local_dist/ipthing_linux_amd64"

critic:
	gocritic check ./...

vet:
	go vet ./...

scan-image:
	trivy image ipthing:latest

mac-install: build
	install .local_dist/ipthing_darwin_amd64 /usr/local/bin/ipthing

linux-amd64-install: build-linux-amd64
	sudo install .local_dist/ipthing_linux_amd64 /usr/local/bin/ipthing

install:
	go install ./...

find-updates:
	go list -u -m -json all | go-mod-outdated -update -direct

NAME   := ghcr.io/jonhadfield/ipthing
TAG    := $(shell git rev-parse --short HEAD)
IMG    := ${NAME}:${TAG}
LATEST := ${NAME}:latest

build-docker:
	docker build --platform=linux/x86_64 --build-arg VERSION_VAR="[$(BUILD_TAG)-$(BUILD_SHA)] $(BUILD_DATE) UTC" -t ${IMG} .
	docker tag ${IMG} ${LATEST}
	docker tag ${LATEST} ipthing:latest

#release-docker: build-docker scan-image docker-push
release-docker: build-docker docker-push

docker-push: login
	docker --log-level debug push ${IMG}
	docker --log-level debug push ${LATEST}

redeploy:
	kubectl -n ipthing rollout restart deployments/ipthing

login:
	@echo ${CR_PAT} | docker login ghcr.io -u jonhadfield --password-stdin

help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := build
