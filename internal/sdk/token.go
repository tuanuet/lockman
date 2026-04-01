package sdk

import (
	"encoding/base64"
	"encoding/binary"
	"errors"
	"strings"
)

const holdTokenPrefix = "h1_"

var errInvalidHoldToken = errors.New("invalid hold token")

// EncodeHoldToken encodes resource keys and owner identity as a compact hold token.
func EncodeHoldToken(resourceKeys []string, ownerID string) (string, error) {
	if len(resourceKeys) > int(^uint16(0)) {
		return "", errInvalidHoldToken
	}

	payload := make([]byte, 0)
	payload = binary.BigEndian.AppendUint16(payload, uint16(len(resourceKeys)))
	for _, key := range resourceKeys {
		if len(key) > int(^uint16(0)) {
			return "", errInvalidHoldToken
		}
		payload = binary.BigEndian.AppendUint16(payload, uint16(len(key)))
		payload = append(payload, key...)
	}

	if len(ownerID) > int(^uint16(0)) {
		return "", errInvalidHoldToken
	}
	payload = binary.BigEndian.AppendUint16(payload, uint16(len(ownerID)))
	payload = append(payload, ownerID...)

	return holdTokenPrefix + base64.RawURLEncoding.EncodeToString(payload), nil
}

// DecodeHoldToken decodes a hold token into resource keys and owner identity.
func DecodeHoldToken(token string) (resourceKeys []string, ownerID string, err error) {
	if token == "" || !strings.HasPrefix(token, holdTokenPrefix) {
		return nil, "", errInvalidHoldToken
	}

	decoded, decodeErr := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(token, holdTokenPrefix))
	if decodeErr != nil {
		return nil, "", errInvalidHoldToken
	}

	offset := 0
	readUint16 := func() (uint16, error) {
		if len(decoded)-offset < 2 {
			return 0, errInvalidHoldToken
		}
		v := binary.BigEndian.Uint16(decoded[offset : offset+2])
		offset += 2
		return v, nil
	}

	keyCount, readErr := readUint16()
	if readErr != nil {
		return nil, "", readErr
	}

	resourceKeys = make([]string, 0, keyCount)
	for i := 0; i < int(keyCount); i++ {
		keyLen, readErr := readUint16()
		if readErr != nil {
			return nil, "", readErr
		}
		if len(decoded)-offset < int(keyLen) {
			return nil, "", errInvalidHoldToken
		}
		resourceKeys = append(resourceKeys, string(decoded[offset:offset+int(keyLen)]))
		offset += int(keyLen)
	}

	ownerLen, readErr := readUint16()
	if readErr != nil {
		return nil, "", readErr
	}
	if len(decoded)-offset < int(ownerLen) {
		return nil, "", errInvalidHoldToken
	}
	ownerID = string(decoded[offset : offset+int(ownerLen)])
	offset += int(ownerLen)
	if offset != len(decoded) {
		return nil, "", errInvalidHoldToken
	}

	return resourceKeys, ownerID, nil
}
