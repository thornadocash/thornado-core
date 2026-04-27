use anyhow::{Context, Result};
use clap::Parser;
use std::path::PathBuf;
use tendermint_abci::ServerBuilder;
use thornado_abci::ThornadoAbciApp;
use thornado_core::{load_snapshot, AppState};

const DEFAULT_READ_BUF_SIZE: usize = 1024 * 1024;

#[derive(Debug, Parser)]
#[command(about = "Run the Thornado ABCI application for a local CometBFT node")]
struct Cli {
    /// TCP address CometBFT should connect to as proxy_app.
    #[arg(long, default_value = "127.0.0.1:26658")]
    listen: String,

    /// JSON snapshot to use as deterministic genesis state.
    #[arg(long)]
    genesis_state: Option<PathBuf>,

    /// Per-connection ABCI read buffer size.
    #[arg(long, default_value_t = DEFAULT_READ_BUF_SIZE)]
    read_buf_size: usize,
}

fn main() -> Result<()> {
    let cli = Cli::parse();
    let state = match cli.genesis_state {
        Some(path) => {
            load_snapshot(&path).with_context(|| format!("failed to load {}", path.display()))?
        }
        None => AppState::default(),
    };

    let server = ServerBuilder::new(cli.read_buf_size)
        .bind(&cli.listen, ThornadoAbciApp::new(state))
        .with_context(|| format!("failed to bind ABCI server at {}", cli.listen))?;
    eprintln!("thornado-abci listening on {}", server.local_addr());
    server.listen().context("ABCI server stopped")
}
