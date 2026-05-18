package gaia

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	atypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	grpc "google.golang.org/grpc"
)

// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://godoc.org/google.golang.org/grpc#ClientConn.NewStream.
type MockAccountServiceClient interface {
	// Accounts returns all the existing accounts
	//
	// Since: cosmos-sdk 0.43
	Accounts(ctx context.Context, in *atypes.QueryAccountsRequest, opts ...grpc.CallOption) (*atypes.QueryAccountsResponse, error)

	// Account returns account details based on address.
	Account(ctx context.Context, in *atypes.QueryAccountRequest, opts ...grpc.CallOption) (*atypes.QueryAccountResponse, error)

	// AccountAddressByID returns account address based on account number.
	//
	// Since: cosmos-sdk 0.46.2
	AccountAddressByID(ctx context.Context, in *atypes.QueryAccountAddressByIDRequest, opts ...grpc.CallOption) (*atypes.QueryAccountAddressByIDResponse, error)

	// Params queries all parameters.
	Params(ctx context.Context, in *atypes.QueryParamsRequest, opts ...grpc.CallOption) (*atypes.QueryParamsResponse, error)

	// ModuleAccounts returns all the existing module accounts.
	//
	// Since: cosmos-sdk 0.46
	ModuleAccounts(ctx context.Context, in *atypes.QueryModuleAccountsRequest, opts ...grpc.CallOption) (*atypes.QueryModuleAccountsResponse, error)

	ModuleAccountByName(ctx context.Context, in *atypes.QueryModuleAccountByNameRequest, opts ...grpc.CallOption) (*atypes.QueryModuleAccountByNameResponse, error)

	// Bech32Prefix queries bech32Prefix
	//
	// Since: cosmos-sdk 0.46
	Bech32Prefix(ctx context.Context, in *atypes.Bech32PrefixRequest, opts ...grpc.CallOption) (*atypes.Bech32PrefixResponse, error)

	// AddressBytesToString converts Account Address bytes to string
	//
	// Since: cosmos-sdk 0.46
	AddressBytesToString(ctx context.Context, in *atypes.AddressBytesToStringRequest, opts ...grpc.CallOption) (*atypes.AddressBytesToStringResponse, error)

	// AddressStringToBytes converts Address string to bytes
	//
	// Since: cosmos-sdk 0.46
	AddressStringToBytes(ctx context.Context, in *atypes.AddressStringToBytesRequest, opts ...grpc.CallOption) (*atypes.AddressStringToBytesResponse, error)

	// AccountInfo queries account info which is common to all account types.
	//
	// Since: cosmos-sdk 0.47
	AccountInfo(ctx context.Context, in *atypes.QueryAccountInfoRequest, opts ...grpc.CallOption) (*atypes.QueryAccountInfoResponse, error)
}

type mockAccountServiceClient struct{}

func NewMockAccountServiceClient() MockAccountServiceClient {
	return &mockAccountServiceClient{}
}

//go:embed test-data/account_by_address.json
var accountByAddress []byte

func (c *mockAccountServiceClient) Account(ctx context.Context, in *atypes.QueryAccountRequest, opts ...grpc.CallOption) (*atypes.QueryAccountResponse, error) {
	out := new(atypes.QueryAccountResponse)
	registry := codectypes.NewInterfaceRegistry()
	atypes.RegisterInterfaces(registry)
	cryptocodec.RegisterInterfaces(registry)
	cdc := codec.NewProtoCodec(registry)
	err := cdc.UnmarshalJSON(accountByAddress, out)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal block by height: %s", err)
	}
	return out, nil
}

func (c *mockAccountServiceClient) Accounts(ctx context.Context, in *atypes.QueryAccountsRequest, opts ...grpc.CallOption) (*atypes.QueryAccountsResponse, error) {
	return nil, nil
}

func (c *mockAccountServiceClient) AccountAddressByID(ctx context.Context, in *atypes.QueryAccountAddressByIDRequest, opts ...grpc.CallOption) (*atypes.QueryAccountAddressByIDResponse, error) {
	return nil, nil
}

func (c *mockAccountServiceClient) Params(ctx context.Context, in *atypes.QueryParamsRequest, opts ...grpc.CallOption) (*atypes.QueryParamsResponse, error) {
	return nil, nil
}

func (c *mockAccountServiceClient) ModuleAccounts(ctx context.Context, in *atypes.QueryModuleAccountsRequest, opts ...grpc.CallOption) (*atypes.QueryModuleAccountsResponse, error) {
	return nil, nil
}

func (c *mockAccountServiceClient) ModuleAccountByName(ctx context.Context, in *atypes.QueryModuleAccountByNameRequest, opts ...grpc.CallOption) (*atypes.QueryModuleAccountByNameResponse, error) {
	return nil, nil
}

func (c *mockAccountServiceClient) Bech32Prefix(ctx context.Context, in *atypes.Bech32PrefixRequest, opts ...grpc.CallOption) (*atypes.Bech32PrefixResponse, error) {
	return nil, nil
}

func (c *mockAccountServiceClient) AddressBytesToString(ctx context.Context, in *atypes.AddressBytesToStringRequest, opts ...grpc.CallOption) (*atypes.AddressBytesToStringResponse, error) {
	return nil, nil
}

func (c *mockAccountServiceClient) AddressStringToBytes(ctx context.Context, in *atypes.AddressStringToBytesRequest, opts ...grpc.CallOption) (*atypes.AddressStringToBytesResponse, error) {
	return nil, nil
}

func (c *mockAccountServiceClient) AccountInfo(ctx context.Context, in *atypes.QueryAccountInfoRequest, opts ...grpc.CallOption) (*atypes.QueryAccountInfoResponse, error) {
	return nil, nil
}
