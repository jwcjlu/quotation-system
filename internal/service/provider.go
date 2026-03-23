package service

import "github.com/google/wire"

var ProviderSet = wire.NewSet(NewBomService, NewAgentService)
