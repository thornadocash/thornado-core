# txscript

This package is forked from https://github.com/gcash/bchd/

Package txscript implements the bitcoin transaction script language. There is
a comprehensive test suite.

This package has intentionally been designed so it can be used as a standalone
package for any projects needing to use or validate bitcoin transaction scripts.

## Bitcoin Scripts

Bitcoin provides a stack-based, FORTH-like language for the scripts in
the bitcoin transactions. This language is not turing complete
although it is still fairly powerful. A description of the language
can be found at https://en.bitcoin.it/wiki/Script

## Installation and Updating

```bash
$ go get -u gitlab.com/thorchain/bifrost/bchd-txscript
```

## Examples

- [Standard Pay-to-pubkey-hash Script](http://godoc.org/gitlab.com/thorchain/bifrost/bchd-txscript#example-PayToAddrScript)
  Demonstrates creating a script which pays to a bitcoin cash address. It also
  prints the created script hex and uses the DisasmString function to display
  the disassembled script.

- [Extracting Details from Standard Scripts](http://godoc.org/gitlab.com/thorchain/bifrost/bchd-txscript#example-ExtractPkScriptAddrs)
  Demonstrates extracting information from a standard public key script.

- [Manually Signing a Transaction Output](http://godoc.org/gitlab.com/thorchain/bifrost/bchd-txscript#example-SignTxOutput)
  Demonstrates manually creating and signing a redeem transaction.

## License

Package txscript is licensed under the [copyfree](http://copyfree.org) ISC
License.
