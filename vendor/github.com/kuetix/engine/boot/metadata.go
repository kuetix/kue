package boot

import (
	"github.com/kuetix/engine/engine/domain/interfaces"
)

var MetaFunctionCache = map[string]map[string]map[string]interfaces.FunctionMetadata{}

// AddMetaFunctionCache adds metadata to the cache
//
//goland:noinspection GoUnusedExportedFunction
func AddMetaFunctionCache(metadata map[string]map[string]map[string]interfaces.FunctionMetadata) {
	for service, transitions := range metadata {
		if MetaFunctionCache[service] == nil {
			MetaFunctionCache[service] = map[string]map[string]interfaces.FunctionMetadata{}
		}
		for transition, functions := range transitions {
			if MetaFunctionCache[service][transition] == nil {
				MetaFunctionCache[service][transition] = map[string]interfaces.FunctionMetadata{}
			}
			for funcName, meta := range functions {
				MetaFunctionCache[service][transition][funcName] = meta
			}
		}
	}
}
