.DEFAULT_GOAL := run-docker

VERSION ?= dev
BUILD_TIME := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

run-docker: build-linux docker-build docker-run

run:
	. venv/bin/activate && \
	pip install -r requirements.txt && \
	go run cmd/bot/main.go

build-linux:
	CGO_ENABLED=0 go build -p 4 -ldflags="-w -s -X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME)" -o bin/gachigazer ./cmd/bot/main.go

docker-build:
	docker build --network host -t gg -f docker/Dockerfile .

docker-run:
	docker run --network host -v ./gachigazer.toml:/app/gachigazer.toml -v ./bot.db:/app/data/bot.db -v ~/.cache/go-ytdlp/:/root/.cache/go-ytdlp gg

