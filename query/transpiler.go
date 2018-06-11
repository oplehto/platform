package query

import (
	"context"

	"github.com/influxdata/platform"
)

// Transpiler can convert a query from a source lanague into a query spec.
type Transpiler interface {
	// Transpile will perform the transpilation. The config is an implementation-specific
	// interface that can be retrieved by using DefaultConfig and modifying the returned
	// value using reflection. If a config of nil is passed in, the Transpiler will use
	// the default configuration.
	Transpile(ctx context.Context, txt string, config interface{}) (*Spec, error)

	// DefaultConfig returns the default configuration for this transpiler.
	// It can then be customized using reflection.
	DefaultConfig() interface{}
}

// QueryWithTranspile executes a query by first transpiling the query.
func QueryWithTranspile(ctx context.Context, orgID platform.ID, q string, qs QueryService, transpiler Transpiler) (ResultIterator, error) {
	spec, err := transpiler.Transpile(ctx, q, nil)
	if err != nil {
		return nil, err
	}

	return qs.Query(ctx, orgID, spec)
}
