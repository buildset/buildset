package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/Masterminds/squirrel"
)

// squirrel has no vocabulary for UNION, so the halves are built separately and joined here, which
// keeps the arguments ordered and the placeholders numbered correctly.
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

	// Renumbered once, here, so each half stays unaware of how many arguments the other contributed.
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
