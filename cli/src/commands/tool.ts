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
    description: t.description || "-",
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
        process.stderr.write("错误：无法读取或解析输入文件/JSON\n");
        return null;
      }
      return parsed;
    }
    if (json) {
      const parsed = JSON.parse(json);
      if (!isRecord(parsed)) {
        process.stderr.write("错误：无法读取或解析输入文件/JSON\n");
        return null;
      }
      return parsed;
    }
    return null;
  } catch (err) {
    process.stderr.write("错误：无法读取或解析输入文件/JSON\n");
    return null;
  }
}

export class ToolListCommand extends Command {
  static paths = [["tool", "list"]];
  static usage = Command.Usage({ description: "列出所有 tool" });

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
  static usage = Command.Usage({ description: "查看 tool 详情" });

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
  static usage = Command.Usage({ description: "创建自定义工具（单文件上传）" });

  file = Option.String("--file", { description: "工具元数据 YAML/JSON 文件路径" });
  json = Option.String("--json", { description: "内联 JSON 元数据（name/title/description）" });
  source = Option.String("--source", { description: "工具源文件路径（.ts/.mts/.js/.mjs，必填）" });
  output = Option.String("--output", "yaml");

  async execute(): Promise<number> {
    const invalid = validateOutput(this.output);
    if (invalid !== null) return invalid;
    if (!this.file && !this.json) {
      process.stderr.write("错误：必须提供 --file 或 --json（元数据）\n");
      return 2;
    }
    if (!this.source) {
      process.stderr.write("错误：必须提供 --source（工具源文件路径）\n");
      return 2;
    }
    const body = loadInput(this.file, this.json);
    if (!body) return 2;
    const name = typeof body.name === "string" ? body.name : "";
    if (!name) {
      process.stderr.write("错误：元数据中缺少 name\n");
      return 2;
    }
    let fileBuffer: Buffer;
    try {
      fileBuffer = readFileSync(this.source);
    } catch {
      process.stderr.write(`错误：无法读取源文件 ${this.source}\n`);
      return 2;
    }
    const t = await createTool({
      name,
      title: typeof body.title === "string" ? body.title : undefined,
      description: typeof body.description === "string" ? body.description : undefined,
      fileBuffer,
      fileName: basename(this.source),
    });
    renderTool(t, this.output);
    return 0;
  }
}

export class ToolUpdateCommand extends Command {
  static paths = [["tool", "update"]];
  static usage = Command.Usage({ description: "更新 tool" });

  name = Option.String();
  file = Option.String("--file", { description: "tool.yaml 文件路径" });
  json = Option.String("--json", { description: "内联 JSON 定义" });
  output = Option.String("--output", "yaml");

  async execute(): Promise<number> {
    const invalid = validateOutput(this.output);
    if (invalid !== null) {
      return invalid;
    }
    if (!this.file && !this.json) {
      process.stderr.write("错误：必须提供 --file 或 --json\n");
      return 2;
    }
    const body = loadInput(this.file, this.json);
    if (!body) {
      return 2;
    }
    const t = await updateTool(this.name, body as { title?: string; description?: string });
    renderTool(t, this.output);
    return 0;
  }
}

export class ToolDeleteCommand extends Command {
  static paths = [["tool", "delete"]];
  static usage = Command.Usage({ description: "删除 tool" });

  name = Option.String();

  async execute(): Promise<number> {
    await deleteTool(this.name);
    console.log(`已删除 tool：${this.name}`);
    return 0;
  }
}

export class ToolUploadCommand extends Command {
  static paths = [["tool", "upload"]];
  static usage = Command.Usage({ description: "补传/替换自定义工具文件" });

  name = Option.String();
  source = Option.String("--source", { description: "工具源文件路径（必填）" });
  output = Option.String("--output", "yaml");

  async execute(): Promise<number> {
    const invalid = validateOutput(this.output);
    if (invalid !== null) return invalid;
    if (!this.source) {
      process.stderr.write("错误：必须提供 --source（工具源文件路径）\n");
      return 2;
    }
    let fileBuffer: Buffer;
    try {
      fileBuffer = readFileSync(this.source);
    } catch {
      process.stderr.write(`错误：无法读取源文件 ${this.source}\n`);
      return 2;
    }
    const t = await uploadToolFile(this.name, { fileBuffer, fileName: basename(this.source) });
    renderTool(t, this.output);
    return 0;
  }
}

export class ToolDownloadCommand extends Command {
  static paths = [["tool", "download"]];
  static usage = Command.Usage({ description: "获取自定义工具下载地址" });

  name = Option.String();

  async execute(): Promise<number> {
    const res = await downloadTool(this.name);
    console.log(res.url);
    if (res.expiresIn > 0) {
      process.stderr.write(`有效期 ${res.expiresIn} 秒\n`);
    }
    return 0;
  }
}
