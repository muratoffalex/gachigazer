#!/bin/bash

set -e

# Define the directory where the script is located
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DOCKER_COMPOSE_DIR="$(dirname "$SCRIPT_DIR")"

cd "$DOCKER_COMPOSE_DIR"

# Check for the presence of the previous version
if [ ! -f deployments/previous/version ]; then
    echo "No previous version found"
    exit 1
fi

# Stop the current containers
docker compose down

# Rotation of directories with versions
rm -rf deployments/tmp
mv deployments/current deployments/tmp
mv deployments/previous deployments/current
mv deployments/tmp deployments/previous

# Update symbolic links
ln -sf deployments/current/docker-compose.yml docker-compose.yml
ln -sf deployments/current/rollback.sh rollback.sh

# Starting a container with the previous version
export IMAGE_TAG=$(cat deployments/current/version)
export CONTAINER_UID=$(id -u)
export CONTAINER_GID=$(id -g)
docker compose pull && docker compose up -d

echo "Successfully rolled back to version: $IMAGE_TAG"
