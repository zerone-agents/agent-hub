import { Command, Option } from "clipanion";
import { readFileSync } from "node:fs";
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
  type Tool,
  type CreateToolInput,
  type UpdateToolInput,
} from "../client/tool";

function toolToRow(t: Tool): Record<string, unknown> {
  return {
    name: t.name,
    title: t.title || "-",
    description: t.description || "-",
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
    outputTable(arr.map(toolToRow), ["name", "title", "description", "isDefault"]);
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
  static usage = Command.Usage({ description: "创建 tool" });

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
    const t = await createTool(body as CreateToolInput);
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
    const t = await updateTool(this.name, body as UpdateToolInput);
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
