package repo

import "github.com/google/wire"

var Provider = wire.NewSet(
	NewRepo,
	NewRawQuery,
	NewResource,
	NewVersion,
	NewStorage,
)
