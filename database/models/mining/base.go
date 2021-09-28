package mining

import "github.com/mcdexio/mai3-trade-mining-watcher/database/models"

// AllModels collects available models.
var AllModels = []interface{}{
	&models.System{},

	&Progress{},
	&Schedule{},
	&UserInfo{},
}
