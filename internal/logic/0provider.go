package logic

import (
	"github.com/MirrorChyan/resource-backend/internal/logic/dispense"
	"github.com/google/wire"
)

var Provider = wire.NewSet(
	NewResourceLogic,
	NewVersionLogic,
	NewStorageLogic,
	dispense.NewDistributeLogic,
)
