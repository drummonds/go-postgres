package pglike

import (
	"crypto/md5"
	"database/sql/driver"
	"encoding/hex"
	"fmt"
	"math/rand"
	"regexp"
	"strings"
	"time"

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

	// pg_to_char(datetime_text, pg_format) -> formatted string
	sqlite.MustRegisterDeterministicScalarFunction("pg_to_char", 2,
		func(ctx *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
			if args[0] == nil || args[1] == nil {
				return nil, nil
			}
			dtStr, ok1 := args[0].(string)
			pgFmt, ok2 := args[1].(string)
			if !ok1 || !ok2 {
				return nil, nil
			}
			t, err := parseDateTime(dtStr)
			if err != nil {
				return dtStr, nil
			}
			return formatPGStyle(t, pgFmt), nil
		},
	)

	// pg_similar_match(str, pattern) -> 1 if matches SQL SIMILAR TO pattern, 0 otherwise
	// SIMILAR TO patterns use: % (any string), _ (any char), | (alternation), () (grouping)
	sqlite.MustRegisterDeterministicScalarFunction("pg_similar_match", 2,
		func(ctx *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
			if args[0] == nil || args[1] == nil {
				return int64(0), nil
			}
			str, ok1 := args[0].(string)
			pattern, ok2 := args[1].(string)
			if !ok1 || !ok2 {
				return int64(0), nil
			}
			re := convertSimilarToRegex(pattern)
			matched, err := regexp.MatchString(re, str)
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

// parseDateTime parses a datetime string in common SQLite/ISO formats.
func parseDateTime(s string) (time.Time, error) {
	formats := []string{
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05.000",
		"2006-01-02T15:04:05Z",
		"2006-01-02",
		"15:04:05",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot parse %q", s)
}

// formatPGStyle formats a time using PostgreSQL format patterns.
func formatPGStyle(t time.Time, pgFmt string) string {
	months := []string{"", "January", "February", "March", "April", "May", "June",
		"July", "August", "September", "October", "November", "December"}
	monthsShort := []string{"", "Jan", "Feb", "Mar", "Apr", "May", "Jun",
		"Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}
	days := []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}
	daysShort := []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}

	r := strings.NewReplacer(
		"YYYY", fmt.Sprintf("%04d", t.Year()),
		"YY", fmt.Sprintf("%02d", t.Year()%100),
		"Month", months[t.Month()],
		"MONTH", strings.ToUpper(months[t.Month()]),
		"month", strings.ToLower(months[t.Month()]),
		"Mon", monthsShort[t.Month()],
		"MON", strings.ToUpper(monthsShort[t.Month()]),
		"mon", strings.ToLower(monthsShort[t.Month()]),
		"MM", fmt.Sprintf("%02d", t.Month()),
		"Day", days[t.Weekday()],
		"DAY", strings.ToUpper(days[t.Weekday()]),
		"day", strings.ToLower(days[t.Weekday()]),
		"Dy", daysShort[t.Weekday()],
		"DY", strings.ToUpper(daysShort[t.Weekday()]),
		"dy", strings.ToLower(daysShort[t.Weekday()]),
		"DD", fmt.Sprintf("%02d", t.Day()),
		"HH24", fmt.Sprintf("%02d", t.Hour()),
		"HH12", fmt.Sprintf("%02d", (t.Hour()+11)%12+1),
		"HH", fmt.Sprintf("%02d", t.Hour()),
		"MI", fmt.Sprintf("%02d", t.Minute()),
		"SS", fmt.Sprintf("%02d", t.Second()),
		"AM", map[bool]string{true: "AM", false: "PM"}[t.Hour() < 12],
		"PM", map[bool]string{true: "AM", false: "PM"}[t.Hour() < 12],
		"Q", fmt.Sprintf("%d", (int(t.Month())-1)/3+1),
	)
	return r.Replace(pgFmt)
}

// convertSimilarToRegex converts a SQL SIMILAR TO pattern to a Go regex.
// SIMILAR TO uses: % (any string), _ (any char), | (alternation), () (grouping).
func convertSimilarToRegex(pattern string) string {
	var b strings.Builder
	b.WriteString("^")
	for _, ch := range pattern {
		switch ch {
		case '%':
			b.WriteString(".*")
		case '_':
			b.WriteString(".")
		case '|', '(', ')':
			b.WriteRune(ch)
		case '.', '^', '$', '+', '?', '{', '}', '[', ']', '\\', '*':
			b.WriteRune('\\')
			b.WriteRune(ch)
		default:
			b.WriteRune(ch)
		}
	}
	b.WriteString("$")
	return b.String()
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
