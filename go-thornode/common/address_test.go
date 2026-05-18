package common

import (
	"fmt"

	. "gopkg.in/check.v1"
)

type AddressSuite struct{}

var _ = Suite(&AddressSuite{})

func (s *AddressSuite) TestAddress(c *C) {
	c.Assert(AllChains, HasLen, 16)

	c.Check(Address("").IsEmpty(), Equals, true)
	c.Check(NoAddress.Equals(Address("")), Equals, true)

	var notMockNet ChainNetwork
	switch CurrentChainNetwork {
	case StageNet:
		notMockNet = StageNet
	case ChainNet:
		notMockNet = ChainNet
	default:
		notMockNet = MainNet
	}

	addr, err := NewAddress("")
	c.Assert(err, IsNil)
	c.Assert(addr.String(), Equals, "")

	testCases := []struct {
		Address    string
		Chain      Chain  // chain to be checked against
		Chains     Chains // chains that also pass IsChain
		Network    ChainNetwork
		Info       string
		ShouldFail bool
	}{
		{
			Address:    "1lejrrtta9cgr49fuh7ktu3sddhe0ff7wenlpn6",
			ShouldFail: true,
		},
		{
			Address:    "bogus",
			ShouldFail: true,
		},
		{
			Address: "thor1kljxxccrheghavaw97u78le6yy3sdj7h696nl4",
			Chain:   THORChain,
			Network: notMockNet,
		},
		{
			Address: "tthor1x6m28lezv00ugcahqv5w2eagrm9396j2gf6zjpd4auf9mv4h",
			Chain:   THORChain,
			Network: MockNet,
		},
		{
			Address: "0x90f2b1ae50e6018230e90a33f98c7844a0ab635a",
			Chain:   ETHChain,
			Network: CurrentChainNetwork,
		},
		{
			Address:    "0x90f2b1ae50e6018230e90a33f98c7844a0ab635aaaaaaaaa",
			ShouldFail: true,
			Info:       "invalid length",
		},
		{
			Address:    "0x90f2b1ae50e6018230e90a33f98c7844a0ab63zz",
			ShouldFail: true,
			Info:       "invalid hex string",
		},
		{
			Address: "1MirQ9bwyQcGVJPwKUgapu5ouK2E2Ey4gX",
			Chain:   BTCChain,
			Chains:  Chains{BCHChain},
			Network: notMockNet,
			Info:    "mainnet p2pkh",
		},
		{
			Address: "mrX9vMRYLfVy1BnZbc5gZjuyaqH3ZW2ZHz",
			Chain:   BTCChain,
			Chains:  Chains{LTCChain, DOGEChain, BCHChain},
			Network: MockNet,
			Info:    "testnet p2pkh",
		},
		{
			Address: "12MzCDwodF9G1e7jfwLXfR164RNtx4BRVG",
			Chain:   BTCChain,
			Chains:  Chains{BCHChain},
			Network: notMockNet,
			Info:    "mainnet p2pkh",
		},
		{
			Address: "3QJmV3qfvL9SuYo34YihAf3sRCW3qSinyC",
			Chain:   BTCChain,
			Chains:  Chains{BCHChain},
			Network: notMockNet,
			Info:    "mainnet p2sh",
		},
		{
			Address: "3NukJ6fYZJ5Kk8bPjycAnruZkE5Q7UW7i8",
			Chain:   BTCChain,
			Chains:  Chains{BCHChain},
			Network: notMockNet,
			Info:    "mainnet p2sh",
		},
		{
			Address: "2NBFNJTktNa7GZusGbDbGKRZTxdK9VVez3n",
			Chain:   BTCChain,
			Chains:  Chains{BCHChain, DOGEChain},
			Network: MockNet,
			Info:    "mocknet p2sh",
		},
		{
			Address:    "02192d74d0cb94344c9569c2e77901573d8d7903c3ebec3a957724895dca52c6b4",
			ShouldFail: true,
			Info:       "mainnet p2pk compressed (0x02), UTXO SignTx unable to sign for (not a THORChain-supported format)",
		},
		{
			Address:    "03b0bd634234abbb1ba1e986e884185c61cf43e001f9137f23c2c409273eb16e65",
			ShouldFail: true,
			Info:       "mainnet p2pk compressed (0x03), UTXO SignTx unable to sign for (not a THORChain-supported format)",
		},
		{
			Address: "0411db93e1dcdb8a016b49840f8c53bc1eb68a382e97b1482ecad7b148a6909a5cb2" +
				"e0eaddfb84ccf9744464f82e160bfa9b8b64f9d4c03f999b8643f656b412a3",
			ShouldFail: true,
			Info:       "mainnet p2pk uncompressed (0x04), UTXO SignTx unable to sign for (not a THORChain-supported format)",
		},
		{
			Address: "06192d74d0cb94344c9569c2e77901573d8d7903c3ebec3a957724895dca52c6b4" +
				"0d45264838c0bd96852662ce6a847b197376830160c6d2eb5e6a4c44d33f453e",
			ShouldFail: true,
			Info:       "mainnet p2pk hybrid (0x06), UTXO SignTx unable to sign for (not a THORChain-supported format)",
		},
		{
			Address: "07b0bd634234abbb1ba1e986e884185c61cf43e001f9137f23c2c409273eb16e65" +
				"37a576782eba668a7ef8bd3b3cfb1edb7117ab65129b8a2e681f3c1e0908ef7b",
			ShouldFail: true,
			Info:       "mainnet p2pk hybrid (0x07), UTXO SignTx unable to sign for (not a THORChain-supported format)",
		},
		{
			Address: "BC1QW508D6QEJXTDG4Y5R3ZARVARY0C5XW7KV8F3T4",
			Chain:   BTCChain,
			Chains:  Chains{DOGEChain},
			Network: notMockNet,
			Info:    "segwit mainnet p2wpkh v0",
		},
		{
			Address: "bc1qrp33g0q5c5txsp9arysrx4k6zdkfs4nce4xj0gdcccefvpysxf3qccfmv3",
			Chain:   BTCChain,
			Chains:  Chains{DOGEChain},
			Network: notMockNet,
			Info:    "segwit mainnet p2wsh v0",
		},
		{
			Address: "tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx",
			Chain:   BTCChain,
			Chains:  Chains{DOGEChain},
			Network: MockNet,
			Info:    "segwit mocknet p2wpkh v0",
		},
		{
			Address: "tb1qqqqqp399et2xygdj5xreqhjjvcmzhxw4aywxecjdzew6hylgvsesrxh6hy",
			Chain:   BTCChain,
			Chains:  Chains{DOGEChain},
			Network: MockNet,
			Info:    "segwit mocknet p2wsh witness v0",
		},
		{
			Address: "bc1pw508d6qejxtdg4y5r3zarvary0c5xw7kw508d6qejxtdg4y5r3zarvary0c5xw7k7grplx",
			Chain:   BTCChain,
			Network: notMockNet,
			Info:    "segwit mainnet witness v1",
		},
		{
			Address: "BC1SW50QA3JX3S",
			Chain:   BTCChain,
			Network: notMockNet,
			Info:    "segwit mainnet witness v16",
		},
		{
			Address: "bcrt1qqqnde7kqe5sf96j6zf8jpzwr44dh4gkd3ehaqh",
			Chain:   BTCChain,
			Chains:  Chains{DOGEChain},
			Network: MockNet,
		},
		{
			Address: "bc1pfy63nact82mfmts5jv87p2uayxqs29gf8070td7kzhwzx6zc9ruq9u7xy7",
			Chain:   BTCChain,
			Network: notMockNet,
			Info:    "Taproot mainnet (bech32m)",
		},
		{
			Address: "tc1qw508d6qejxtdg4y5r3zarvary0c5xw7kg3g4ty",
			Network: CurrentChainNetwork,
			Info:    "segwit invalid hrp bech32 succeed but IsChain fails",
		},
		{
			Address: "rQwpQ54X5gJyLGg4QGp3HSkjdf3u37NqiZ",
			Chain:   XRPChain,
			Network: CurrentChainNetwork,
			Info:    "Valid XRP address",
		},
		{
			Address: "r4KYDZBbcAaJJ5MQPwRR9apJ2p5EBz8bq8",
			Chain:   XRPChain,
			Network: CurrentChainNetwork,
			Info:    "Valid XRP address",
		},
		{
			Address:    "r4KYDZBbcAaJJ5MQPwRR9apJ2p5EBz8bq",
			ShouldFail: true,
			Info:       "XRP address that decodes, but has incorrect checksum",
		},
		{
			Address: "TKSrg7Ffs81r8QsLMfqRHG82hbFYz2Bw5W",
			Chain:   TRONChain,
			Network: CurrentChainNetwork,
		},
		{
			Address: "mtyBWSzMZaCxJ1xy9apJBZzXz648BZrpJg",
			Chain:   DOGEChain,
			Chains:  Chains{BTCChain, BCHChain, LTCChain},
			Network: MockNet,
		},
		{
			Address: "cosmos1vx6vkpn8mgk7tfv3x6n8kaypw080pa46lf7kjw",
			Chain:   GAIAChain,
			Network: CurrentChainNetwork,
		},
		{
			Address: "noble1vx6vkpn8mgk7tfv3x6n8kaypw080pa46h2t72q",
			Chain:   NOBLEChain,
			Network: CurrentChainNetwork,
		},
		{
			Address: "t1bgJzZzNhbXEgFcRo9XmJHdLadhEAbzFbh",
			Chain:   ZECChain,
			Network: notMockNet,
			Info:    "p2pkh address",
		},
		{
			Address: "t3cNKv7UpFVqcmRJrvFCMiuzGj9ywpKtDP5",
			Chain:   ZECChain,
			Network: notMockNet,
			Info:    "p2sh address",
		},
		{
			Address: "tex1cd8heyc2v8fwe7q78m88x7q9zs2fu82cth96vn",
			Chain:   ZECChain,
			Network: notMockNet,
			Info:    "tex address",
		},
		{
			Address: "tmTX4KQUn6G2jpVosTsqW9xJ6Bcn3hLu8Fx",
			Chain:   ZECChain,
			Network: MockNet,
			Info:    "p2pkh address",
		},
		{
			Address: "t2QMWxnax7xSzK7tbqzCQGYAuqeD7fkK7Mk",
			Chain:   ZECChain,
			Network: MockNet,
			Info:    "p2sh address",
		},
		{
			Address: "textest1cd8heyc2v8fwe7q78m88x7q9zs2fu82cje6vfg",
			Chain:   ZECChain,
			Network: MockNet,
			Info:    "tex address",
		},
		{
			Address: "0x3619ed62358db1c24fc0a180c6e2959cc79e22646c7cf3cb7ffa9cdeb0ab7698",
			Chain:   SUIChain,
			Network: CurrentChainNetwork,
		},
		{
			Address: "addr1vyacmg042vn3r6g9e5uv25sjj4jmm27vewztphr9qrtafkgl35pq2",
			Chain:   ADAChain,
			Network: notMockNet,
		},
		{
			Address: "addr_test1vqacmg042vn3r6g9e5uv25sjj4jmm27vewztphr9qrtafkgyeqa00",
			Chain:   ADAChain,
			Network: MockNet,
		},
	}

	networks := []string{
		"testnet", "mainnet", "mocknet", "stagenet", "chainnet",
	}

	for _, tc := range testCases {
		info := fmt.Sprintf("'%s', (%s)", tc.Address, tc.Info)
		addr, err = NewAddress(tc.Address)
		if tc.ShouldFail {
			c.Assert(err, NotNil, Commentf(info))
			c.Assert(addr, Equals, NoAddress, Commentf(info))
			continue
		}

		// valid address
		c.Assert(err, IsNil)

		testChains := map[Chain]bool{}
		for _, chain := range append(tc.Chains, tc.Chain) {
			testChains[chain] = true
		}

		for chain := range testChains {
			for _, other := range AllChains {
				_, found := testChains[other]

				if found || other.IsEVM() && chain.IsEVM() {
					c.Assert(addr.IsChain(chain), Equals, true, Commentf("is %s: %s", chain, info))
					c.Assert(addr.GetNetwork(tc.Chain), Equals, tc.Network, Commentf("is %s: %s", networks[tc.Network], info))
				} else {
					c.Assert(addr.IsChain(other), Equals, false, Commentf("is not %s: %s", chain, info))
				}
			}
		}

	}

	// Test invalid Noble address (wrong prefix)
	addr, err = NewAddress("cosmos1vx6vkpn8mgk7tfv3x6n8kaypw080pa46lf7kjw")
	c.Assert(err, IsNil)
	c.Check(addr.IsChain(NOBLEChain), Equals, false)

	// Test invalid GAIA address (wrong prefix)
	addr, err = NewAddress("noble1vx6vkpn8mgk7tfv3x6n8kaypw080pa46h2t72q")
	c.Assert(err, IsNil)
	c.Check(addr.IsChain(GAIAChain), Equals, false)
}

