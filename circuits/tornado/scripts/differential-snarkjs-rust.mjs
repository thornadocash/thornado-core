#!/usr/bin/env node
/**
 * Differential audit: snarkjs vk/proof checks + Rust native crypto + optional full prove path.
 */
import { createRequire } from 'node:module';
import { existsSync, readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { spawnSync } from 'node:child_process';

const require = createRequire(import.meta.url);
const __dirname = dirname(fileURLToPath(import.meta.url));
const root = join(__dirname, '..');
const repoRoot = join(root, '../..');
const artifacts = join(root, 'artifacts');

function run(cmd, args, opts = {}) {
  const res = spawnSync(cmd, args, { stdio: 'inherit', ...opts });
  if (res.status !== 0) {
    process.exit(res.status ?? 1);
  }
}

function wasmPath() {
  const candidates = [
    join(artifacts, 'withdraw.wasm'),
    join(root, 'build', 'withdraw.wasm'),
  ];
  return candidates.find((path) => existsSync(path));
}

async function main() {
  run('node', [join(__dirname, 'check-production-artifacts.mjs')]);
  run('node', [join(__dirname, 'gen-rust-test-vectors.mjs')]);

  const vkPath = join(artifacts, 'withdraw_verification_key.json');
  const vk = JSON.parse(readFileSync(vkPath, 'utf8'));
  if (!String(vk.protocol).startsWith('groth') || vk.nPublic !== 6) {
    throw new Error('unexpected production vk metadata');
  }

  const snarkjs = require('snarkjs');
  const manifest = JSON.parse(readFileSync(join(artifacts, 'MANIFEST.json'), 'utf8'));
  const vkDigest = manifest.withdraw_verification_key_sha256;
  const { createHash } = await import('node:crypto');
  const actualVkDigest = createHash('sha256').update(readFileSync(vkPath)).digest('hex');
  if (actualVkDigest !== vkDigest) {
    throw new Error('vk digest mismatch against MANIFEST');
  }
  console.log('snarkjs vk metadata ok:', vk.protocol, vk.nPublic, 'public inputs');

  run('cargo', ['test', '-p', 'thornado-shielder', '--', '--nocapture'], { cwd: repoRoot });

  const wasm = wasmPath();
  const zkey = join(artifacts, 'withdraw_proving_key.json');
  if (wasm && existsSync(zkey)) {
    console.log('running full snarkjs prove -> Rust verify differential');
    run(
      'cargo',
      [
        'test',
        '-p',
        'thornado-shielder',
        '--features',
        'proof-tests',
        'groth16_withdraw_roundtrip',
        '--',
        '--nocapture',
      ],
      { cwd: repoRoot, env: { ...process.env, SHIELDER_WITHDRAW_WASM: wasm } },
    );
  } else {
    console.log(
      'skipping full groth16 roundtrip (missing withdraw.wasm); vk + native crypto checks passed',
    );
  }
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
