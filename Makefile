up:
	docker compose up --build
down:
	docker compose down
logs:
	docker compose logs -f
ps:
	docker compose ps
test:
	cd backend && go test ./...
