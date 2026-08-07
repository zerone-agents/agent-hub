import { Command, Option } from "clipanion";
import { outputJson } from "../output/json";
import { outputYaml } from "../output/yaml";
import { outputTable } from "../output/table";
import { validateOutput } from "../output/validate";
import {
  listSkills,
  getSkill,
  createSkill,
  updateSkill,
  deleteSkill,
  downloadSkill,
  type Skill,
} from "../client/skill";
import { validateSkillDir, packDir } from "../zip";

// ── Helpers ───────────────────────────────────────────────────

function skillToRow(s: Skill): Record<string, unknown> {
  return {
    name: s.name,
    title: s.title || s.description?.slice(0, 30) || "-",
    type: s.type || "-",
    size: s.fileSize ? `${(s.fileSize / 1024).toFixed(0)}KB` : "-",
    hash: s.fileHash ? s.fileHash.slice(0, 12) : "-",
  };
}

function renderSkill(item: Skill | Skill[], output: string) {
  if (output === "json") {
    outputJson(item);
  } else if (output === "yaml") {
    outputYaml(item);
  } else {
    const arr = Array.isArray(item) ? item : [item];
    outputTable(
      arr.map(skillToRow),
      ["name", "title", "type", "size", "hash"],
    );
  }
}

interface PreparedSkillUpload {
  skillName: string;
  zipBuffer: Buffer;
}

async function prepareSkillUpload(
  fromDir: string,
  requestedName?: string,
): Promise<PreparedSkillUpload | null> {
  if (!requestedName) {
    process.stderr.write("错误：必须用 --name 指定 skill 名称\n");
    return null;
  }
  const result = await validateSkillDir(fromDir);
  if (!result.valid) {
    for (const err of result.errors) process.stderr.write(`错误：${err}\n`);
    return null;
  }
  process.stderr.write(`正在打包...（找到 ${result.skills.length} 个 SKILL.md）\n`);
  const zipBuffer = await packDir(fromDir, requestedName);
  process.stderr.write(`已打包 ${zipBuffer.length} 字节\n`);
  return { skillName: requestedName, zipBuffer };
}

// ── List ──────────────────────────────────────────────────────

export class SkillListCommand extends Command {
  static paths = [["skill", "list"]];
  static usage = Command.Usage({ description: "列出所有 skill" });

  output = Option.String("--output", "table");
  type = Option.String("--type", { description: "按类型筛选（expert / community）" });

  async execute(): Promise<number> {
    const invalid = validateOutput(this.output);
    if (invalid !== null) return invalid;
    const list = await listSkills(this.type || undefined);
    renderSkill(list, this.output);
    return 0;
  }
}

// ── Get ───────────────────────────────────────────────────────

export class SkillGetCommand extends Command {
  static paths = [["skill", "get"]];
  static usage = Command.Usage({ description: "查看 skill 详情" });

  name = Option.String();
  output = Option.String("--output", "yaml");

  async execute(): Promise<number> {
    const invalid = validateOutput(this.output);
    if (invalid !== null) return invalid;
    const sk = await getSkill(this.name);
    renderSkill(sk, this.output);
    return 0;
  }
}

// ── Create (pack + upload) ────────────────────────────────────

export class SkillCreateCommand extends Command {
  static paths = [["skill", "create"]];
  static usage = Command.Usage({
    description: "从目录打包并上传 skill（目录必须包含 SKILL.md）",
  });

  fromDir = Option.String("--from-dir", { description: "skill 目录路径" });
  name = Option.String("--name", { description: "skill 标识名（默认从 SKILL.md frontmatter 读取）" });
  title = Option.String("--title", { description: "展示名（默认从 SKILL.md frontmatter description 读取）" });
  titleEn = Option.String("--title-en", { description: "英文展示名" });
  description = Option.String("--description", { description: "中文描述" });
  descriptionEn = Option.String("--description-en", { description: "英文描述" });
  type = Option.String("--type", { description: "类型，默认 community" });
  output = Option.String("--output", "yaml");

  async execute(): Promise<number> {
    const invalid = validateOutput(this.output);
    if (invalid !== null) return invalid;
    if (!this.fromDir) {
      process.stderr.write("错误：必须提供 --from-dir 参数\n");
      return 1;
    }

    const prepared = await prepareSkillUpload(this.fromDir, this.name);
    if (!prepared) return 2;
    const { skillName } = prepared;

    process.stderr.write("正在上传...\n");
    const created = await createSkill({
      name: skillName,
      title: this.title || skillName,
      titleEn: this.titleEn,
      description: this.description ?? "",
      descriptionEn: this.descriptionEn,
      type: this.type || "community",
      zipBuffer: prepared.zipBuffer,
    });

    renderSkill(created, this.output);
    process.stderr.write(`✓ 已创建 skill：${skillName}\n`);
    return 0;
  }
}

// ── Delete ────────────────────────────────────────────────────

export class SkillUpdateCommand extends Command {
  static paths = [["skill", "update"]];
  static usage = Command.Usage({ description: "从目录打包并更新 skill" });

  name = Option.String();
  fromDir = Option.String("--from-dir", { description: "skill 目录路径" });
  title = Option.String("--title", { description: "展示名" });
  titleEn = Option.String("--title-en", { description: "英文展示名" });
  description = Option.String("--description", { description: "中文描述" });
  descriptionEn = Option.String("--description-en", { description: "英文描述" });
  type = Option.String("--type", { description: "类型" });
  output = Option.String("--output", "yaml");

  async execute(): Promise<number> {
    const invalid = validateOutput(this.output);
    if (invalid !== null) return invalid;
    if (!this.fromDir) {
      process.stderr.write("错误：必须提供 --from-dir 参数\n");
      return 1;
    }
    const prepared = await prepareSkillUpload(this.fromDir, this.name);
    if (!prepared) return 2;
    process.stderr.write("正在上传...\n");
    const updated = await updateSkill(this.name, {
      title: this.title || undefined,
      titleEn: this.titleEn,
      description: this.description ?? "",
      descriptionEn: this.descriptionEn,
      type: this.type || undefined,
      zipBuffer: prepared.zipBuffer,
    });
    renderSkill(updated, this.output);
    process.stderr.write(`✓ 已更新 skill：${this.name}\n`);
    return 0;
  }
}

export class SkillDeleteCommand extends Command {
  static paths = [["skill", "delete"]];
  static usage = Command.Usage({ description: "删除 skill" });

  name = Option.String();

  async execute(): Promise<number> {
    await deleteSkill(this.name);
    console.log(`已删除 skill：${this.name}`);
    return 0;
  }
}

// ── Download ──────────────────────────────────────────────────

export class SkillDownloadCommand extends Command {
  static paths = [["skill", "download"]];
  static usage = Command.Usage({ description: "获取 skill 下载链接" });

  name = Option.String();
  output = Option.String("--output", "table");

  async execute(): Promise<number> {
    const invalid = validateOutput(this.output);
    if (invalid !== null) return invalid;
    const result = await downloadSkill(this.name);
    if (this.output === "json") {
      outputJson(result);
    } else {
      console.log(`下载链接（${result.expiresIn}秒内有效）：`);
      console.log(result.url);
    }
    return 0;
  }
}