func (s *AddressSuite) TestAddressMapping(c *C) {
	// bech32
	addr, err := NewAddress("thor1x6m28lezv00ugcahqv5w2eagrm9396j2gf6zjpd4aulvagh5")
	c.Check(err, IsNil)
	mapped, err := addr.MappedAccAddress()
	c.Check(err, IsNil)
	c.Check(mapped.String(), Equals, "cosmos1x6m28lezv00ugcahqv5w2eagrm9396j2gf6zjpd4aulq9g5n")

	// evm
	addr, err = NewAddress("0x90f2b1ae50e6018230e90a33f98c7844a0ab635a")
	c.Check(err, IsNil)
	mapped, err = addr.MappedAccAddress()
	c.Check(err, IsNil)
	c.Check(mapped.String(), Equals, "cosmos1jretrtjsucqcyv8fpgelnrrcgjs2kc66pfx7w6")

	mapped, err = EVMNullAddress.MappedAccAddress()
	c.Check(err, IsNil)
	c.Check(mapped.String(), Equals, "cosmos1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqnrql8a")

	// segwit
	addr, err = NewAddress("bc1pfy63nact82mfmts5jv87p2uayxqs29gf8070td7kzhwzx6zc9ruq9u7xy7")
	c.Check(err, IsNil)
	mapped, err = addr.MappedAccAddress()
	c.Check(err, IsNil)
	c.Check(mapped.String(), Equals, "cosmos1pfy63nact82mfmts5jv87p2uayxqs29gf8070td7kzhwzx6zc9ruqdehkrz")

	// Invalid
	addr, err = NewAddress("r4KYDZBbcAaJJ5MQPwRR9apJ2p5EBz8bq8")
	c.Check(err, IsNil)
	_, err = addr.MappedAccAddress()
	c.Check(err, NotNil)
}
