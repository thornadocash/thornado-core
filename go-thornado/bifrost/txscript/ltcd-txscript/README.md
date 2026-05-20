# txscript

[![ISC License](http://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)

This package is forked from https://github.com/ltcsuite/ltcd/

Package txscript implements the litecoin transaction script language. There is
a comprehensive test suite.

This package has intentionally been designed so it can be used as a standalone
package for any projects needing to use or validate litecoin transaction scripts.

## Litecoin Scripts

Litecoin provides a stack-based, FORTH-like language for the scripts in
the litecoin transactions. This language is not turing complete
although it is still fairly powerful. A description of the language
can be found at https://en.litecoin.it/wiki/Script

## Installation and Updating

```bash
$ go get -u gitlab.com/thorchain/bifrost/ltcd-txscript
```

## Examples

- [Standard Pay-to-pubkey-hash Script](https://pkg.go.dev/gitlab.com/thorchain/bifrost/ltcd-txscript#example-PayToAddrScript)
  Demonstrates creating a script which pays to a litecoin address. It also
  prints the created script hex and uses the DisasmString function to display
  the disassembled script.

- [Extracting Details from Standard Scripts](https://pkg.go.dev/gitlab.com/thorchain/bifrost/ltcd-txscript#example-ExtractPkScriptAddrs)
  Demonstrates extracting information from a standard public key script.

- [Manually Signing a Transaction Output](https://pkg.go.dev/gitlab.com/thorchain/bifrost/ltcd-txscript#example-SignTxOutput)
  Demonstrates manually creating and signing a redeem transaction.

## License

Package txscript is licensed under the [copyfree](http://copyfree.org) ISC
License.
