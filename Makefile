.DEFAULT_GOAL := run-docker

VERSION ?= dev
BUILD_TIME ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
OUTPUT_PATH ?= bin/gachigazer

run-docker: build-dev docker-build docker-run

run:
	. venv/bin/activate && \
	pip install -r requirements.txt && \
	go run cmd/bot/main.go

mock:
	mockery

test:
	gotestsum --format-icons hivis ./...

test-ci: mock
	go test ./...

build-dev:
	CGO_ENABLED=0 go build -ldflags="-X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME)" -o $(OUTPUT_PATH) ./cmd/bot/main.go

build-release:
	CGO_ENABLED=0 go build -ldflags="-w -s -X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME)" -trimpath -o $(OUTPUT_PATH) ./cmd/bot/main.go

docker-build: build-dev
	docker build --network host -t gg -f docker/Dockerfile .

docker-run: docker-build
	docker compose up

docker-stop:
	docker compose down

docker-sh:
	docker exec -it gg sh
