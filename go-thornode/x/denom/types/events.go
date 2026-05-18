package types

// event types
//
//nolint:gosec
const (
	EventCreateDenom      = "create_denom"
	EventMintTokens       = "mint"
	EventBurnTokens       = "burn"
	EventChangeDenomAdmin = "change_admin"

	AttributeAmount          = "amount"
	AttributeCreator         = "creator"
	AttributeNewTokenDenom   = "new_token_denom"
	AttributeMintToAddress   = "mint_to_address"
	AttributeBurnFromAddress = "burn_from_address"
	AttributeDenom           = "denom"
	AttributeNewAdmin        = "new_admin"
)
