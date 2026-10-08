.PHONY: entgen wiregen build

entgen:
	@go generate ./internal/ent

wiregen:
	@wire gen ./internal/wire

build:
	@go build -o ./bin/app .