package types

import "context"

// HostExecFunc adapts a function to a host-op executor (Exec method).
//
// This is the canonical definition shared across packages to avoid duplicate,
// subtly-incompatible function types.
type HostExecFunc func(ctx context.Context, req HostOpRequest) HostOpResponse

func (f HostExecFunc) Exec(ctx context.Context, req HostOpRequest) HostOpResponse {
	return f(ctx, req)
}
