.PHONY: dev_pg_up dev_pg_down dev_pg_log lint

dev_pg_up:
	docker-compose down --remove-orphans
	docker-compose up -d --build --force-recreate --remove-orphans postgres

dev_pg_down:
	docker-compose down --remove-orphans

dev_pg_log:
	docker-compose logs -f

lint:
	golangci-lint run