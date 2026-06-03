// Tiny, dependency-free syntax highlighter.
//
// Search results span many languages, so instead of pulling in a multi-hundred-KB
// grammar library we do a single-pass scan that classifies the universally-shared
// token shapes — comments, strings, numbers and a common keyword set — picking the
// comment/string style from the language family. It's "good enough to read", not a
// full parser, and adds ~2KB to the bundle.

export type TokClass = "plain" | "comment" | "string" | "number" | "keyword";

export interface Tok {
  t: string;
  c: TokClass;
}

const KEYWORDS = new Set([
  // declarations / structure
  "func", "function", "fn", "def", "lambda", "class", "struct", "interface", "enum",
  "trait", "impl", "type", "module", "mod", "namespace", "package", "import", "export",
  "from", "use", "using", "include", "require", "const", "let", "var", "val", "static",
  "final", "public", "private", "protected", "internal", "abstract", "virtual", "override",
  "extends", "implements", "pub", "mut", "ref", "out", "async", "await", "yield", "defer", "go",
  // control flow
  "if", "else", "elif", "for", "while", "do", "switch", "case", "match", "when", "default",
  "break", "continue", "return", "goto", "try", "catch", "except", "finally", "throw",
  "throws", "raise", "with", "as", "in", "of", "is", "and", "or", "not", "then", "end",
  "begin", "pass", "where", "select", "loop",
  // types / literals
  "int", "uint", "long", "short", "byte", "char", "float", "double", "bool", "boolean",
  "string", "str", "void", "any", "unknown", "never", "object", "map", "list", "vec",
  "true", "false", "null", "nil", "none", "undefined", "this", "self", "super", "new",
  "delete", "typeof", "instanceof", "sizeof",
]);

interface LangCfg {
  line: string[];
  block?: [string, string];
  quotes: string;
}

function configFor(language: string): LangCfg {
  const l = (language || "").toLowerCase();
  if (l.includes("sql")) {
    return { line: ["--"], block: ["/*", "*/"], quotes: "'" };
  }
  const hash = [
    "python", "py", "ruby", "rb", "shell", "bash", "sh", "zsh", "fish", "yaml", "yml",
    "toml", "ini", "perl", "makefile", "dockerfile", "conf", "nginx", "graphql", "elixir",
  ].some((x) => l.includes(x));
  if (hash) {
    return { line: ["#"], quotes: "\"'" };
  }
  if (l.includes("lua")) {
    return { line: ["--"], block: ["--[[", "]]"], quotes: "\"'" };
  }
  // C-family default (go, js, ts, java, rust, c, cpp, kotlin, swift, php, scala, …)
  return { line: ["//"], block: ["/*", "*/"], quotes: "\"'`" };
}

const ID_START = /[A-Za-z_$]/;
const ID_CHAR = /[A-Za-z0-9_$]/;
const NUM_CHAR = /[0-9a-fA-FxXoObB._]/;

/** Highlight `code` into an array of lines, each a list of classified tokens. */
export function highlightLines(code: string, language: string): Tok[][] {
  const cfg = configFor(language);
  const out: Tok[] = [];
  const n = code.length;
  let plain = "";
  const flush = () => {
    if (plain) {
      out.push({ t: plain, c: "plain" });
      plain = "";
    }
  };

  let i = 0;
  while (i < n) {
    const ch = code[i];

    if (cfg.block && code.startsWith(cfg.block[0], i)) {
      flush();
      const end = code.indexOf(cfg.block[1], i + cfg.block[0].length);
      const stop = end === -1 ? n : end + cfg.block[1].length;
      out.push({ t: code.slice(i, stop), c: "comment" });
      i = stop;
      continue;
    }

    let lineComment: string | undefined;
    for (const m of cfg.line) {
      if (code.startsWith(m, i)) {
        lineComment = m;
        break;
      }
    }
    if (lineComment) {
      flush();
      let end = code.indexOf("\n", i);
      if (end === -1) end = n;
      out.push({ t: code.slice(i, end), c: "comment" });
      i = end;
      continue;
    }

    if (cfg.quotes.includes(ch)) {
      flush();
      const multiline = ch === "`";
      let j = i + 1;
      while (j < n) {
        if (code[j] === "\\") {
          j += 2;
          continue;
        }
        if (code[j] === ch) {
          j++;
          break;
        }
        if (!multiline && code[j] === "\n") break;
        j++;
      }
      out.push({ t: code.slice(i, j), c: "string" });
      i = j;
      continue;
    }

    if (ch >= "0" && ch <= "9") {
      flush();
      let j = i + 1;
      while (j < n && NUM_CHAR.test(code[j])) j++;
      out.push({ t: code.slice(i, j), c: "number" });
      i = j;
      continue;
    }

    if (ID_START.test(ch)) {
      let j = i + 1;
      while (j < n && ID_CHAR.test(code[j])) j++;
      const word = code.slice(i, j);
      if (KEYWORDS.has(word)) {
        flush();
        out.push({ t: word, c: "keyword" });
      } else {
        plain += word;
      }
      i = j;
      continue;
    }

    plain += ch;
    i++;
  }
  flush();

  // Split tokens on newlines into per-line token arrays.
  const lines: Tok[][] = [[]];
  for (const tok of out) {
    const parts = tok.t.split("\n");
    for (let k = 0; k < parts.length; k++) {
      if (k > 0) lines.push([]);
      if (parts[k]) lines[lines.length - 1].push({ t: parts[k], c: tok.c });
    }
  }
  return lines;
}

export const TOK_CLASS: Record<TokClass, string> = {
  plain: "",
  comment: "text-zinc-500 italic",
  string: "text-emerald-400",
  number: "text-amber-300",
  keyword: "text-sky-400",
};
