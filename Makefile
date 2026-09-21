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

BUILD_TAG := $(shell git describe --tags --abbrev=0 2>/dev/null || echo dev)
BUILD_SHA := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_DATE := $(shell date -u '+%Y/%m/%d:%H:%M:%S')
VERSION_LDFLAGS := -X "main.version=[$(BUILD_TAG)-$(BUILD_SHA)] $(BUILD_DATE) UTC" -X "main.buildTag=$(BUILD_TAG)" -X "main.buildSHA=$(BUILD_SHA)"

build:
	go build -ldflags '-s -w $(VERSION_LDFLAGS)' -o ".local_dist/ipthing_darwin_amd64"

build-all: fmt
	GOOS=darwin  CGO_ENABLED=0 GOARCH=amd64 go build -ldflags '-s -w $(VERSION_LDFLAGS)' -o ".local_dist/ipthing_darwin_amd64"
	GOOS=linux   CGO_ENABLED=0 GOARCH=amd64 go build -ldflags '-s -w $(VERSION_LDFLAGS)' -o ".local_dist/ipthing_linux_amd64"
	GOOS=linux   CGO_ENABLED=0 GOARCH=386 go build -ldflags '-s -w $(VERSION_LDFLAGS)' -o ".local_dist/ipthing_linux_386"
	GOOS=linux   CGO_ENABLED=0 GOARCH=arm go build -ldflags '-s -w $(VERSION_LDFLAGS)' -o ".local_dist/ipthing_linux_arm"
	GOOS=linux   CGO_ENABLED=0 GOARCH=arm64 go build -ldflags '-s -w $(VERSION_LDFLAGS)' -o ".local_dist/ipthing_linux_arm64"
	GOOS=netbsd  CGO_ENABLED=0 GOARCH=amd64 go build -ldflags '-s -w $(VERSION_LDFLAGS)' -o ".local_dist/ipthing_netbsd_amd64"
	GOOS=openbsd CGO_ENABLED=0 GOARCH=amd64 go build -ldflags '-s -w $(VERSION_LDFLAGS)' -o ".local_dist/ipthing_openbsd_amd64"
	GOOS=freebsd CGO_ENABLED=0 GOARCH=amd64 go build -ldflags '-s -w $(VERSION_LDFLAGS)' -o ".local_dist/ipthing_freebsd_amd64"
	GOOS=windows CGO_ENABLED=0 GOARCH=amd64 go build -ldflags '-s -w $(VERSION_LDFLAGS)' -o ".local_dist/ipthing_windows_amd64.exe"

build-linux-amd64: fmt
	GOOS=linux CGO_ENABLED=0 GOARCH=amd64 go build -ldflags '-s -w $(VERSION_LDFLAGS)' -o ".local_dist/ipthing_linux_amd64"

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
	DOCKER_BUILDKIT=1 docker build --platform=linux/x86_64 \
		--build-arg VERSION_VAR="[$(BUILD_TAG)-$(BUILD_SHA)] $(BUILD_DATE) UTC" \
		--build-arg BUILD_TAG="$(BUILD_TAG)" \
		--build-arg BUILD_SHA="$(BUILD_SHA)" \
		-t ${IMG} .
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

# Server management targets
start: ## Start the server in foreground
	go run .

start-background: ## Start the server in background
	@echo "Starting IPThing server in background..."
	@nohup go run . > ipthing.log 2>&1 & echo $$! > ipthing.pid
	@echo "Server started with PID: $$(cat ipthing.pid)"
	@echo "Logs are being written to ipthing.log"

stop: ## Stop the background server
	@if [ -f ipthing.pid ]; then \
		PID=$$(cat ipthing.pid); \
		if kill -0 $$PID 2>/dev/null; then \
			echo "Stopping IPThing server (PID: $$PID)..."; \
			pkill -P $$PID 2>/dev/null || true; \
			kill $$PID 2>/dev/null || true; \
			rm -f ipthing.pid; \
			echo "Server stopped."; \
		else \
			echo "No server running with PID: $$PID"; \
			rm -f ipthing.pid; \
		fi \
	else \
		echo "No PID file found. Attempting to find and stop ipthing processes..."; \
		pkill -f "go run ." || echo "No running ipthing process found."; \
	fi

restart: stop start-background ## Restart the server in background

status: ## Check if the server is running
	@if [ -f ipthing.pid ]; then \
		PID=$$(cat ipthing.pid); \
		if kill -0 $$PID 2>/dev/null; then \
			echo "IPThing server is running (PID: $$PID)"; \
		else \
			echo "IPThing server is not running (stale PID file: $$PID)"; \
			rm -f ipthing.pid; \
		fi \
	else \
		if pgrep -f "go run ." >/dev/null; then \
			echo "IPThing server is running (no PID file)"; \
		else \
			echo "IPThing server is not running"; \
		fi \
	fi

logs: ## Tail the server logs
	@if [ -f ipthing.log ]; then \
		tail -f ipthing.log; \
	else \
		echo "No log file found. Start the server with 'make start-background' first."; \
	fi

clean-logs: ## Clean server logs and PID file
	rm -f ipthing.log ipthing.pid

help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := build
