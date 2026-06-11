#!/usr/bin/env node
/**
 * Tornado Cash crypto primitives via circomlibjs (matches production circom semantics).
 * stdin: JSON request, stdout: JSON response
 */
import { createRequire } from 'node:module';
import { readFileSync } from 'node:fs';

const require = createRequire(import.meta.url);

const FIELD =
  21888242871839275222246405745257275088548364400416034343698204186575808495617n;

async function load() {
  const { buildPedersenHash, buildMimcSponge, buildBabyjub } = require('circomlibjs');
  return {
    pedersen: await buildPedersenHash(),
    mimc: await buildMimcSponge(),
    babyJub: await buildBabyjub(),
  };
}

function fieldFromDecimal(value) {
  return BigInt(value) % FIELD;
}

function fieldToDecimal(value) {
  return ((value % FIELD) + FIELD) % FIELD;
}

function leBytesToBigint(bytes) {
  let value = 0n;
  for (let i = bytes.length - 1; i >= 0; i -= 1) {
    value = (value << 8n) + BigInt(bytes[i]);
  }
  return fieldToDecimal(value);
}

function bigintToLeBytes(value, length) {
  let v = fieldToDecimal(value);
  const out = Buffer.alloc(length);
  for (let i = 0; i < length; i += 1) {
    out[i] = Number(v & 0xffn);
    v >>= 8n;
  }
  return out;
}

function fieldElementBits(value) {
  return bigintToLeBytes(fieldFromDecimal(value), 31);
}

function pedersenX(pedersen, babyJub, msg) {
  const packed = pedersen.hash(msg);
  return babyJub.F.toString(babyJub.unpackPoint(packed)[0]);
}

function mimcHashLeftRight(mimc, left, right) {
  return mimc.F.toString(mimc.multiHash([fieldFromDecimal(left), fieldFromDecimal(right)], 0n, 1));
}

function zeroSubtree(mimc, level) {
  let value = 0n;
  for (let i = 0; i < level; i += 1) {
    value = fieldFromDecimal(mimcHashLeftRight(mimc, value.toString(), value.toString()));
  }
  return value.toString();
}

function incrementalRoot(mimc, leaves) {
  const depth = 20;
  const filled = Array(depth).fill(0n);
  let root = zeroSubtree(mimc, depth);
  for (let index = 0; index < leaves.length; index += 1) {
    let currentIndex = index;
    let current = fieldFromDecimal(leaves[index]);
    for (let level = 0; level < depth; level += 1) {
      if (currentIndex % 2 === 0) {
        filled[level] = current;
        current = fieldFromDecimal(
          mimcHashLeftRight(mimc, current.toString(), zeroSubtree(mimc, level)),
        );
      } else {
        current = fieldFromDecimal(
          mimcHashLeftRight(mimc, filled[level].toString(), current.toString()),
        );
      }
      currentIndex = Math.floor(currentIndex / 2);
    }
    root = current;
  }
  return root.toString();
}

async function main() {
  const input = JSON.parse(readFileSync(0, 'utf8'));
  const { pedersen, mimc, babyJub } = await load();
  switch (input.op) {
    case 'note_commitment': {
      const nullifier = fieldElementBits(input.nullifier);
      const secret = fieldElementBits(input.secret);
      const msg = Buffer.concat([nullifier, secret]);
      process.stdout.write(JSON.stringify({ value: pedersenX(pedersen, babyJub, msg) }));
      break;
    }
    case 'nullifier_hash': {
      const msg = fieldElementBits(input.nullifier);
      process.stdout.write(JSON.stringify({ value: pedersenX(pedersen, babyJub, msg) }));
      break;
    }
    case 'mimc_hash': {
      process.stdout.write(
        JSON.stringify({ value: mimcHashLeftRight(mimc, input.left, input.right) }),
      );
      break;
    }
    case 'merkle_root': {
      process.stdout.write(JSON.stringify({ value: incrementalRoot(mimc, input.leaves) }));
      break;
    }
    default:
      throw new Error(`unknown op: ${input.op}`);
  }
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
