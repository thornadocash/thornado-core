//! Dump a signer.redb txout store: cargo run --example dump_store -- <path>
fn main() -> anyhow::Result<()> {
    let path = std::env::args().nth(1).expect("usage: dump_store <signer.redb>");
    let store = thornado_bifrost_signer::store::SignerStore::open(&path)?;
    let items = store.list()?;
    println!("items: {}", items.len());
    for it in items {
        println!(
            "height={} idx={} epoch={} status={:?} deferred_until={} retry_until={} out_hash={:?} vault=..{} in_hash=..{}",
            it.height,
            it.index,
            it.epoch,
            it.status,
            it.deferred_until_height,
            it.retry_until_height,
            &it.item.out_hash.get(..8).unwrap_or(&it.item.out_hash),
            &it.item.vault_pub_key[it.item.vault_pub_key.len().saturating_sub(8)..],
            &it.item.in_hash[it.item.in_hash.len().saturating_sub(8)..],
        );
    }
    Ok(())
}
