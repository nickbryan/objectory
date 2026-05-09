.DEFAULT_GOAL := help

# Colours used in help
GREEN    := $(shell tput -Txterm setaf 2)
WHITE    := $(shell tput -Txterm setaf 7)
YELLOW   := $(shell tput -Txterm setaf 3)
RESET    := $(shell tput -Txterm sgr0)

HELP_FUN = %help; \
	while(<>) { push @{$$help{$$2 // 'Misc'}}, [$$1, $$3] \
	if /^([a-zA-Z\-]+)\s*:.*\#\#(?:@([a-zA-Z\-]+))?\s(.*)$$/ }; \
	for (sort keys %help) { \
	print "${WHITE}$$_${RESET}\n"; \
	for (@{$$help{$$_}}) { \
	$$sep = " " x (32 - length $$_->[0]); \
	print "  ${YELLOW}$$_->[0]${RESET}$$sep${GREEN}$$_->[1]${RESET}\n"; \
	}; \
	print "\n"; } \
	$$sep = " " x (32 - length "help"); \
	print "${WHITE}Options${RESET}\n"; \
	print "  ${YELLOW}help${RESET}$$sep${GREEN}Prints this help${RESET}\n";

help:
	@echo "\nUsage: make ${YELLOW}<target>${RESET}\n\nThe following targets are available:\n";
	@perl -e '$(HELP_FUN)' $(MAKEFILE_LIST)

guard-%:
	@ if [ "${${*}}" = "" ]; then \
		echo "Required variable ['$*'] is not set!"; \
		exit 1; \
	fi

start: ##@Run
	docker compose up -d --remove-orphans $(start_args)

stop: ##@Run
	docker compose down -v

rebuild: ##@Run
	make stop
	make start start_args="--force-recreate --build"

watch: ##@Run
	docker compose up --remove-orphans

logs: ##@Run
	docker compose logs -f

tidy: ##@Go
	docker compose exec api go mod tidy

update: ##@Go
	docker compose exec api go get -u

testdbname=objectory_test
test: ##@Test
	@make -s psql cmd="CREATE DATABASE $(testdbname)"
	@make -s migrate dbname=$(testdbname)
	-docker compose exec api go test -count=1 ./...
	@make -s psql cmd="DROP DATABASE $(testdbname)"

psql: guard-cmd ##@Database
	docker compose exec database psql "postgres://objectory:secret@localhost:5432/objectory?sslmode=disable" -c "$(cmd)"

lint: ##@Test
	go tool golangci-lint run $(args) ./...

lint-fix: ##@Test
	make lint args="--fix"

dbname?=objectory
migrate: ##@Database
	go tool goose -dir api/internal/storage/postgres/migrations postgres \
		"postgres://objectory:secret@localhost:5432/$(dbname)?sslmode=disable" up

migration: guard-name ##@Database
	go tool goose -dir api/internal/storage/postgres/migrations create $(name) sql

sqlc: ##@Database
	go tool sqlc generate
