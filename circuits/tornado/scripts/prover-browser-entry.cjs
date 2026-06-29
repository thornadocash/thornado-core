const snarkjsOld = require('snarkjs-old');
const bigInt = require('snarkjs-old/src/bigint');
const buildGroth16 = require('websnark').buildGroth16;
const { WitnessCalculatorBuilder } = require('circom_runtime/build/main.cjs');

function exactArrayBuffer(buffer) {
  if (buffer instanceof ArrayBuffer) return buffer;
  return buffer.buffer.slice(buffer.byteOffset, buffer.byteOffset + buffer.byteLength);
}
function writeUint32(view, offset, value) {
  view.setUint32(offset, value, true);
  return offset + 4;
}
function signalToBigInt(signal) {
  if (typeof signal === 'bigint') return signal;
  if (typeof signal === 'number') return BigInt(signal);
  if (typeof signal === 'string') return BigInt(signal);
  if (signal && typeof signal.toString === 'function') return BigInt(signal.toString());
  throw new Error(`unsupported witness signal type: ${typeof signal}`);
}
function witnessToBuffer(witness) {
  const buffer = new ArrayBuffer(witness.length * 32);
  const view = new DataView(buffer);
  let offset = 0;
  for (const signal of witness) {
    const value = signalToBigInt(signal);
    if (value < 0n) throw new Error('negative witness signal');
    for (let word = 0; word < 8; word += 1) {
      offset = writeUint32(view, offset, Number((value >> BigInt(word * 32)) & 0xffffffffn));
    }
  }
  return buffer;
}
function fieldFromBinWitness(witnessBuffer, index) {
  const bytes = witnessBuffer instanceof Uint8Array ? witnessBuffer : new Uint8Array(witnessBuffer);
  const start = index * 32;
  if (bytes.length < start + 32) {
    throw new Error('binary witness too short');
  }
  let value = 0n;
  for (let i = 31; i >= 0; i -= 1) {
    value = (value << 8n) + BigInt(bytes[start + i]);
  }
  return value.toString();
}
async function proveWithdraw(input, witnessWasmBuffer, provingKeyBuffer) {
  return proveWithdrawWithWasm(input, witnessWasmBuffer, provingKeyBuffer);
}
async function proveWithdrawWithJson(input, circuitJson, provingKeyBuffer) {
  const wtns = withdrawWitnessInput(input);
  const circuit = new snarkjsOld.Circuit(circuitJson);
  const witness = calculateWitnessQueued(circuit, snarkjsOld.unstringifyBigInts(wtns));
  const publicSignals = witness.slice(1, 1 + circuit.nPubInputs).map((signal) => signal.toString());
  if (publicSignals[0] !== input.root || publicSignals[1] !== input.nullifierHash) {
    throw new Error('public signal mismatch during prove');
  }
  return proveWitness(witness, provingKeyBuffer);
}
async function proveWithdrawWithWasm(input, witnessWasmBuffer, provingKeyBuffer) {
  const wtns = withdrawWitnessInput(input);
  const witnessCalculator = await WitnessCalculatorBuilder(exactArrayBuffer(witnessWasmBuffer));
  const witnessBin = await witnessCalculator.calculateBinWitness(snarkjsOld.unstringifyBigInts(wtns));
  const publicSignals = [1, 2, 3, 4, 5, 6].map((index) => fieldFromBinWitness(witnessBin, index));
  if (publicSignals[0] !== input.root || publicSignals[1] !== input.nullifierHash) {
    throw new Error('public signal mismatch during prove');
  }
  return proveWitnessBin(witnessBin, provingKeyBuffer);
}
function withdrawWitnessInput(input) {
  return {
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
}
function calculateWitnessQueued(circuit, inputSignals, options) {
  options = options || {};
  if (!options.logFunction) options.logFunction = console.log;
  const ctx = new QueuedRTCtx(circuit, options);

  function iterateSelector(values, sels, cb) {
    if (!Array.isArray(values)) {
      return cb(sels, values);
    }
    for (let i = 0; i < values.length; i += 1) {
      sels.push(i);
      iterateSelector(values[i], sels, cb);
      sels.pop();
    }
  }

  ctx.setSignal('one', [], bigInt(1));
  ctx.drainTriggers();

  for (const c in ctx.notInitSignals) {
    if (ctx.notInitSignals[c] === 0) ctx.queueComponent(c);
  }
  ctx.drainTriggers();

  for (const s in inputSignals) {
    ctx.currentComponent = 'main';
    iterateSelector(inputSignals[s], [], (selector, value) => {
      if (typeof value === 'undefined') throw new Error(`Signal not defined:${s}`);
      ctx.setSignal(s, selector, bigInt(value));
      ctx.drainTriggers();
    });
  }

  for (let i = 0; i < circuit.nInputs; i += 1) {
    const idx = circuit.inputIdx(i);
    if (typeof ctx.witness[idx] === 'undefined') {
      throw new Error(`Input Signal not assigned: ${circuit.signalNames(idx)}`);
    }
  }

  ctx.drainTriggers();
  for (let i = 0; i < ctx.witness.length; i += 1) {
    if (typeof ctx.witness[i] === 'undefined') {
      ctx.drainTriggers();
    }
    if (typeof ctx.witness[i] === 'undefined') {
      throw new Error(`Signal not assigned: ${circuit.signalNames(i)}`);
    }
    if (options.logOutput) options.logFunction(`${circuit.signalNames(i)} --> ${ctx.witness[i].toString()}`);
  }
  return ctx.witness.slice(0, circuit.nVars);
}

class QueuedRTCtx {
  constructor(circuit, options) {
    this.options = options;
    this.scopes = [];
    this.circuit = circuit;
    this.witness = new Array(circuit.nSignals);
    this.notInitSignals = {};
    this.pendingTriggers = [];
    this.pendingTriggerSet = {};
    this.draining = false;
    for (const c in this.circuit.components) {
      this.notInitSignals[c] = this.circuit.components[c].inputSignals;
    }
  }

  _sels2str(sels) {
    let res = '';
    for (let i = 0; i < sels.length; i += 1) {
      res += `[${sels[i]}]`;
    }
    return res;
  }

  queueComponent(c) {
    if (this.notInitSignals[c] !== 0 || this.pendingTriggerSet[c]) return;
    this.pendingTriggerSet[c] = true;
    this.pendingTriggers.push(c);
  }

  drainTriggers() {
    if (this.draining) {
      while (this.pendingTriggers.length) {
        const c = this.pendingTriggers.shift();
        delete this.pendingTriggerSet[c];
        if (this.notInitSignals[c] === 0) this.triggerComponent(c);
      }
      return;
    }
    this.draining = true;
    while (this.pendingTriggers.length) {
      const c = this.pendingTriggers.shift();
      delete this.pendingTriggerSet[c];
      if (this.notInitSignals[c] === 0) this.triggerComponent(c);
    }
    this.draining = false;
  }

  setPin(componentName, componentSels, signalName, signalSels, value) {
    let fullName = componentName === 'one' ? 'one' : `${this.currentComponent}.${componentName}`;
    fullName += `${this._sels2str(componentSels)}.${signalName}${this._sels2str(signalSels)}`;
    this.setSignalFullName(fullName, value);
  }

  setSignal(name, sels, value) {
    let fullName = this.currentComponent ? `${this.currentComponent}.${name}` : name;
    fullName += this._sels2str(sels);
    this.setSignalFullName(fullName, value);
  }

  triggerComponent(c) {
    if (this.options.logTrigger) this.options.logFunction(`Component Triggered: ${this.circuit.components[c].name}`);
    this.notInitSignals[c] -= 1;
    const oldComponent = this.currentComponent;
    this.currentComponent = this.circuit.components[c].name;
    const template = this.circuit.components[c].template;
    const newScope = {};
    for (const p in this.circuit.components[c].params) {
      newScope[p] = this.circuit.components[c].params[p];
    }
    const oldScope = this.scopes;
    this.scopes = [this.scopes[0], newScope];
    this.circuit.templates[template](this);
    this.scopes = oldScope;
    this.currentComponent = oldComponent;
    if (this.options.logTrigger) this.options.logFunction(`End Component Triggered: ${this.circuit.components[c].name}`);
  }

  callFunction(functionName, params) {
    const newScope = {};
    for (let p = 0; p < this.circuit.functions[functionName].params.length; p += 1) {
      const paramName = this.circuit.functions[functionName].params[p];
      newScope[paramName] = params[p];
    }
    const oldScope = this.scopes;
    this.scopes = [this.scopes[0], newScope];
    const res = this.circuit.functions[functionName].func(this);
    this.scopes = oldScope;
    return res;
  }

  setSignalFullName(fullName, value) {
    if (this.options.logSet) this.options.logFunction(`set ${fullName} <-- ${value.toString()}`);
    const sId = this.circuit.getSignalIdx(fullName);
    const firstInit = typeof this.witness[sId] === 'undefined';
    this.witness[sId] = bigInt(value);
    const callComponents = [];
    for (let i = 0; i < this.circuit.signals[sId].triggerComponents.length; i += 1) {
      const idCmp = this.circuit.signals[sId].triggerComponents[i];
      if (firstInit) this.notInitSignals[idCmp] -= 1;
      callComponents.push(idCmp);
    }
    callComponents.forEach((c) => {
      if (this.notInitSignals[c] === 0) this.queueComponent(c);
    });
    return this.witness[sId];
  }

  setVar(name, sels, value) {
    function setVarArray(a, sels2, nextValue) {
      if (sels2.length === 1) {
        a[sels2[0]] = nextValue;
      } else {
        if (typeof a[sels2[0]] === 'undefined') a[sels2[0]] = [];
        setVarArray(a[sels2[0]], sels2.slice(1), nextValue);
      }
    }
    const scope = this.scopes[this.scopes.length - 1];
    if (sels.length === 0) {
      scope[name] = value;
    } else {
      if (typeof scope[name] === 'undefined') scope[name] = [];
      setVarArray(scope[name], sels, value);
    }
    return value;
  }

  getVar(name, sels) {
    function select(a, sels2) {
      return sels2.length === 0 ? a : select(a[sels2[0]], sels2.slice(1));
    }
    for (let i = this.scopes.length - 1; i >= 0; i -= 1) {
      if (typeof this.scopes[i][name] !== 'undefined') return select(this.scopes[i][name], sels);
    }
    throw new Error(`Variable not defined: ${name}`);
  }

  getSignal(name, sels) {
    let fullName = name === 'one' ? 'one' : `${this.currentComponent}.${name}`;
    fullName += this._sels2str(sels);
    return this.getSignalFullName(fullName);
  }

  getPin(componentName, componentSels, signalName, signalSels) {
    let fullName = componentName === 'one' ? 'one' : `${this.currentComponent}.${componentName}`;
    fullName += `${this._sels2str(componentSels)}.${signalName}${this._sels2str(signalSels)}`;
    return this.getSignalFullName(fullName);
  }

  getSignalFullName(fullName) {
    const sId = this.circuit.getSignalIdx(fullName);
    if (typeof this.witness[sId] === 'undefined') {
      this.drainTriggers();
    }
    if (typeof this.witness[sId] === 'undefined') {
      throw new Error(`Signal not initialized: ${fullName}`);
    }
    if (this.options.logGet) this.options.logFunction(`get --->${fullName} = ${this.witness[sId].toString()}`);
    return this.witness[sId];
  }

  assert(a, b, errStr) {
    const ba = bigInt(a);
    const bb = bigInt(b);
    if (!ba.equals(bb)) {
      throw new Error(`Constraint doesn't match ${this.currentComponent}: ${errStr} -> ${ba.toString()} != ${bb.toString()}`);
    }
  }
}
async function proveWitness(witness, provingKeyBuffer) {
  return proveWitnessBin(witnessToBuffer(witness), provingKeyBuffer);
}
async function proveWitnessBin(witnessBuffer, provingKeyBuffer) {
  const groth16 = await buildGroth16();
  try {
    const proof = snarkjsOld.stringifyBigInts(await groth16.proof(exactArrayBuffer(witnessBuffer), exactArrayBuffer(provingKeyBuffer)));
    return { pi_a: proof.pi_a, pi_b: proof.pi_b, pi_c: proof.pi_c, protocol: proof.protocol || 'groth' };
  } finally {
    if (typeof groth16.terminate === 'function') groth16.terminate();
  }
}
self.ThornadoBrowserProver = { proveWithdraw, proveWithdrawWithJson, proveWithdrawWithWasm };
