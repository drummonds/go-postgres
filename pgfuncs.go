package pglike

import (
	"crypto/md5"
	"database/sql/driver"
	"encoding/hex"
	"fmt"
	"math/rand"
	"regexp"
	"strings"

	"modernc.org/sqlite"
)

// registerPGFunctions registers PostgreSQL-compatible functions in the SQLite engine.
// These are registered globally and available on all connections.
func registerPGFunctions() {
	// gen_random_uuid() -> UUID v4 string
	sqlite.MustRegisterScalarFunction("gen_random_uuid", 0,
		func(ctx *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
			return generateUUIDv4(), nil
		},
	)

	// md5(string) -> hex MD5 hash
	sqlite.MustRegisterDeterministicScalarFunction("md5", 1,
		func(ctx *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
			var data []byte
			switch v := args[0].(type) {
			case string:
				data = []byte(v)
			case []byte:
				data = v
			case nil:
				return nil, nil
			default:
				data = []byte(fmt.Sprint(v))
			}
			h := md5.Sum(data)
			return hex.EncodeToString(h[:]), nil
		},
	)

	// split_part(string, delimiter, field) -> nth field (1-indexed)
	sqlite.MustRegisterDeterministicScalarFunction("split_part", 3,
		func(ctx *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
			if args[0] == nil || args[1] == nil || args[2] == nil {
				return nil, nil
			}
			str, ok1 := args[0].(string)
			delim, ok2 := args[1].(string)
			field, ok3 := args[2].(int64)
			if !ok1 || !ok2 || !ok3 {
				return "", nil
			}
			parts := strings.Split(str, delim)
			idx := int(field) - 1 // PG is 1-indexed
			if idx < 0 || idx >= len(parts) {
				return "", nil
			}
			return parts[idx], nil
		},
	)

	// pg_regex_match(str, pattern, case_insensitive) -> 1 if matches, 0 otherwise
	sqlite.MustRegisterDeterministicScalarFunction("pg_regex_match", 3,
		func(ctx *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
			if args[0] == nil || args[1] == nil {
				return int64(0), nil
			}
			str, ok1 := args[0].(string)
			pattern, ok2 := args[1].(string)
			if !ok1 || !ok2 {
				return int64(0), nil
			}
			caseInsensitive, _ := args[2].(int64)
			if caseInsensitive == 1 {
				pattern = "(?i)" + pattern
			}
			matched, err := regexp.MatchString(pattern, str)
			if err != nil {
				return int64(0), nil
			}
			if matched {
				return int64(1), nil
			}
			return int64(0), nil
		},
	)

	// pg_typeof(expr) -> type name as string
	sqlite.MustRegisterScalarFunction("pg_typeof", 1,
		func(ctx *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
			switch args[0].(type) {
			case nil:
				return "unknown", nil
			case int64:
				return "integer", nil
			case float64:
				return "double precision", nil
			case string:
				return "text", nil
			case []byte:
				return "bytea", nil
			default:
				return "unknown", nil
			}
		},
	)
}

// generateUUIDv4 generates a random UUID v4 string.
func generateUUIDv4() string {
	var uuid [16]byte
	_, _ = rand.Read(uuid[:])
	uuid[6] = (uuid[6] & 0x0f) | 0x40 // version 4
	uuid[8] = (uuid[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16])
}
