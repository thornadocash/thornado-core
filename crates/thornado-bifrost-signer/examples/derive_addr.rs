//! Derive vault taproot addresses: cargo run --example derive_addr -- <bech32 pubkey> <paths...>
fn main() -> anyhow::Result<()> {
    let mut args = std::env::args().skip(1);
    let pk_bech = args.next().expect("pubkey");
    let pk = thornado_bifrost_signer::chain::decode_bech32_pubkey(&pk_bech).map_err(|e| anyhow::anyhow!("{e}"))?;
    for p in args {
        let path: u64 = p.parse()?;
        let vault = thornado_bifrost_signer::tx_builder::TaprootVault::derive(&pk, path).map_err(|e| anyhow::anyhow!("{e}"))?;
        let addr = bitcoin::Address::from_script(vault.script_pubkey().as_script(), bitcoin::Network::Regtest)?;
        println!("path {path}: {addr}");
    }
    Ok(())
}
