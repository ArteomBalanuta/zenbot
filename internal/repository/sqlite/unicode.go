package sqlite

import (
	"database/sql/driver"
	"fmt"
	"math"
	"strconv"
	"strings"

	sqliteDriver "modernc.org/sqlite"
)

func init() {
	// Register once before any connection opens, including test fixtures. SQLite's
	// ASCII-only LOWER otherwise disagrees with Go's nickname normalization.
	sqliteDriver.MustRegisterDeterministicScalarFunction("lower", 1, func(_ *sqliteDriver.FunctionContext, args []driver.Value) (driver.Value, error) {
		switch value := args[0].(type) {
		case nil:
			return nil, nil
		case string:
			return strings.ToLower(value), nil
		case []byte:
			return strings.ToLower(string(value)), nil
		case int64:
			return strconv.FormatInt(value, 10), nil
		case float64:
			// SQLite lower coerces numeric arguments to text, including the decimal
			// marker distinguishing integral real values from integer values.
			if math.IsInf(value, 1) {
				return "inf", nil
			}
			if math.IsInf(value, -1) {
				return "-inf", nil
			}
			if value == 0 {
				return "0.0", nil
			}
			text := strconv.FormatFloat(value, 'g', 15, 64)
			if exponent := strings.IndexByte(text, 'e'); exponent >= 0 && !strings.Contains(text[:exponent], ".") {
				text = text[:exponent] + ".0" + text[exponent:]
			} else if !strings.ContainsAny(text, ".eE") {
				text += ".0"
			}
			return strings.ToLower(text), nil
		default:
			return nil, fmt.Errorf("unsupported SQLite lower argument %T", value)
		}
	})
}
