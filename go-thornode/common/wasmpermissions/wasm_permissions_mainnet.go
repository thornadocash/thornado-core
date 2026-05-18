//go:build !mocknet
// +build !mocknet

package wasmpermissions

var WasmPermissionsRaw = WasmPermissions{
	Store: map[string]bool{
		// Rujira multisig 3/3
		"thor1e0lmk5juawc46jwjwd0xfz587njej7ay5fh6cd": true,
		// Rujira multisig 3/4
		"thor1hmfsqvr4cyh02z6le3wej0v4y7l605zlxjw022": true,
		// Rujira DAODAO
		"thor1pnad3hhgktqde00jl6wvyuuspatle000wl9pehgqxmehl7974e4szc4zpn": true,
	},
	Instantiate: map[string]bool{
		// Rujira multisig 3/3
		"thor1e0lmk5juawc46jwjwd0xfz587njej7ay5fh6cd": true,
		// Rujira multisig 3/4
		"thor1hmfsqvr4cyh02z6le3wej0v4y7l605zlxjw022": true,
		// Rujira DAODAO
		"thor1pnad3hhgktqde00jl6wvyuuspatle000wl9pehgqxmehl7974e4szc4zpn": true,
		// DAODAO
		"thor1gg2hk8nnap6u6axlkv0rjfghd2vjlwkyshhe8s": true,
		// Levana Ruji Perps
		"thor1440jp0ukj8ew3z2fd4zmdqgxhn5ghd7ghg2kmr": true,
		// Nami
		"thor1zjwanvezcjp6hefgt6vqfnrrdm8yj9za3s8ss0": true,
		// Auto
		"thor1lt2r7uwly4gwx7kdmdp86md3zzdrqlt3dgr0ag": true,
		// Calc DAODAO
		"thor17dxtxrne37gguxdeun4n36vqd5jmxxku5tr6gkuhhsh4lz9e8gksck4ygu": true,
		// Liquidy DAODAO
		"thor1j95vmsmkevynmenkkxhlu5at9exgtsuck6nhh79f0x4zx85r5ajqnpnhj2": true,
		// Fuzion DAODAO
		"thor1e69r4z9fgx5ghz2l4dfqv0zqw2yhsqf946tgfvpyzsnah2lg7aesfel69y": true,
		// Redacted DAODAO
		"thor15qymde6pkjxl2c068lk2gq0c7rcps4mckd3ngzwgy5n2mx6ms6mq3xntrt": true,
	},
}
