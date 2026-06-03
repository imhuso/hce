// GitHub-ish colors + display names for the language identifiers our splitter
// emits (see internal/splitter). Unknown languages fall back to a neutral gray.

interface LangMeta {
  name: string;
  color: string;
}

const LANGS: Record<string, LangMeta> = {
  go: { name: "Go", color: "#00ADD8" },
  typescript: { name: "TypeScript", color: "#3178c6" },
  javascript: { name: "JavaScript", color: "#f1e05a" },
  python: { name: "Python", color: "#3572A5" },
  java: { name: "Java", color: "#b07219" },
  rust: { name: "Rust", color: "#dea584" },
  c: { name: "C", color: "#555555" },
  cpp: { name: "C++", color: "#f34b7d" },
  csharp: { name: "C#", color: "#178600" },
  ruby: { name: "Ruby", color: "#701516" },
  php: { name: "PHP", color: "#4F5D95" },
  swift: { name: "Swift", color: "#F05138" },
  kotlin: { name: "Kotlin", color: "#A97BFF" },
  scala: { name: "Scala", color: "#c22d40" },
  lua: { name: "Lua", color: "#000080" },
  bash: { name: "Shell", color: "#89e051" },
  markdown: { name: "Markdown", color: "#083fa1" },
  other: { name: "其他", color: "#8b949e" },
};

const FALLBACK: LangMeta = { name: "其他", color: "#8b949e" };

export function langMeta(id: string): LangMeta {
  return LANGS[id.toLowerCase()] ?? { name: id || "其他", color: FALLBACK.color };
}
