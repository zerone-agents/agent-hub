import { Command, Option } from "clipanion";
import { readFileSync } from "node:fs";
import { basename } from "node:path";
import { parse } from "yaml";
import { outputJson } from "../output/json";
import { outputYaml } from "../output/yaml";
import { outputTable } from "../output/table";
import { validateOutput } from "../output/validate";
import {
  listTools,
  getTool,
  createTool,
  updateTool,
  deleteTool,
  uploadToolFile,
  downloadTool,
  type Tool,
} from "../client/tool";

function toolToRow(t: Tool): Record<string, unknown> {
  return {
    name: t.name,
    title: t.title || "-",
    // 英文优先展示（D2 决策：descriptionEn || description），与 skill.ts 的
    // `||` 链回退形态一致；yaml/json 输出由 outputYaml/outputJson 原样透传
    // 完整对象（含 descriptionEn），同 skill.ts 不做列级加工。
    description: t.descriptionEn || t.description || "-",
    source: t.source ?? "-",
    isDefault: t.isDefault ?? false,
  };
}

function renderTool(item: Tool | Tool[], output: string) {
  if (output === "json") {
    outputJson(item);
  } else if (output === "yaml") {
    outputYaml(item);
  } else {
    const arr = Array.isArray(item) ? item : [item];
    outputTable(arr.map(toolToRow), ["name", "title", "description", "source", "isDefault"]);
  }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function loadInput(file?: string, json?: string): Record<string, unknown> | null {
  try {
    if (file) {
      const raw = readFileSync(file, "utf-8");
      const parsed = parse(raw);
      if (!isRecord(parsed)) {
        process.stderr.write("Error: cannot read or parse input file/JSON\n");
        return null;
      }
      return parsed;
    }
    if (json) {
      const parsed = JSON.parse(json);
      if (!isRecord(parsed)) {
        process.stderr.write("Error: cannot read or parse input file/JSON\n");
        return null;
      }
      return parsed;
    }
    return null;
  } catch (err) {
    process.stderr.write("Error: cannot read or parse input file/JSON\n");
    return null;
  }
}

export class ToolListCommand extends Command {
  static paths = [["tool", "list"]];
  static usage = Command.Usage({ description: "List all tools" });

  output = Option.String("--output", "table");

  async execute(): Promise<number> {
    const invalid = validateOutput(this.output);
    if (invalid !== null) {
      return invalid;
    }
    const list = await listTools();
    renderTool(list, this.output);
    return 0;
  }
}

export class ToolGetCommand extends Command {
  static paths = [["tool", "get"]];
  static usage = Command.Usage({ description: "View tool details" });

  name = Option.String();
  output = Option.String("--output", "yaml");

  async execute(): Promise<number> {
    const invalid = validateOutput(this.output);
    if (invalid !== null) {
      return invalid;
    }
    const t = await getTool(this.name);
    renderTool(t, this.output);
    return 0;
  }
}

export class ToolCreateCommand extends Command {
  static paths = [["tool", "create"]];
  static usage = Command.Usage({ description: "Create a custom tool (single file upload)" });

  file = Option.String("--file", { description: "Tool metadata YAML/JSON file path" });
  json = Option.String("--json", { description: "Inline JSON metadata (name/title/description/descriptionEn)" });
  source = Option.String("--source", { description: "Tool source file path (.ts/.mts/.js/.mjs, required)" });
  output = Option.String("--output", "yaml");

  async execute(): Promise<number> {
    const invalid = validateOutput(this.output);
    if (invalid !== null) return invalid;
    if (!this.file && !this.json) {
      process.stderr.write("Error: must provide --file or --json (metadata)\n");
      return 2;
    }
    if (!this.source) {
      process.stderr.write("Error: must provide --source (tool source file path)\n");
      return 2;
    }
    const body = loadInput(this.file, this.json);
    if (!body) return 2;
    const name = typeof body.name === "string" ? body.name : "";
    if (!name) {
      process.stderr.write("Error: metadata is missing name\n");
      return 2;
    }
    let fileBuffer: Buffer;
    try {
      fileBuffer = readFileSync(this.source);
    } catch {
      process.stderr.write(`Error: cannot read source file  ${this.source}\n`);
      return 2;
    }
    const t = await createTool({
      name,
      title: typeof body.title === "string" ? body.title : undefined,
      description: typeof body.description === "string" ? body.description : undefined,
      descriptionEn: typeof body.descriptionEn === "string" ? body.descriptionEn : undefined,
      fileBuffer,
      fileName: basename(this.source),
    });
    renderTool(t, this.output);
    return 0;
  }
}

// tool update 仅支持 title/description/descriptionEn；name 允许出现（复用 create 的
// 同一份元数据文件）但忽略——name 由命令行位置参数指定。其余字段（如 isDefault）
// 服务端契约（UpdateToolInput）不接受、会被 Go JSON 解码器静默忽略，
// 整包转发会造成假成功，故直接报错（对齐 issue #97）。
const UPDATE_ALLOWED_FIELDS = new Set(["name", "title", "description", "descriptionEn"]);

export class ToolUpdateCommand extends Command {
  static paths = [["tool", "update"]];
  static usage = Command.Usage({ description: "Update a tool" });

  name = Option.String();
  file = Option.String("--file", { description: "Path to tool.yaml" });
  json = Option.String("--json", { description: "Inline JSON definition" });
  output = Option.String("--output", "yaml");

  async execute(): Promise<number> {
    const invalid = validateOutput(this.output);
    if (invalid !== null) {
      return invalid;
    }
    if (!this.file && !this.json) {
      process.stderr.write("Error: must provide --file or --json\n");
      return 2;
    }
    const body = loadInput(this.file, this.json);
    if (!body) {
      return 2;
    }
    const unknown = Object.keys(body).filter((k) => !UPDATE_ALLOWED_FIELDS.has(k));
    if (unknown.length > 0) {
      process.stderr.write(
        `Error: unsupported metadata field(s): ${unknown.join(", ")} (tool update supports only title/description/descriptionEn; name is taken from the CLI and ignored)
`,
      );
      return 2;
    }
    // Allowlisted fields must be strings: a non-string value (e.g.
    // {"title":123}) used to be silently dropped, producing an empty PUT that
    // reports success while changing nothing — a fake success for scripts.
    const badType = Object.keys(body).filter((k) => k !== "name" && typeof body[k] !== "string");
    if (badType.length > 0) {
      process.stderr.write(
        `Error: invalid field type: ${badType.join(", ")} (tool update title/description/descriptionEn must be strings)
`,
      );
      return 2;
    }
    const t = await updateTool(this.name, {
      title: typeof body.title === "string" ? body.title : undefined,
      description: typeof body.description === "string" ? body.description : undefined,
      descriptionEn: typeof body.descriptionEn === "string" ? body.descriptionEn : undefined,
    });
    renderTool(t, this.output);
    return 0;
  }
}

export class ToolDeleteCommand extends Command {
  static paths = [["tool", "delete"]];
  static usage = Command.Usage({ description: "Delete a tool" });

  name = Option.String();

  async execute(): Promise<number> {
    await deleteTool(this.name);
    console.log(`Deleted tool: ${this.name}`);
    return 0;
  }
}

export class ToolUploadCommand extends Command {
  static paths = [["tool", "upload"]];
  static usage = Command.Usage({ description: "Upload/replace custom tool file" });

  name = Option.String();
  source = Option.String("--source", { description: "Tool source file path (required)" });
  output = Option.String("--output", "yaml");

  async execute(): Promise<number> {
    const invalid = validateOutput(this.output);
    if (invalid !== null) return invalid;
    if (!this.source) {
      process.stderr.write("Error: must provide --source (tool source file path)\n");
      return 2;
    }
    let fileBuffer: Buffer;
    try {
      fileBuffer = readFileSync(this.source);
    } catch {
      process.stderr.write(`Error: cannot read source file  ${this.source}\n`);
      return 2;
    }
    const t = await uploadToolFile(this.name, { fileBuffer, fileName: basename(this.source) });
    renderTool(t, this.output);
    return 0;
  }
}

export class ToolDownloadCommand extends Command {
  static paths = [["tool", "download"]];
  static usage = Command.Usage({ description: "Get custom tool download URL" });

  name = Option.String();

  async execute(): Promise<number> {
    const res = await downloadTool(this.name);
    console.log(res.url);
    if (res.expiresIn > 0) {
      process.stderr.write(`Valid for ${res.expiresIn}s
`);
    }
    return 0;
  }
}
