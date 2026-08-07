import { glob, readdir, stat, readFile } from "node:fs/promises";
import { join, relative } from "node:path";
import { parse } from "yaml";
import JSZip from "jszip";

export interface SkillEntry {
  /** SKILL.md path relative to input dir (POSIX separators), e.g. "commit/SKILL.md" or "SKILL.md". */
  path: string;
  /** frontmatter.name (undefined when frontmatter missing or name absent). */
  name?: string;
  /** frontmatter.description (undefined when frontmatter missing or description absent). */
  description?: string;
}

export interface ValidateResult {
  valid: boolean;
  errors: string[];
  /** Every SKILL.md found under dir (sorted by path). Includes entries even
   * when their frontmatter has errors, so callers can show what was found. */
  skills: SkillEntry[];
}

const MAX_SIZE = 50 * 1024 * 1024; // 50MB
const EXCLUDED = new Set([".git", "node_modules", ".DS_Store", "dist", "build", ".zerone-uploads"]);

/**
 * Validate a skill directory before packing. Accepts both single-skill
 * (top-level SKILL.md) and bundle layouts (one or more nested SKILL.md
 * anywhere in the tree), matching the SDK's `loadSkillsFromDir` glob
 * semantics so what we validate here is exactly what the runtime will
 * register.
 *
 * Checks:
 * 1. At least one SKILL.md exists under dir (excluding build/hidden dirs).
 * 2. Each SKILL.md has a YAML frontmatter block with non-empty `name` and
 *    `description`. SDK requires `description` (silent skip if missing);
 *    hub additionally requires `name` so the runtime skill name is
 *    deterministic regardless of zip parent-dir layout.
 * 3. Total dir size (excluding hidden/build dirs) ≤ 50MB.
 *
 * Every error is prefixed with the SKILL.md relative path so the user
 * can tell which file in a bundle failed.
 */
export async function validateSkillDir(dir: string): Promise<ValidateResult> {
  const errors: string[] = [];
  const skills: SkillEntry[] = [];

  // Glob all SKILL.md anywhere in the tree (Node 22+ fs.promises.glob).
  // Matches SDK's `loadSkillsFromDir` semantics so what we validate at
  // upload time is exactly what the runtime will register.
  // Wrap in try-catch: glob throws ENOENT on a nonexistent cwd (Bun/Node
  // both do this); treat that as "no SKILL.md found".
  // Filter post-glob against EXCLUDED dir-name segments; Node's glob
  // supports `exclude` predicate (not `ignore` array), so portable code
  // filters results manually.
  const rawMatches: string[] = [];
  try {
    for await (const entry of glob("**/SKILL.md", { cwd: dir })) {
      rawMatches.push(entry);
    }
  } catch {
    // cwd missing/unreadable — fall through with empty matches
  }
  const matches = rawMatches
    .filter((p) => {
      const segments = p.split("/");
      // Last segment is "SKILL.md"; check parent path segments only.
      return segments.slice(0, -1).every((s) => !EXCLUDED.has(s));
    })
    .sort();

  if (matches.length === 0) {
    errors.push("缺少 SKILL.md 文件");
  } else {
    for (const relPath of matches) {
      const entry: SkillEntry = { path: relPath };
      let content: string;
      try {
        content = await readFile(join(dir, relPath), "utf-8");
      } catch (err) {
        errors.push(`${relPath}: 读取失败 (${(err as Error).message})`);
        skills.push(entry);
        continue;
      }

      const fmMatch = content.match(/^---\n([\s\S]*?)\n---/);
      if (!fmMatch) {
        errors.push(`${relPath}: frontmatter 缺失或未闭合（必须以 --- 开头并以 --- 结束）`);
        skills.push(entry);
        continue;
      }
      const fm = parse(fmMatch[1]) as Record<string, unknown>;
      if (!fm.name) {
        errors.push(`${relPath}: frontmatter 缺少 name 字段`);
      } else {
        entry.name = String(fm.name);
      }
      if (!fm.description) {
        errors.push(`${relPath}: frontmatter 缺少 description 字段`);
      } else {
        entry.description = String(fm.description);
      }
      skills.push(entry);
    }
  }

  const totalSize = await dirSize(dir);
  if (totalSize > MAX_SIZE) {
    errors.push(`目录大小 ${(totalSize / 1024 / 1024).toFixed(1)}MB 超过上限 50MB`);
  }

  return { valid: errors.length === 0, errors, skills };
}

async function dirSize(dir: string): Promise<number> {
  let total = 0;
  let entries: import("node:fs").Dirent[];
  try {
    entries = await readdir(dir, { withFileTypes: true });
  } catch {
    // Dir missing/unreadable — treat as zero size so validateSkillDir can
    // still surface the more useful "缺少 SKILL.md" error rather than crash.
    return 0;
  }
  for (const entry of entries) {
    if (EXCLUDED.has(entry.name)) continue;
    const full = join(dir, entry.name);
    if (entry.isDirectory()) {
      total += await dirSize(full);
    } else {
      const st = await stat(full);
      total += st.size;
    }
  }
  return total;
}

/**
 * Pack a skill directory into a zip Buffer.
 * Files are placed at the archive root preserving their relative paths
 * (e.g. input `./SKILL.md` → zip `SKILL.md`; input `./commit/SKILL.md` →
 * zip `commit/SKILL.md`). No wrapper directory is added — the deployer
 * already extracts under `skillsDir/<hub-name>/`, so wrapping would
 * produce a redundant `skillsDir/<name>/<name>/SKILL.md` path.
 *
 * `skillName` is kept in the signature for callers that pass it through
 * from the command layer; it is intentionally unused here.
 * Excludes .git, node_modules, dist, build, .DS_Store directories.
 */
export async function packDir(dir: string, _skillName: string): Promise<Buffer> {
  const zip = new JSZip();
  const files = await collectFiles(dir, dir);
  for (const filePath of files) {
    const relPath = relative(dir, filePath);
    const content = await readFile(filePath);
    zip.file(relPath, content);
  }
  return zip.generateAsync({ type: "nodebuffer" }) as Promise<Buffer>;
}

async function collectFiles(root: string, current: string): Promise<string[]> {
  const results: string[] = [];
  const entries = await readdir(current, { withFileTypes: true });
  for (const entry of entries) {
    if (EXCLUDED.has(entry.name)) continue;
    const full = join(current, entry.name);
    if (entry.isDirectory()) {
      results.push(...await collectFiles(root, full));
    } else {
      results.push(full);
    }
  }
  return results;
}
