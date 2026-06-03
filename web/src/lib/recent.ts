// Recent search history (per browser), used for one-click re-runs.

export interface RecentSearch {
  id: string;
  query: string;
  ts: number;
}

const STORAGE_KEY = "hce.recent.v1";
const MAX = 8;

export function loadRecent(): RecentSearch[] {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return [];
    const arr = JSON.parse(raw);
    return Array.isArray(arr) ? arr.filter((r) => r?.id && r?.query) : [];
  } catch {
    return [];
  }
}

export function pushRecent(id: string, query: string): RecentSearch[] {
  const q = query.trim();
  if (!id || !q) return loadRecent();
  const next = [
    { id, query: q, ts: Date.now() },
    ...loadRecent().filter((r) => !(r.id === id && r.query === q)),
  ].slice(0, MAX);
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(next));
  } catch {
    /* ignore */
  }
  return next;
}
