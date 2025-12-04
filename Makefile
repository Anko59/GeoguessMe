.PHONY: up down logs test backend-init

up:
	docker compose up -d

dev:
	docker compose up

down:
	docker compose down

logs:
	docker compose logs -f

test: test-backend test-frontend

test-backend:
	cd backend && go test ./...

test-frontend:
	cd frontend && npm test

test-integration:
	cd backend && go test ./integration_test/...

backend-init:
	mkdir -p backend
	cd backend && go mod init geoguessme
