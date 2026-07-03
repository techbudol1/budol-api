import { createHash } from "node:crypto";
import { buildPoseidon } from "circomlibjs";

type Request = {
  action: "buildInput" | "createNote" | "marketRoot";
  amount?: string | number;
  marketId?: string | number;
  note?: Partial<PrivateClaimNote>;
  outcome?: string | number;
  pathElements?: Array<string | number>;
  pathIndices?: Array<string | number>;
  resolvedOutcome?: string | number;
  root?: string | number;
  secret?: string | number;
  sortLeaves?: boolean;
  userSalt?: string | number;
  leaves?: Array<string | number>;
};

type PrivateClaimNote = {
  amount: string;
  leaf: string;
  marketId: string;
  nullifierHash: string;
  outcome: string;
  secret: string;
  userSalt: string;
  version: "budol-private-claim-v1";
};

const FIELD_MODULUS = 21888242871839275222246405745257275088548364400416034343698204186575808495617n;
const DEFAULT_TREE_LEVELS = 20;

const body = await new Response(Bun.stdin.stream()).text();
const request = JSON.parse(body || "{}") as Request;
const poseidon = await buildPoseidon();
const field = poseidon.F;

function toBigInt(value: string | number | bigint | undefined, name: string): bigint {
  if (value === undefined || value === null || value === "") {
    throw new Error(`${name} is required`);
  }
  if (typeof value === "bigint") return normalizeField(value);
  if (typeof value === "number") return normalizeField(BigInt(value));
  const trimmed = String(value).trim();
  if (!trimmed) throw new Error(`${name} is required`);
  if (trimmed.startsWith("0x") || /^[0-9]+$/.test(trimmed)) {
    return normalizeField(trimmed.startsWith("0x") ? BigInt(trimmed) : BigInt(trimmed));
  }
  return normalizeField(BigInt(`0x${createHash("sha256").update(trimmed).digest("hex")}`));
}

function normalizeField(value: bigint): bigint {
  const result = value % FIELD_MODULUS;
  return result >= 0n ? result : result + FIELD_MODULUS;
}

function randomField(): bigint {
  const bytes = new Uint8Array(31);
  crypto.getRandomValues(bytes);
  return normalizeField(BigInt(`0x${Array.from(bytes, (byte) => byte.toString(16).padStart(2, "0")).join("")}`));
}

function fieldString(value: bigint): string {
  return normalizeField(value).toString();
}

function hash(inputs: bigint[]): bigint {
  return normalizeField(BigInt(field.toString(poseidon(inputs.map(normalizeField)))));
}

function modInverse(value: bigint): bigint {
  let a = normalizeField(value);
  if (a === 0n) throw new Error("amount must be nonzero");
  let b = FIELD_MODULUS;
  let x0 = 1n;
  let x1 = 0n;
  while (b !== 0n) {
    const quotient = a / b;
    [a, b] = [b, a - quotient * b];
    [x0, x1] = [x1, x0 - quotient * x1];
  }
  return normalizeField(x0);
}

function createNote(input: {
  amount?: string | number;
  marketId?: string | number;
  outcome?: string | number;
  secret?: string | number;
  userSalt?: string | number;
}): PrivateClaimNote {
  const marketId = toBigInt(input.marketId, "marketId");
  const outcome = toBigInt(input.outcome, "outcome");
  const amount = toBigInt(input.amount, "amount");
  const userSalt = input.userSalt === undefined ? randomField() : toBigInt(input.userSalt, "userSalt");
  const secret = input.secret === undefined ? randomField() : toBigInt(input.secret, "secret");
  const leaf = hash([marketId, outcome, amount, userSalt, secret]);
  const nullifierHash = hash([secret, marketId]);
  return {
    amount: fieldString(amount),
    leaf: fieldString(leaf),
    marketId: fieldString(marketId),
    nullifierHash: fieldString(nullifierHash),
    outcome: fieldString(outcome),
    secret: fieldString(secret),
    userSalt: fieldString(userSalt),
    version: "budol-private-claim-v1",
  };
}

