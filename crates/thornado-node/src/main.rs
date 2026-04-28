use anyhow::Result;
use clap::Parser;
use std::net::SocketAddr;
use std::path::PathBuf;
use thornado_bitcoin::BitcoinRpcConfig;
use thornado_node::{router, NodeConfig, NodeState};

#[derive(Debug, Parser)]
#[command(name = "thornado-node")]
#[command(about = "Local Thornado MVP HTTP node")]
struct Cli {
    #[arg(long, default_value = "127.0.0.1:3030")]
    listen: SocketAddr,
    #[arg(long)]
    state: Option<PathBuf>,
    #[arg(long)]
    node_id: Option<String>,
    #[arg(long)]
    frost_signer: Option<PathBuf>,
    #[arg(long)]
    bitcoin_state: Option<PathBuf>,
    #[arg(long = "peer")]
    peers: Vec<String>,
    #[arg(long)]
    cometbft_rpc: Option<String>,
    #[arg(long)]
    bitcoin_rpc_url: Option<String>,
    #[arg(long)]
    bitcoin_rpc_user: Option<String>,
    #[arg(long)]
    bitcoin_rpc_password: Option<String>,
    #[arg(long)]
    churn_cycle_secs: Option<u64>,
}

#[tokio::main]
async fn main() -> Result<()> {
    let cli = Cli::parse();
    let bitcoin_rpc = cli.bitcoin_rpc_url.map(|url| BitcoinRpcConfig {
        url,
        user: cli
            .bitcoin_rpc_user
            .or_else(|| std::env::var("BITCOIN_RPC_USER").ok())
            .unwrap_or_else(|| "user".to_string()),
        password: cli
            .bitcoin_rpc_password
            .or_else(|| std::env::var("BITCOIN_RPC_PASSWORD").ok())
            .unwrap_or_else(|| "password".to_string()),
    });
    let mut state = NodeState::from_config_or_default(
        NodeConfig {
            snapshot_path: cli.state,
            frost_signer_path: cli.frost_signer,
            bitcoin_state_path: cli.bitcoin_state,
            bitcoin_rpc,
            node_id: cli.node_id,
            churn_cycle_ms: cli.churn_cycle_secs.map(|secs| secs.saturating_mul(1000)),
        },
        cli.peers,
    )?;
    if let Some(rpc) = cli.cometbft_rpc {
        state = state.with_cometbft_rpc(rpc);
    }
    let listener = tokio::net::TcpListener::bind(cli.listen).await?;
    println!(
        "thornado-node listening on http://{}",
        listener.local_addr()?
    );
    axum::serve(listener, router(state))
        .with_graceful_shutdown(shutdown_signal())
        .await?;
    Ok(())
}

async fn shutdown_signal() {
    let _ = tokio::signal::ctrl_c().await;
}
