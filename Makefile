.PHONY: up down test web-build api-test compose-config

up:
	docker compose up --build

down:
	docker compose down

test:
	cd api && go test ./...

api-test:
	cd api && go test ./...

web-build:
	cd web && npm install && npm run lint && npm run build

compose-config:
	docker compose config

