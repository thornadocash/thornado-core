#!/usr/bin/env node
/**
 * Generates Pedersen / MiMC / Merkle test vectors for thornado-shielder Rust tests.
 */
import { writeFileSync, mkdirSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
const __dirname = dirname(fileURLToPath(import.meta.url));

const { buildPedersenHash, buildMimcSponge, buildBabyjub } = require('circomlibjs');

async function main() {
  const pedersen = await buildPedersenHash();
  const mimcSponge = await buildMimcSponge();
  const babyJub = await buildBabyjub();

  const fieldMod = BigInt(
    '21888242871839275222246405745257275088548364400416034343698204186575808495617',
  );

  function fieldFromHex(hex) {
    return BigInt('0x' + hex.replace(/^0x/i, '')) % fieldMod;
  }

  function leBytesToBigint(bytes) {
    let value = 0n;
    for (let i = bytes.length - 1; i >= 0; i--) {
      value = (value << 8n) + BigInt(bytes[i]);
    }
    return value % fieldMod;
  }

  function pedersenFieldDecimal(value) {
    return babyJub.F.toString(value);
  }

  function fieldToDecimal(value) {
    const v = typeof value === 'bigint' ? value : BigInt(value);
    const norm = ((v % fieldMod) + fieldMod) % fieldMod;
    return norm.toString();
  }

  function mimcHashLeftRight(left, right) {
    return mimcSponge.F.toString(mimcSponge.multiHash([left, right], 0n, 1));
  }

  function zeroLeaf() {
    return 0n;
  }

  function zeroSubtree(level) {
    let value = zeroLeaf();
    for (let i = 0; i < level; i++) {
      value = mimcHashLeftRight(value, value);
    }
    return value;
  }

  function incrementalRoot(leaves) {
    const depth = 20;
    const filled = Array(depth).fill(zeroLeaf());
    let root = zeroSubtree(depth);
    for (let index = 0; index < leaves.length; index++) {
      let currentIndex = index;
      let current = leaves[index];
      for (let level = 0; level < depth; level++) {
        if (currentIndex % 2 === 0) {
          filled[level] = current;
          current = mimcHashLeftRight(current, zeroSubtree(level));
        } else {
          current = mimcHashLeftRight(filled[level], current);
        }
        currentIndex = Math.floor(currentIndex / 2);
      }
      root = current;
    }
    return root;
  }

  function commitmentHasher(nullifierHex, secretHex) {
    const nullifier = fieldFromHex(nullifierHex);
    const secret = fieldFromHex(secretHex);
    const nullifierBuff = bigintToLeBytes(nullifier, 31);
    const secretBuff = bigintToLeBytes(secret, 31);
    const commitmentMsg = Buffer.concat([nullifierBuff, secretBuff]);
    const commitmentPoint = pedersen.hash(commitmentMsg);
    const nullifierPoint = pedersen.hash(nullifierBuff);
    return {
      commitment: pedersenFieldDecimal(babyJub.unpackPoint(commitmentPoint)[0]),
      nullifierHash: pedersenFieldDecimal(babyJub.unpackPoint(nullifierPoint)[0]),
    };
  }

  function bigintToLeBytes(value, length) {
    let v = ((value % fieldMod) + fieldMod) % fieldMod;
    const buff = Buffer.alloc(length);
    for (let i = 0; i < length; i++) {
      buff[i] = Number(v & 0xffn);
      v >>= 8n;
    }
    return buff;
  }

  const vectors = { pedersen: [], mimc: [], merkle: [] };

  for (const [nf, sec] of [
    [
      '0101010101010101010101010101010101010101010101010101010101010101',
      '0202020202020202020202020202020202020202020202020202020202020202',
    ],
    [
      '0303030303030303030303030303030303030303030303030303030303030303',
      '0404040404040404040404040404040404040404040404040404040404040404',
    ],
  ]) {
    vectors.pedersen.push({ nullifier: nf, secret: sec, ...commitmentHasher(nf, sec) });
  }

  const left = fieldFromHex('0101010101010101010101010101010101010101010101010101010101010101');
  const right = fieldFromHex('0202020202020202020202020202020202020202020202020202020202020202');
  vectors.mimc.push({
    left: fieldToDecimal(left),
    right: fieldToDecimal(right),
    hash: fieldToDecimal(mimcHashLeftRight(left, right)),
  });

  const leaf = BigInt(vectors.pedersen[0].commitment);
  vectors.merkle.push({
    leaves: [fieldToDecimal(leaf)],
    root: fieldToDecimal(incrementalRoot([leaf])),
  });

  const outDir = join(__dirname, '../../../crates/thornado-shielder/testdata');
  mkdirSync(outDir, { recursive: true });
  const outPath = join(outDir, 'tornado_vectors.json');
  writeFileSync(outPath, JSON.stringify(vectors, null, 2) + '\n');
  console.log('wrote', outPath);
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
