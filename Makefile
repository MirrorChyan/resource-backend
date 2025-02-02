.PHONY: entgen wiregen build

entgen:
	go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./internal/ent/schema

wiregen:
	wire gen ./internal/wire

build:
	go build -o ./bin/app .