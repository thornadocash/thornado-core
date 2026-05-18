//go:build mocknet
// +build mocknet

package wasmpermissions

var WasmPermissionsRaw = WasmPermissions{
	Store: map[string]bool{
		"tthor1jgnk2mg88m57csrmrlrd6c3qe4lag3e33y2f3k": true,
		"tthor1tdfqy34uptx207scymqsy4k5uzfmry5sf7z3dw": false,
		"tthor1khtl8ch2zgay00c47ukvulam3a4faw2500g7lu": true,
	},
	Instantiate: map[string]bool{
		"tthor1jgnk2mg88m57csrmrlrd6c3qe4lag3e33y2f3k": true,
		"tthor1tdfqy34uptx207scymqsy4k5uzfmry5sf7z3dw": false,
		"tthor1khtl8ch2zgay00c47ukvulam3a4faw2500g7lu": true,
	},
}
