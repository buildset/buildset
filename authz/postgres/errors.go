package postgres

import "errors"

// sqlStater is implemented by the driver's error type. Matching the method rather than the concrete
// type keeps the driver out of this package's imports.
type sqlStater interface {
	error
	SQLState() string
}

func sqlState(err error) string {
	if stater, ok := errors.AsType[sqlStater](err); ok {
		return stater.SQLState()
	}

	return ""
}

// https://www.postgresql.org/docs/current/errcodes-appendix.html
const foreignKeyViolation = "23503"

func isForeignKeyViolation(err error) bool {
	return sqlState(err) == foreignKeyViolation
}
