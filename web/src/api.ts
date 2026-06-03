const API_BASE = (import.meta.env.VITE_API_BASE as string | undefined) ?? 'http://localhost:9528/api/v1';

export interface ApiResponse<T = unknown> {
  code: number;
  message: string;
  data?: T;
}

export interface SearchResult {
  content: string;
  relative_path: string;
  start_line: number;
  end_line: number;
  language: string;
  score: number;
}

export interface IndexInfo {
  codebase_id: string;
  collection: string;
  num_chunks: number;
  languages?: Record<string, number>;
}

export async function searchCode(
  codebaseId: string,
  query: string,
  topK = 5,
): Promise<ApiResponse<SearchResult[]>> {
  const res = await fetch(`${API_BASE}/search`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ codebase_id: codebaseId, query, top_k: topK }),
  });
  return res.json();
}

export async function listIndexes(): Promise<ApiResponse<IndexInfo[]>> {
  const res = await fetch(`${API_BASE}/indexes`);
  return res.json();
}

export async function clearIndex(codebaseId: string): Promise<ApiResponse> {
  const res = await fetch(`${API_BASE}/index?codebase_id=${encodeURIComponent(codebaseId)}`, {
    method: 'DELETE',
  });
  return res.json();
}

export async function healthCheck(): Promise<ApiResponse<{ status: string }>> {
  const res = await fetch(`${API_BASE}/health`);
  return res.json();
}
