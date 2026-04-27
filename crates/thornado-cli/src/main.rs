use anyhow::{bail, Context, Result};
use clap::{Args, Parser, Subcommand};
use std::path::PathBuf;
use thornado_core::{
    derive_split_receipt, execute_command, happy_path_state, load_snapshot, save_snapshot,
    AppState, Command, Event, MockCustodySigner, MockProofVerifier,
};

#[derive(Debug, Parser)]
#[command(name = "thornado")]
#[command(about = "Local Thornado MVP simulator")]
struct Cli {
    #[arg(long, global = true, default_value = "thornado-state.json")]
    state: PathBuf,
    #[command(subcommand)]
    command: TopCommand,
}

#[derive(Debug, Subcommand)]
enum TopCommand {
    Demo(DemoCommand),
    Deposit(DepositCommand),
    Split(SplitArgs),
    Withdraw(WithdrawArgs),
    Churn(ChurnCommand),
    Snapshot(SnapshotCommand),
}

#[derive(Debug, Args)]
struct DemoCommand {
    #[command(subcommand)]
    command: DemoSubcommand,
}

#[derive(Debug, Subcommand)]
enum DemoSubcommand {
    HappyPath,
}

#[derive(Debug, Args)]
struct DepositCommand {
    #[command(subcommand)]
    command: DepositSubcommand,
}

#[derive(Debug, Subcommand)]
enum DepositSubcommand {
    Request {
        #[arg(long)]
        pow_token: String,
        #[arg(long, default_value = "local-client-pubkey")]
        user_pubkey: String,
    },
    Confirm {
        #[arg(long)]
        intent: String,
        #[arg(long)]
        txid: String,
        #[arg(long)]
        amount_sats: u64,
    },
}

#[derive(Debug, Args)]
struct SplitArgs {
    #[arg(long)]
    deposit: String,
    #[arg(long, default_value = "local-client-seed")]
    client_seed: String,
}

#[derive(Debug, Args)]
struct WithdrawArgs {
    #[arg(long)]
    note: String,
    #[arg(long)]
    to: String,
    #[arg(long)]
    fee_sats: u64,
}

#[derive(Debug, Args)]
struct ChurnCommand {
    #[command(subcommand)]
    command: ChurnSubcommand,
}

#[derive(Debug, Subcommand)]
enum ChurnSubcommand {
    Start,
    MarkOffline {
        #[arg(long)]
        node: String,
    },
    ApplyPenalties,
}

#[derive(Debug, Args)]
struct SnapshotCommand {
    #[command(subcommand)]
    command: SnapshotSubcommand,
}

#[derive(Debug, Subcommand)]
enum SnapshotSubcommand {
    Save {
        #[arg(long)]
        path: PathBuf,
    },
    Load {
        #[arg(long)]
        path: PathBuf,
    },
}

fn main() -> Result<()> {
    let cli = Cli::parse();
    let verifier = MockProofVerifier;
    let signer = MockCustodySigner;

    match cli.command {
        TopCommand::Demo(DemoCommand {
            command: DemoSubcommand::HappyPath,
        }) => {
            let (state, receipt) = happy_path_state()?;
            let note = receipt
                .notes
                .first()
                .context("demo did not mint any notes")?;
            print_json(&serde_json::json!({
                "state_hash": thornado_core::state_hash(&state),
                "first_note": note,
                "withdrawal_disabled": "plaintext note withdrawal is disabled; use a witness-hiding ZK proof backend",
                "fee_state": state.fees,
            }))
        }
        TopCommand::Deposit(DepositCommand { command }) => {
            let mut state = load_or_default(&cli.state)?;
            let command = match command {
                DepositSubcommand::Request {
                    pow_token,
                    user_pubkey,
                } => Command::RequestDepositAddress {
                    pow_token,
                    user_pubkey,
                },
                DepositSubcommand::Confirm {
                    intent,
                    txid,
                    amount_sats,
                } => {
                    execute_command(
                        &mut state,
                        Command::ObserveDeposit {
                            intent_id: intent.clone(),
                            txid,
                            amount_sats,
                        },
                        &verifier,
                        &signer,
                    )?;
                    Command::ConfirmDeposit { intent_id: intent }
                }
            };
            let events = execute_command(&mut state, command, &verifier, &signer)?;
            save_snapshot(&state, &cli.state)?;
            print_events(&events)
        }
        TopCommand::Split(args) => {
            let mut state = load_or_default(&cli.state)?;
            let amount = state
                .deposits
                .intents
                .get(&args.deposit)
                .and_then(|intent| intent.amount_sats)
                .context("deposit amount not found")?;
            let receipt = derive_split_receipt(&args.deposit, amount, &args.client_seed)?;
            let events = execute_command(
                &mut state,
                Command::SplitDepositIntoNotes {
                    deposit_id: args.deposit.clone(),
                    note_commitments: receipt.commitments(),
                },
                &verifier,
                &signer,
            )?;
            save_snapshot(&state, &cli.state)?;
            print_json(&serde_json::json!({
                "events": events,
                "receipt": receipt,
            }))
        }
        TopCommand::Withdraw(_args) => {
            bail!("plaintext note withdrawal is disabled; use a witness-hiding ZK proof backend")
        }
        TopCommand::Churn(ChurnCommand { command }) => {
            let mut state = load_or_default(&cli.state)?;
            let command = match command {
                ChurnSubcommand::Start => Command::StartChurnEpoch,
                ChurnSubcommand::MarkOffline { node } => Command::MarkNodeOffline { node_id: node },
                ChurnSubcommand::ApplyPenalties => Command::ApplyChurnPenalties,
            };
            let events = execute_command(&mut state, command, &verifier, &signer)?;
            save_snapshot(&state, &cli.state)?;
            print_events(&events)
        }
        TopCommand::Snapshot(SnapshotCommand { command }) => match command {
            SnapshotSubcommand::Save { path } => {
                let state = load_or_default(&cli.state)?;
                save_snapshot(&state, path)?;
                print_json(&serde_json::json!({
                    "state_hash": thornado_core::state_hash(&state),
                }))
            }
            SnapshotSubcommand::Load { path } => {
                let state = load_snapshot(path)?;
                save_snapshot(&state, &cli.state)?;
                print_json(&serde_json::json!({
                    "state_hash": thornado_core::state_hash(&state),
                }))
            }
        },
    }
}

fn load_or_default(path: &PathBuf) -> Result<AppState> {
    if path.exists() {
        load_snapshot(path).map_err(Into::into)
    } else {
        Ok(AppState::default())
    }
}

fn print_events(events: &[Event]) -> Result<()> {
    print_json(&serde_json::json!({ "events": events }))
}

fn print_json(value: &serde_json::Value) -> Result<()> {
    println!("{}", serde_json::to_string_pretty(value)?);
    Ok(())
}
