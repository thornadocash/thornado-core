use anyhow::Result;
use clap::Parser;
use std::net::SocketAddr;
use std::path::PathBuf;
use thornado_node::{router, NodeState};

#[derive(Debug, Parser)]
#[command(name = "thornado-node")]
#[command(about = "Local Thornado MVP HTTP node")]
struct Cli {
    #[arg(long, default_value = "127.0.0.1:3030")]
    listen: SocketAddr,
    #[arg(long)]
    state: Option<PathBuf>,
    #[arg(long = "peer")]
    peers: Vec<String>,
}

#[tokio::main]
async fn main() -> Result<()> {
    let cli = Cli::parse();
    let state = NodeState::from_snapshot_or_default_with_peers(cli.state, cli.peers)?;
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
