package thornado

import (
	"fmt"

	"github.com/thornadocash/go-thornado/common/cosmos"
)

type TypedHandler[M cosmos.Msg] interface {
	MsgHandler

	validate(ctx cosmos.Context, msg M) error
	handle(ctx cosmos.Context, msg M) (*cosmos.Result, error)
}

type BaseHandler[M cosmos.Msg] struct {
	TypedHandler[M]

	mgr      Manager
	logger   func(cosmos.Context, M)
	nodes    LadderDispatch[func(cosmos.Context, Manager, M) error]
	handlers LadderDispatch[func(cosmos.Context, Manager, M) (*cosmos.Result, error)]
}

// Sadly Go's type system is still primitive and does not allow for generics in
// type aliases, so the following cleaner convenience type does not work:
//
//	type Nodes[M MsgType] = LadderDispatch[func(cosmos.Context, Manager, M) error]
//
// See https://github.com/golang/go/issues/46477 for ongoing discussion.
//
// Instead we use these thin convenience functions to do the same type wrapping:
func NewNodes[M cosmos.Msg]() LadderDispatch[func(cosmos.Context, Manager, M) error] {
	return LadderDispatch[func(cosmos.Context, Manager, M) error]{}
}

func NewHandlers[M cosmos.Msg]() LadderDispatch[func(cosmos.Context, Manager, M) (*cosmos.Result, error)] {
	return LadderDispatch[func(cosmos.Context, Manager, M) (*cosmos.Result, error)]{}
}

func typeof(v interface{}) string {
	return fmt.Sprintf("%T", v)
}

func (h BaseHandler[M]) Run(ctx cosmos.Context, m cosmos.Msg) (*cosmos.Result, error) {
	msg, ok := m.(M)
	if !ok {
		return nil, errInvalidMessage
	}
	if err := h.validate(ctx, msg); err != nil {
		ctx.Logger().Error(typeof(m)+" failed validation", "error", err)
		return nil, err
	}
	if h.logger != nil {
		h.logger(ctx, msg)
	}
	result, err := h.handle(ctx, msg)
	if err != nil {
		ctx.Logger().Error(typeof(m)+" failed handler", "error", err)
	}
	return result, err
}

func (h BaseHandler[M]) validate(ctx cosmos.Context, msg M) error {
	version := h.mgr.GetVersion()
	node := h.nodes.Get(version)
	if node == nil {
		return errBadVersion
	}
	return node(ctx, h.mgr, msg)
}

func (h BaseHandler[M]) handle(ctx cosmos.Context, msg M) (*cosmos.Result, error) {
	version := h.mgr.GetVersion()
	handler := h.handlers.Get(version)
	if handler != nil {
		return handler(ctx, h.mgr, msg)
	}
	// handlerNoCosmos := h.handlersNC.Get(version)
	// if handlerNoCosmos != nil {
	// 	err := handlerNoCosmos(ctx, h.mgr, msg)
	// 	if err != nil {
	// 		return nil, err
	// 	} else {
	// 		return &cosmos.Result{}, nil
	// 	}
	// }
	return nil, errBadVersion
}
