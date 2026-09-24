up:
	docker compose up --build

down:
	docker compose down

logs:
	docker compose logs -f

test:
	docker compose run --rm backend go test ./...

test-backend:
	docker compose run --rm backend go test ./...

test-frontend:
	docker compose run --rm frontend npm test -- --run

lint:
	docker compose run --rm frontend npm run lint

format:
	docker compose run --rm backend gofmt -w ./cmd

build:
	docker compose build

seed:
	@echo "Seed pendiente para una iteración posterior."
