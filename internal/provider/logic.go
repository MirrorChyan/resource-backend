package provider

import (
	"github.com/MirrorChyan/resource-backend/internal/logic"
	"github.com/MirrorChyan/resource-backend/internal/logic/dispense"
	"github.com/google/wire"
)

var LogicSet = wire.NewSet(
	logic.NewResourceLogic,
	logic.NewVersionLogic,
	logic.NewLatestVersionLogic,
	logic.NewStorageLogic,
	dispense.NewDistributeLogic,
)
