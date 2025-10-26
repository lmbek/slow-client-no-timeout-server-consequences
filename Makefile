.PHONY: help build run up down logs ps rebuild stop

# Simple helpers to build and run the PHP server
# Requires Docker. For compose-based workflows, Docker Desktop (with Compose plugin) is enough.

help:
	@echo "Targets:"
	@echo "  make build    - Build the slow-php image from php-server/"
	@echo "  make run      - Run the container in the foreground on localhost:8080 (Ctrl+C to stop)"
	@echo "  make up       - docker compose up -d (build if needed, run in background)"
	@echo "  make down     - docker compose down (stop and remove)"
	@echo "  make logs     - Follow logs from the compose service"
	@echo "  make ps       - Show compose services status"
	@echo "  make rebuild  - Force rebuild via compose (no cache)"

# Build the image using the Dockerfile in php-server/
build:
	docker build -t slow-php ./php-server

# Run the built image mapping host 8080 -> container 8080, remove on exit
run:
	docker run --rm -p 8080:8080 slow-php

# Compose conveniences (uses docker-compose.yml in repo root)
up:
	docker compose up --build -d

down:
	docker compose down

logs:
	docker compose logs -f

ps:
	docker compose ps

rebuild:
	docker compose build --no-cache
