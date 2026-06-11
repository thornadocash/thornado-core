#!/usr/bin/env node
/**
 * Prove a Tornado withdraw using production snarkjs artifacts.
 * stdin: JSON witness/public inputs, stdout: snarkjs groth16 proof JSON
 */
import { createRequire } from 'node:module';
import { existsSync, readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const require = createRequire(import.meta.url);
const __dirname = dirname(fileURLToPath(import.meta.url));
const root = join(__dirname, '..');
const artifacts = join(root, 'artifacts');
const snarkjsOld = require('snarkjs-old');
const buildGroth16 = require('websnark').buildGroth16;

function exactArrayBuffer(buffer) {
  return buffer.buffer.slice(buffer.byteOffset, buffer.byteOffset + buffer.byteLength);
}

function writeUint32(view, offset, value) {
  view.setUint32(offset, value, true);
  return offset + 4;
}

function signalToBigInt(signal) {
  if (typeof signal === 'bigint') {
    return signal;
  }
  if (typeof signal === 'number') {
    return BigInt(signal);
  }
  if (typeof signal === 'string') {
    return BigInt(signal);
  }
  if (signal && typeof signal.toString === 'function') {
    return BigInt(signal.toString());
  }
  throw new Error(`unsupported witness signal type: ${typeof signal}`);
}

function witnessToBuffer(witness) {
  const buffer = Buffer.alloc(witness.length * 32);
  const view = new DataView(buffer.buffer, buffer.byteOffset, buffer.byteLength);
  let offset = 0;
  for (const signal of witness) {
    const value = signalToBigInt(signal);
    if (value < 0n) {
      throw new Error('negative witness signal');
    }
    for (let word = 0; word < 8; word += 1) {
      offset = writeUint32(
        view,
        offset,
        Number((value >> BigInt(word * 32)) & 0xffffffffn),
      );
    }
  }
  return buffer;
}

async function main() {
  const input = JSON.parse(readFileSync(0, 'utf8'));
  const circuitPath = join(artifacts, 'withdraw.json');
  const provingKeyPath = join(artifacts, 'withdraw_proving_key.bin');
  if (!existsSync(circuitPath) || !existsSync(provingKeyPath)) {
    throw new Error(
      'missing production proving artifacts; run `npm run download-artifacts` in circuits/tornado',
    );
  }
  const wtns = {
    nullifier: input.nullifier,
    secret: input.secret,
    pathElements: input.pathElements,
    pathIndices: input.pathIndices,
    root: input.root,
    nullifierHash: input.nullifierHash,
    recipient: input.recipient,
    relayer: input.relayer,
    fee: input.fee,
    refund: input.refund,
  };
  const circuit = new snarkjsOld.Circuit(JSON.parse(readFileSync(circuitPath, 'utf8')));
  const witness = circuit.calculateWitness(snarkjsOld.unstringifyBigInts(wtns));
  const publicSignals = witness
    .slice(1, 1 + circuit.nPubInputs)
    .map((signal) => signal.toString());
  if (
    publicSignals[0] !== input.root ||
    publicSignals[1] !== input.nullifierHash
  ) {
    throw new Error('public signal mismatch during prove');
  }
  const witnessBuffer = witnessToBuffer(witness);
  const provingKey = readFileSync(provingKeyPath);
  const groth16 = await buildGroth16();
  let proof;
  try {
    proof = snarkjsOld.stringifyBigInts(
      await groth16.proof(
        exactArrayBuffer(witnessBuffer),
        exactArrayBuffer(provingKey),
      ),
    );
  } finally {
    if (typeof groth16.terminate === 'function') {
      groth16.terminate();
    }
  }
  process.stdout.write(
    JSON.stringify({
      pi_a: proof.pi_a,
      pi_b: proof.pi_b,
      pi_c: proof.pi_c,
      protocol: proof.protocol || 'groth',
    }),
  );
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
