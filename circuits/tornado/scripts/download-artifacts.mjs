#!/usr/bin/env node
/**
 * Download production Groth16 artifacts from the official Tornado Cash release.
 * Same files as tornadocash/tornado-core scripts/downloadKeys.js (release v2.1).
 */
import { createWriteStream, mkdirSync, existsSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { pipeline } from 'node:stream/promises';

const RELEASE = 'v2.1';
const BASE = `https://github.com/tornadocash/tornado-core/releases/download/${RELEASE}`;
const FILES = [
  'withdraw.json',
  'withdraw_proving_key.bin',
  'withdraw_proving_key.json',
  'withdraw_verification_key.json',
  'Verifier.sol',
  'tornado.params',
  'tornado_no_zeros.params',
];

const root = join(dirname(fileURLToPath(import.meta.url)), '..');
const outDir = join(root, 'artifacts');

async function download(name) {
  const url = `${BASE}/${name}`;
  const dest = join(outDir, name);
  const res = await fetch(url);
  if (!res.ok) {
    throw new Error(`failed to download ${name}: ${res.status} ${res.statusText}`);
  }
  await pipeline(res.body, createWriteStream(dest));
  console.log(`downloaded ${name}`);
}

async function main() {
  if (!existsSync(outDir)) {
    mkdirSync(outDir, { recursive: true });
  }
  for (const file of FILES) {
    await download(file);
  }
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
