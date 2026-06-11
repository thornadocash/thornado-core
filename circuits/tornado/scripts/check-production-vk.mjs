#!/usr/bin/env node
/** Sanity-check vendored vk matches tornado-core v2.1 release metadata. */
import { readFileSync } from 'node:fs';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = join(dirname(fileURLToPath(import.meta.url)), '..');
const manifest = JSON.parse(readFileSync(join(root, 'artifacts/MANIFEST.json'), 'utf8'));
const vk = JSON.parse(readFileSync(join(root, 'artifacts/withdraw_verification_key.json'), 'utf8'));

if (!vk.protocol || !String(vk.protocol).startsWith('groth')) {
  throw new Error(`unexpected vk protocol: ${vk.protocol}`);
}
if (manifest.protocol !== 'tornado-cash-groth16-v2.1') {
  throw new Error('manifest protocol mismatch');
}
console.log('production vk ok:', manifest.release, vk.nPublic, 'public inputs');
