package app

import (
	"github.com/cosmos/cosmos-sdk/baseapp"
	gogogrpc "github.com/cosmos/gogoproto/grpc"
	"google.golang.org/grpc"
)

var _ gogogrpc.Server = &MsgServiceRouter{}

// MsgServiceRouter extends baseapp's MsgServiceRouter to support custom routing
type MsgServiceRouter struct {
	*baseapp.MsgServiceRouter
	customRoutes map[string]interface{}
}

func NewMsgServiceRouter(bAppMsr *baseapp.MsgServiceRouter) *MsgServiceRouter {
	return &MsgServiceRouter{
		MsgServiceRouter: bAppMsr, // Use the provided baseapp router instead of creating new
		customRoutes:     make(map[string]interface{}),
	}
}

func (msr *MsgServiceRouter) RegisterService(sd *grpc.ServiceDesc, handler interface{}) {
	// Check if we have a custom handler for this service
	if customHandler := msr.customRoutes[sd.ServiceName]; customHandler != nil {
		handler = customHandler
	}

	// Only register with the base router - we're using the one from baseapp
	msr.MsgServiceRouter.RegisterService(sd, handler)
}

func (msr *MsgServiceRouter) AddCustomRoute(serviceName string, handler interface{}) {
	msr.customRoutes[serviceName] = handler
}