function normalizeArray(values: Array<string | number> | undefined, levels: number, name: string): bigint[] {
  const result = values?.map((value, index) => toBigInt(value, `${name}[${index}]`)) || [];
  while (result.length < levels) result.push(0n);
  if (result.length > levels) {
    throw new Error(`${name} must have at most ${levels} entries`);
  }
  return result;
}

function computeRoot(leaf: bigint, pathElements: bigint[], pathIndices: bigint[]): bigint {
  let current = normalizeField(leaf);
  for (let index = 0; index < pathElements.length; index += 1) {
    const direction = normalizeField(pathIndices[index]);
    if (direction !== 0n && direction !== 1n) {
      throw new Error(`pathIndices[${index}] must be 0 or 1`);
    }
    const sibling = normalizeField(pathElements[index]);
    current = direction === 0n ? hash([current, sibling]) : hash([sibling, current]);
  }
  return current;
}

function computeMarketRoot(rawLeaves: Array<string | number> | undefined, levels: number, sortLeaves = true): { leaves: string[]; root: string } {
  const leaves = (rawLeaves || []).map((value, index) => toBigInt(value, `leaves[${index}]`));
  if (sortLeaves) {
    leaves.sort((a, b) => a < b ? -1 : a > b ? 1 : 0);
  }
  if (leaves.length === 0) {
    return { leaves: [], root: "0" };
  }
  const capacity = 2 ** levels;
  if (leaves.length > capacity) {
    throw new Error(`tree supports at most ${capacity} leaves`);
  }
  let layer = [...leaves];
  for (let level = 0; level < levels; level += 1) {
    if (layer.length % 2 !== 0) {
      layer.push(0n);
    }
    const next: bigint[] = [];
    for (let index = 0; index < layer.length; index += 2) {
      next.push(hash([layer[index], layer[index + 1]]));
    }
    layer = next;
  }
  return { leaves: leaves.map(fieldString), root: fieldString(layer[0]) };
}

function noteFromRequest(): PrivateClaimNote {
  if (request.note?.leaf && request.note?.nullifierHash) {
    return request.note as PrivateClaimNote;
  }
  return createNote({
    amount: request.note?.amount || request.amount,
    marketId: request.note?.marketId || request.marketId,
    outcome: request.note?.outcome || request.outcome,
    secret: request.note?.secret || request.secret,
    userSalt: request.note?.userSalt || request.userSalt,
  });
}

if (request.action === "createNote") {
  console.log(JSON.stringify({ note: createNote(request) }, null, 2));
  process.exit(0);
}

if (request.action === "buildInput") {
  const note = noteFromRequest();
  const pathElements = normalizeArray(request.pathElements, DEFAULT_TREE_LEVELS, "pathElements");
  const pathIndices = normalizeArray(request.pathIndices, DEFAULT_TREE_LEVELS, "pathIndices");
  const leaf = toBigInt(note.leaf, "note.leaf");
  const computedRoot = computeRoot(leaf, pathElements, pathIndices);
  const root = request.root === undefined ? computedRoot : toBigInt(request.root, "root");
  const amount = toBigInt(note.amount, "note.amount");
  const circuitInput = {
    root: fieldString(root),
    resolvedOutcome: fieldString(toBigInt(request.resolvedOutcome, "resolvedOutcome")),
    nullifierHash: fieldString(toBigInt(note.nullifierHash, "note.nullifierHash")),
    marketId: note.marketId,
    outcome: note.outcome,
    amount: note.amount,
    amountInverse: fieldString(modInverse(amount)),
    userSalt: note.userSalt,
    secret: note.secret,
    pathElements: pathElements.map(fieldString),
    pathIndices: pathIndices.map(fieldString),
  };
  console.log(JSON.stringify({ circuitInput, note }, null, 2));
  process.exit(0);
}

if (request.action === "marketRoot") {
  console.log(JSON.stringify({ tree: computeMarketRoot(request.leaves, DEFAULT_TREE_LEVELS, request.sortLeaves !== false) }, null, 2));
  process.exit(0);
}

throw new Error(`unsupported action: ${request.action}`);
