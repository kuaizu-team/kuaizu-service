.PHONY: run build generate tidy run-admin build-admin

REDOCLY_CLI_VERSION := 2.67.1

run:
	go run cmd/server/main.go

build:
	go build -o bin/kuaizu-server cmd/server/main.go

run-admin:
	go run cmd/admin/main.go

build-admin:
	go build -o bin/kuaizu-admin cmd/admin/main.go

generate:
	npx --yes @redocly/cli@$(REDOCLY_CLI_VERSION) bundle api/service/openapi.yaml --output api/service/openapi-bundled.yaml
	go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen --config=api/service/config.yaml api/service/openapi-bundled.yaml

tidy:
	go mod tidy
