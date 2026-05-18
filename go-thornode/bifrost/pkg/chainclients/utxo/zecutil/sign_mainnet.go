//go:build !mocknet
// +build !mocknet

package zecutil

// https://github.com/zcash/zcash/blob/89f5ee5dec3fdfd70202baeaf74f09fa32bfb1a8/src/chainparams.cpp#L99
// https://github.com/zcash/zcash/blob/master/src/consensus/upgrades.cpp#L11
// activation levels are used for testnet because mainnet is already updated
// TODO: need implement own complete chain params and use them
var upgradeParams = []upgradeParam{
	{0, []byte{0x00, 0x00, 0x00, 0x00}},
	{207500, []byte{0x19, 0x1B, 0xA8, 0x5B}},  // Overwinter  0x5ba81b19
	{280000, []byte{0xBB, 0x09, 0xB8, 0x76}},  // Sapling     0x76b809bb
	{653600, []byte{0x60, 0x0E, 0xB4, 0x2B}},  // Blossom     0x2bb40e60
	{903000, []byte{0x0B, 0x23, 0xB9, 0xF5}},  // Heartwood   0xf5b9230b
	{1046400, []byte{0xA6, 0x75, 0xFF, 0xE9}}, // Canopy      0xe9ff75a6
	{1687104, []byte{0xB4, 0xD0, 0xD6, 0xC2}}, // NU5         0xc2d6d0b4
	{2726400, []byte{0x55, 0x10, 0xE7, 0xC8}}, // NU6         0xc8e71055
	{3146400, []byte{0xF0, 0x4D, 0xEC, 0x4D}}, // NU6.1       0x4dec4df0
}
