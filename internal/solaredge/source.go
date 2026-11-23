package solaredge

import "context"

// Source fetches a solar snapshot (cloud API or Modbus).
type Source interface {
	Enabled() bool
	Fetch(ctx context.Context) (Snapshot, error)
}
