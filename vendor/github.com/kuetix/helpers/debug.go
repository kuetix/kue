package helpers

import "encoding/json"

// DebugAsPrettyJsonToBytes returns the lookup context as a pretty json string
//
//goland:noinspection GoUnusedExportedFunction
func DebugAsPrettyJsonToBytes(lookupContext any) ([]byte, error, error) {
	mapLookupContext, errMap := ToMapRecursive(lookupContext)
	jsonLookupContext, errJson := json.MarshalIndent(mapLookupContext, "", "  ")
	return jsonLookupContext, errJson, errMap
}
