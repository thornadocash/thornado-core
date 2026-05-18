package constants

// All strings used in Mimir keys should be recorded here and referred to from elsewhere,
// except for strings referring to arbitrary Assets/Chains.
// Each string should clearly indicate its usage for the final Mimir key (key, template, reference)
// and no Mimir key should require the combination of more than two strings.
const (
	MimirKeySecuredAssetHaltGlobal = "HaltSecuredGlobal"
	MimirKeyWasmPermissionless     = "WasmPermissionless"
	MimirKeyWasmHaltGlobal         = "HaltWasmGlobal"
	MimirKeyWasmMinGasPrice        = "WasmMinGasPrice"
	MimirKeyScheduledMigration     = "ScheduledMigration"

	MimirTemplateEVMAllowanceCheck         = "EVMAllowanceCheck-%s"         // Use with Chain
	MimirTemplateConfMultiplierBasisPoints = "ConfMultiplierBasisPoints-%s" // Use with Chain
	MimirTemplateMaxConfirmations          = "MaxConfirmations-%s"          // Use with Chain
	MimirTemplateSwapSlipBasisPointsMin    = "SwapSlipBasisPointsMin-%s"    // Use with MimirRef
	MimirTemplateSecuredAssetHaltDeposit   = "HaltSecuredDeposit-%s"        // Use with Chain
	MimirTemplateSecuredAssetHaltWithdraw  = "HaltSecuredWithdraw-%s"       // Use with Chain
	MimirTemplateWasmHaltChecksum          = "HaltWasmCs-%s"                // Encode the checksum to base32 to fit within mimir's 64 char limit and case insenstivity. Truncate trailing `=` for brevity
	MimirTemplateWasmHaltContract          = "HaltWasmContract-%s"          // Use contract address checksum (last 6) for brevity and to fit inside mimir's 64 char length
	MimirTemplateWasmHaltDeployer          = "HaltWasmDeployer-%s"          // Use full deployer address to prevent a deployer from instantiating new contracts
	MimirTemplateMaxGas                    = "MaxGas-%s"                    // Use with Chain (e.g., MaxGas-ETH)
	MimirTemplateSwitch                    = "EnableSwitch-%s-%s"           // Use with Chain, Symbol
	MimirTemplatePauseLPDeposit            = "PauseLPDeposit-%s"            // Use with Asset MimirString
	MimirTemplateHaltSigning               = "HaltSigning%s"                // Use with Chain (mixed case, e.g., HaltSigningETH)
	MimirTemplateHaltTrading               = "Halt%sTrading"                // Use with Chain (mixed case, e.g., HaltETHTrading)
	MimirKeyHaltTradingGlobal              = "HaltTrading"                  // Global trading halt

	MimirRefL1           = "L1"           // Use with SwapSlipBasisPoints
	MimirRefSynth        = "Synth"        // Use with SwapSlipBasisPoints
	MimirRefTradeAccount = "TradeAccount" // Use with SwapSlipBasisPoints

	MimirMaxBatchSize           = "AttestationMaxBatchSize"
	MimirPeerConcurrentSends    = "AttestationPeerConcurrentSends"
	MimirPeerConcurrentReceives = "AttestationPeerConcurrentReceives"
)
