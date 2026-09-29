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
    process.stderr.write("Error: --name is required to specify the skill name\n");
    return null;
  }
  const result = await validateSkillDir(fromDir);
  if (!result.valid) {
    for (const err of result.errors) process.stderr.write(`Error: ${err}\n`);
    return null;
  }
  process.stderr.write(`Packing... (found  ${result.skills.length} SKILL.md files)\n`);
  const zipBuffer = await packDir(fromDir, requestedName);
  process.stderr.write(`Packed ${zipBuffer.length} bytes\n`);
  return { skillName: requestedName, zipBuffer };
}

// ── List ──────────────────────────────────────────────────────

export class SkillListCommand extends Command {
  static paths = [["skill", "list"]];
  static usage = Command.Usage({ description: "List all skills" });

  output = Option.String("--output", "table");
  type = Option.String("--type", { description: "Filter by type (expert / community)" });

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
  static usage = Command.Usage({ description: "View skill details" });

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
    description: "Pack and upload a skill from a directory (must contain SKILL.md)",
  });

  fromDir = Option.String("--from-dir", { description: "Skill directory path" });
  name = Option.String("--name", { description: "Skill name (required)" });
  title = Option.String("--title", { description: "Display title (defaults to the --name value)" });
  titleEn = Option.String("--title-en", { description: "English display title" });
  description = Option.String("--description", { description: "Description (Chinese)" });
  descriptionEn = Option.String("--description-en", { description: "English description" });
  type = Option.String("--type", { description: "Type, defaults to community" });
  output = Option.String("--output", "yaml");

  async execute(): Promise<number> {
    const invalid = validateOutput(this.output);
    if (invalid !== null) return invalid;
    if (!this.fromDir) {
      process.stderr.write("Error: must provide --from-dir\n");
      return 1;
    }

    const prepared = await prepareSkillUpload(this.fromDir, this.name);
    if (!prepared) return 2;
    const { skillName } = prepared;

    process.stderr.write("Uploading...\n");
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
    process.stderr.write(`✓ Created skill: ${skillName}\n`);
    return 0;
  }
}

// ── Delete ────────────────────────────────────────────────────

export class SkillUpdateCommand extends Command {
  static paths = [["skill", "update"]];
  static usage = Command.Usage({ description: "Pack and update a skill from a directory" });

  name = Option.String();
  fromDir = Option.String("--from-dir", { description: "Skill directory path" });
  title = Option.String("--title", { description: "Display title" });
  titleEn = Option.String("--title-en", { description: "English display title" });
  description = Option.String("--description", { description: "Description (Chinese)" });
  descriptionEn = Option.String("--description-en", { description: "English description" });
  type = Option.String("--type", { description: "Type" });
  output = Option.String("--output", "yaml");

  async execute(): Promise<number> {
    const invalid = validateOutput(this.output);
    if (invalid !== null) return invalid;
    if (!this.fromDir) {
      process.stderr.write("Error: must provide --from-dir\n");
      return 1;
    }
    const prepared = await prepareSkillUpload(this.fromDir, this.name);
    if (!prepared) return 2;
    process.stderr.write("Uploading...\n");
    const updated = await updateSkill(this.name, {
      title: this.title || undefined,
      titleEn: this.titleEn,
      description: this.description ?? "",
      descriptionEn: this.descriptionEn,
      type: this.type || undefined,
      zipBuffer: prepared.zipBuffer,
    });
    renderSkill(updated, this.output);
    process.stderr.write(`✓ Updated skill: ${this.name}\n`);
    return 0;
  }
}

export class SkillDeleteCommand extends Command {
  static paths = [["skill", "delete"]];
  static usage = Command.Usage({ description: "Delete a skill" });

  name = Option.String();

  async execute(): Promise<number> {
    await deleteSkill(this.name);
    console.log(`Deleted skill: ${this.name}`);
    return 0;
  }
}

// ── Download ──────────────────────────────────────────────────

export class SkillDownloadCommand extends Command {
  static paths = [["skill", "download"]];
  static usage = Command.Usage({ description: "Get a skill download URL" });

  name = Option.String();
  output = Option.String("--output", "table");

  async execute(): Promise<number> {
    const invalid = validateOutput(this.output);
    if (invalid !== null) return invalid;
    const result = await downloadSkill(this.name);
    if (this.output === "json") {
      outputJson(result);
    } else {
      console.log(`Download URL (valid ${result.expiresIn}s):`);
      console.log(result.url);
    }
    return 0;
  }
}
