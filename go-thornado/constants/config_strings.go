package constants

// All strings used in Config keys should be recorded here and referred to from elsewhere,
// except for strings referring to arbitrary Assets/Chains.
// Each string should clearly indicate its usage for the final Config key (key, template, reference)
// and no Config key should require the combination of more than two strings.
const (
	ConfigKeyEnableFrostBTC       = "EnableFrostBTC"
	ConfigKeyNodePauseChainGlobal = "NodePauseChainGlobal"

	ConfigTemplateHaltSigning = "HaltSigning%s" // Use with Chain (mixed case, e.g., HaltSigningBTC)
)
