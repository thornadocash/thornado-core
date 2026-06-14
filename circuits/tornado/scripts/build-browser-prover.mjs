#!/usr/bin/env node
import { copyFileSync, cpSync, mkdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { execFileSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const root = join(__dirname, '..');
const repoRoot = join(root, '..', '..');
const temp = join(root, 'build-browser');
const outDir = join(repoRoot, 'go-thornado', 'ui', 'static', 'prover');

rmSync(temp, { recursive: true, force: true });
mkdirSync(temp, { recursive: true });
cpSync(join(root, 'circom'), join(temp, 'circom'), { recursive: true });
cpSync(join(root, 'vendor'), join(temp, 'vendor'), { recursive: true });
mkdirSync(join(temp, 'build'), { recursive: true });

for (const file of [
  join(temp, 'vendor', 'circomlib', 'circuits', 'pedersen.circom'),
  join(temp, 'vendor', 'circomlib', 'circuits', 'mimcsponge.circom'),
]) {
  let source = readFileSync(file, 'utf8');
  source = source.replace(/var BASE\s*=\s*\[/, 'var BASE[10][2] = [');
  const partial = source.match(/var c_partial\s*=\s*\[([\s\S]*?)\n\s*\]/);
  if (partial) {
    const count = (partial[1].match(/\d+/g) || []).length;
    source = source.replace(/var c_partial\s*=\s*\[/, `var c_partial[${count}] = [`);
  }
  writeFileSync(file, source);
}

execFileSync(
  join(root, 'node_modules', '.bin', 'circom'),
  [
    join(temp, 'circom', 'withdraw.circom'),
    '-r',
    join(temp, 'build', 'withdraw.r1cs'),
    '-w',
    join(temp, 'build', 'withdraw.wasm'),
    '-s',
    join(temp, 'build', 'withdraw.sym'),
    '-l',
    join(temp, 'vendor', 'circomlib', 'circuits'),
  ],
  { cwd: root, stdio: 'inherit' },
);

mkdirSync(outDir, { recursive: true });
copyFileSync(join(root, 'artifacts', 'withdraw.json'), join(outDir, 'withdraw.json'));
copyFileSync(join(temp, 'build', 'withdraw.wasm'), join(outDir, 'withdraw.wasm'));
copyFileSync(join(root, 'artifacts', 'withdraw_proving_key.bin'), join(outDir, 'withdraw_proving_key.bin'));

execFileSync(
  join(root, 'node_modules', '.bin', 'browserify'),
  [
    join(root, 'scripts', 'prover-browser-entry.cjs'),
    '--ignore',
    'worker_threads',
    '-o',
    join(outDir, 'prover.bundle.js'),
  ],
  { cwd: root, stdio: 'inherit' },
);

console.log(`browser prover artifacts written to ${outDir}`);
rmSync(temp, { recursive: true, force: true });
