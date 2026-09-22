package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/Masterminds/squirrel"
)

// union runs two selects as one UNION.
//
// squirrel has no vocabulary for UNION, so the two halves are rendered and joined here. Both are
// still built rather than written, which is what keeps the arguments ordered and the placeholders
// numbered correctly.
func union(ctx context.Context, db squirrel.QueryerContext, format squirrel.PlaceholderFormat, parts ...squirrel.SelectBuilder) (*sql.Rows, error) {
	var (
		clauses   []string
		arguments []any
	)

	for _, part := range parts {
		query, args, err := part.ToSql()
		if err != nil {
			return nil, fmt.Errorf("build union part: %w", err)
		}

		clauses = append(clauses, query)
		arguments = append(arguments, args...)
	}

	// The parts are built with ? placeholders and renumbered once, here, so each half stays
	// unaware of how many arguments the other contributed.
	query, err := format.ReplacePlaceholders(strings.Join(clauses, " UNION "))
	if err != nil {
		return nil, fmt.Errorf("replace placeholders: %w", err)
	}

	rows, err := db.QueryContext(ctx, query, arguments...)
	if err != nil {
		return nil, fmt.Errorf("query union: %w", err)
	}

	return rows, nil
}
