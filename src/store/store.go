package store

import (
	"context"

	"github.com/anandpsdev/apiwatch/src/types"
)

type Store interface {
	Append(ctx context.Context, entry types.Entry) error
	Get(ctx context.Context, id string) (types.Entry, error)
	List(ctx context.Context, filter types.Filter) ([]types.Entry, error)
	Clear(ctx context.Context) error
	Close() error
}
