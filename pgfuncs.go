package pglike

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/ncruces/go-sqlite3"
)

// registerPGFunctions registers PostgreSQL-compatible functions on a SQLite connection.
// Must be called for each new connection.
func registerPGFunctions(conn *sqlite3.Conn) error {
	// gen_random_uuid() -> UUID v4 string
	err := conn.CreateFunction("gen_random_uuid", 0, 0,
		func(ctx sqlite3.Context, arg ...sqlite3.Value) {
			ctx.ResultText(generateUUIDv4())
		},
	)
	if err != nil {
		return err
	}

	// md5(string) -> hex MD5 hash
	err = conn.CreateFunction("md5", 1, sqlite3.DETERMINISTIC,
		func(ctx sqlite3.Context, arg ...sqlite3.Value) {
			if arg[0].Type() == sqlite3.NULL {
				ctx.ResultNull()
				return
			}
			var data []byte
			switch arg[0].Type() {
			case sqlite3.TEXT:
				data = []byte(arg[0].Text())
			case sqlite3.BLOB:
				data = arg[0].RawBlob()
			default:
				data = []byte(fmt.Sprint(arg[0].Text()))
			}
			h := md5.Sum(data)
			ctx.ResultText(hex.EncodeToString(h[:]))
		},
	)
	if err != nil {
		return err
	}

	// split_part(string, delimiter, field) -> nth field (1-indexed)
	err = conn.CreateFunction("split_part", 3, sqlite3.DETERMINISTIC,
		func(ctx sqlite3.Context, arg ...sqlite3.Value) {
			if arg[0].Type() == sqlite3.NULL || arg[1].Type() == sqlite3.NULL || arg[2].Type() == sqlite3.NULL {
				ctx.ResultNull()
				return
			}
			str := arg[0].Text()
			delim := arg[1].Text()
			field := arg[2].Int64()
			parts := strings.Split(str, delim)
			idx := int(field) - 1 // PG is 1-indexed
			if idx < 0 || idx >= len(parts) {
				ctx.ResultText("")
				return
			}
			ctx.ResultText(parts[idx])
		},
	)
	if err != nil {
		return err
	}

	// pg_regex_match(str, pattern, case_insensitive) -> 1 if matches, 0 otherwise
	err = conn.CreateFunction("pg_regex_match", 3, sqlite3.DETERMINISTIC,
		func(ctx sqlite3.Context, arg ...sqlite3.Value) {
			if arg[0].Type() == sqlite3.NULL || arg[1].Type() == sqlite3.NULL {
				ctx.ResultInt64(0)
				return
			}
			str := arg[0].Text()
			pattern := arg[1].Text()
			caseInsensitive := arg[2].Int64()
			if caseInsensitive == 1 {
				pattern = "(?i)" + pattern
			}
			matched, err := regexp.MatchString(pattern, str)
			if err != nil {
				ctx.ResultInt64(0)
				return
			}
			if matched {
				ctx.ResultInt64(1)
			} else {
				ctx.ResultInt64(0)
			}
		},
	)
	if err != nil {
		return err
	}

	// pg_to_char(datetime_text, pg_format) -> formatted string
	err = conn.CreateFunction("pg_to_char", 2, sqlite3.DETERMINISTIC,
		func(ctx sqlite3.Context, arg ...sqlite3.Value) {
			if arg[0].Type() == sqlite3.NULL || arg[1].Type() == sqlite3.NULL {
				ctx.ResultNull()
				return
			}
			dtStr := arg[0].Text()
			pgFmt := arg[1].Text()
			t, err := parseDateTime(dtStr)
			if err != nil {
				ctx.ResultText(dtStr)
				return
			}
			ctx.ResultText(formatPGStyle(t, pgFmt))
		},
	)
	if err != nil {
		return err
	}

	// pg_similar_match(str, pattern) -> 1 if matches SQL SIMILAR TO pattern, 0 otherwise
	err = conn.CreateFunction("pg_similar_match", 2, sqlite3.DETERMINISTIC,
		func(ctx sqlite3.Context, arg ...sqlite3.Value) {
			if arg[0].Type() == sqlite3.NULL || arg[1].Type() == sqlite3.NULL {
				ctx.ResultInt64(0)
				return
			}
			str := arg[0].Text()
			pattern := arg[1].Text()
			re := convertSimilarToRegex(pattern)
			matched, err := regexp.MatchString(re, str)
			if err != nil {
				ctx.ResultInt64(0)
				return
			}
			if matched {
				ctx.ResultInt64(1)
			} else {
				ctx.ResultInt64(0)
			}
		},
	)
	if err != nil {
		return err
	}

	// pg_typeof(expr) -> type name as string
	err = conn.CreateFunction("pg_typeof", 1, 0,
		func(ctx sqlite3.Context, arg ...sqlite3.Value) {
			switch arg[0].Type() {
			case sqlite3.NULL:
				ctx.ResultText("unknown")
			case sqlite3.INTEGER:
				ctx.ResultText("integer")
			case sqlite3.FLOAT:
				ctx.ResultText("double precision")
			case sqlite3.TEXT:
				ctx.ResultText("text")
			case sqlite3.BLOB:
				ctx.ResultText("bytea")
			default:
				ctx.ResultText("unknown")
			}
		},
	)
	if err != nil {
		return err
	}

	return nil
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
