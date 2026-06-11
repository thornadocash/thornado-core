#!/usr/bin/env node
/**
 * Pin production Groth16 artifact digests (v2.1) for CI and release audits.
 */
import { createHash } from 'node:crypto';
import { readFileSync, existsSync } from 'node:fs';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = join(dirname(fileURLToPath(import.meta.url)), '..');
const artifacts = join(root, 'artifacts');
const manifestPath = join(artifacts, 'MANIFEST.json');

function sha256(path) {
  return createHash('sha256').update(readFileSync(path)).digest('hex');
}

function requireFile(name) {
  const path = join(artifacts, name);
  if (!existsSync(path)) {
    throw new Error(`missing artifact ${name}; run npm run download-artifacts`);
  }
  return path;
}

const manifest = JSON.parse(readFileSync(manifestPath, 'utf8'));
const checks = [
  ['withdraw_verification_key.json', 'withdraw_verification_key_sha256'],
  ['withdraw_proving_key.json', 'withdraw_proving_key_sha256'],
];

for (const [file, key] of checks) {
  const expected = manifest[key];
  if (!expected) {
    throw new Error(`MANIFEST.json missing ${key}`);
  }
  const path = requireFile(file);
  const digest = sha256(path);
  if (digest !== expected) {
    throw new Error(`${file} sha256 mismatch: expected ${expected}, got ${digest}`);
  }
  console.log(`${file}: ${digest}`);
}

console.log('production artifacts ok:', manifest.release);
