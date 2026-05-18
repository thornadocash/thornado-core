package ethereum

import _ "embed"

//go:embed abi/router.json
var routerContractABI string

//go:embed abi/erc20.json
var erc20ContractABI string
