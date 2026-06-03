import { collectionName } from "./sha256";

// The server hashes codebase_id one-way into a collection name and never stores
// the id itself, so the web UI can't discover which codebases exist from the API
// alone. Instead we remember every codebase_id this browser has successfully
// used (in localStorage), which powers the history dropdown and lets us map a
// remembered id back to an opaque server collection for search / delete.

export interface RememberedCodebase {
  id: string;
  label?: string;
  lastUsed: number;
}

const STORAGE_KEY = "hce.codebases.v1";

export function loadCodebases(): RememberedCodebase[] {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return [];
    const arr = JSON.parse(raw);
    if (!Array.isArray(arr)) return [];
    return arr
      .filter((c): c is RememberedCodebase => typeof c?.id === "string")
      .sort((a, b) => (b.lastUsed ?? 0) - (a.lastUsed ?? 0));
  } catch {
    return [];
  }
}

function save(list: RememberedCodebase[]) {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(list));
  } catch {
    /* ignore quota / disabled storage */
  }
}

export function rememberCodebase(id: string): RememberedCodebase[] {
  const trimmed = id.trim();
  if (!trimmed) return loadCodebases();
  const list = loadCodebases();
  const existing = list.find((c) => c.id === trimmed);
  if (existing) existing.lastUsed = Date.now();
  else list.push({ id: trimmed, lastUsed: Date.now() });
  save(list);
  return loadCodebases();
}

export function forgetCodebase(id: string): RememberedCodebase[] {
  const list = loadCodebases().filter((c) => c.id !== id);
  save(list);
  return list;
}

/**
 * Split a codebase_id (`<dirName>-<hash8>`) into a readable name + short hash,
 * e.g. "lookah-150babfd" → { name: "lookah", hash: "150babfd" }.
 */
export function prettyId(id: string): { name: string; hash: string } {
  const i = id.lastIndexOf("-");
  if (i > 0 && i < id.length - 1) {
    return { name: id.slice(0, i), hash: id.slice(i + 1) };
  }
  return { name: id, hash: "" };
}

export { collectionName };
