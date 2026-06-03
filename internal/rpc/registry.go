package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type Handler interface {
	Handle(ctx context.Context, params json.RawMessage) (any, error)
}

type HandlerFunc func(ctx context.Context, params json.RawMessage) (any, error)

func (f HandlerFunc) Handle(ctx context.Context, params json.RawMessage) (any, error) {
	return f(ctx, params)
}

type Registry struct {
	handlers map[string]Handler
}

func NewRegistry() *Registry {
	return &Registry{handlers: map[string]Handler{}}
}

func (r *Registry) Add(method string, handler Handler) error {
	method = strings.TrimSpace(method)
	if method == "" {
		return fmt.Errorf("rpc method is required")
	}
	if handler == nil {
		return fmt.Errorf("rpc handler for method %q is nil", method)
	}
	if r.handlers == nil {
		r.handlers = map[string]Handler{}
	}
	if _, exists := r.handlers[method]; exists {
		return fmt.Errorf("duplicate rpc handler registration for method %q", method)
	}
	r.handlers[method] = handler
	return nil
}

func (r *Registry) Handler(method string) (Handler, bool) {
	if r == nil {
		return nil, false
	}
	handler, ok := r.handlers[strings.TrimSpace(method)]
	return handler, ok
}

func BindHandler[Params any, Result any](allowEmptyParams bool, fn func(context.Context, Params) (Result, error)) Handler {
	return HandlerFunc(func(ctx context.Context, params json.RawMessage) (any, error) {
		var p Params
		if len(params) == 0 {
			if allowEmptyParams {
				return fn(ctx, p)
			}
			params = json.RawMessage(`{}`)
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, InvalidParams("invalid params")
		}
		return fn(ctx, p)
	})
}

func AddBoundHandler[Params any, Result any](reg *Registry, method string, allowEmptyParams bool, fn func(context.Context, Params) (Result, error)) error {
	if reg == nil {
		return fmt.Errorf("rpc registry is required")
	}
	return reg.Add(method, BindHandler(allowEmptyParams, fn))
}

func RegisterHandlers(fns ...func() error) error {
	for _, fn := range fns {
		if fn == nil {
			continue
		}
		if err := fn(); err != nil {
			return err
		}
	}
	return nil
}
