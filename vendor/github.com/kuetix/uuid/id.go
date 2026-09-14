package uuid

import (
	"crypto/sha256"
	"encoding/base64"
	"strings"

	"github.com/google/uuid"
)

func IdV5(identity string) uuid.UUID {
	namespace := uuid.NameSpaceOID
	hashId := sha256.Sum256([]byte(identity))
	uuidV5 := uuid.NewHash(sha256.New(), namespace, hashId[:], 5)

	return uuidV5
}

//goland:noinspection GoUnusedExportedFunction
func Base64Id(identity string) string {
	binary, _ := IdV5(identity).MarshalBinary()
	return base64.StdEncoding.EncodeToString(binary)
}

func UUId(identity string) string {
	return IdV5(identity).String()
}

func Id(identity string) string {
	return UId(UUId(identity))
}

func UId(id string) string {
	ui := uuid.MustParse(id)
	return strings.ReplaceAll(ui.String(), "-", "")
}

//goland:noinspection GoUnusedExportedFunction
func Parsed(uid string) string {
	uuidV5, _ := uuid.Parse(uid)

	return uuidV5.String()
}

//goland:noinspection GoUnusedExportedFunction
func UUIDToInt(u uuid.UUID) []byte {
	// Convert UUID to bytes
	uuidBytes := u[:]

	return uuidBytes
}
