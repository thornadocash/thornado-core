use std::net::SocketAddr;
use std::path::PathBuf;

use clap::{Parser, Subcommand};

#[derive(Debug, Parser)]
#[command(name = "thornado-frost")]
#[command(about = "Operator CLI for the Thornado FROST signer sidecar")]
struct Cli {
    #[command(subcommand)]
    command: Command,
}

#[derive(Debug, Subcommand)]
enum Command {
    Serve {
        #[arg(long, default_value = "127.0.0.1:8081")]
        listen: SocketAddr,
        #[arg(long)]
        snapshot: Option<PathBuf>,
    },
    Health {
        #[arg(long, default_value_t = false)]
        signer_loaded: bool,
    },
}

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    let cli = Cli::parse();

    match cli.command {
        Command::Serve { listen, snapshot } => {
            thornado_frost_service::serve(listen, snapshot).await?;
        }
        Command::Health { signer_loaded } => {
            let health = thornado_frost_service::HealthResponse {
                status: "ok",
                service: "thornado-frost-signer",
                signer_loaded,
            };
            println!("{}", serde_json::to_string_pretty(&health)?);
        }
    }

    Ok(())
}
