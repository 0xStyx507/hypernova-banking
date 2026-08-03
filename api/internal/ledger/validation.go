package ledger

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	tigerbeetle "github.com/tigerbeetle/tigerbeetle-go"
)

func normalizeCurrency(value string) (string, error) {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "" {
		value = defaultCurrency
	}
	if value != defaultCurrency {
		return "", ErrInvalidInput
	}
	return value, nil
}

// parseMinorAmount accepts only positive integer minor units. This keeps
// decimal parsing and floating-point rounding out of the financial boundary.
func parseMinorAmount(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" || strings.HasPrefix(value, "+") || strings.HasPrefix(value, "-") {
		return 0, ErrInvalidInput
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return 0, ErrInvalidInput
		}
	}
	parsed, err := strconv.ParseUint(value, 10, 63)
	if err != nil || parsed == 0 {
		return 0, ErrInvalidInput
	}
	return int64(parsed), nil
}

func hashRequest(operationType string, debit, credit tigerbeetle.Uint128, amount int64, currency string) []byte {
	return hashRequestWithScope(operationType, debit, credit, amount, currency, "")
}

func hashRequestWithScope(operationType string, debit, credit tigerbeetle.Uint128, amount int64, currency, scope string) []byte {
	value := fmt.Sprintf("%s|%s|%s|%d|%s|%s", operationType, debit.String(), credit.String(), amount, currency, scope)
	hash := sha256.Sum256([]byte(value))
	return hash[:]
}

func hashAccountCreation(currency string) []byte {
	hash := sha256.Sum256([]byte("account|" + currency))
	return hash[:]
}

func uint128String(value tigerbeetle.Uint128) string {
	parsed := value.BigInt()
	return parsed.String()
}

func uuidToTigerID(id uuid.UUID) tigerbeetle.Uint128 {
	var bytesValue [16]byte
	copy(bytesValue[:], id[:])
	return tigerbeetle.BytesToUint128(bytesValue)
}

func systemAccountID(currency string) tigerbeetle.Uint128 {
	// A fixed namespace keeps development system accounts stable without
	// overlapping normal UUID-v4 account identifiers in practice.
	if currency == defaultCurrency {
		return tigerbeetle.ToUint128(0x1000000000000001)
	}
	return tigerbeetle.ToUint128(0x1000000000000002)
}

func mustTigerID(value string) tigerbeetle.Uint128 {
	parsed, err := tigerbeetle.HexStringToUint128(value)
	if err != nil {
		panic("invalid persisted TigerBeetle identifier: " + hex.EncodeToString([]byte(value)))
	}
	return parsed
}
