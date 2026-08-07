import { Command, Option } from "clipanion";
import { readFileSync } from "node:fs";
import { parse } from "yaml";
import { outputJson } from "../output/json";
import { outputYaml } from "../output/yaml";
import { outputTable } from "../output/table";
import { validateOutput } from "../output/validate";
import {
  listMcps,
  getMcp,
  createMcp,
  updateMcp,
  deleteMcp,
  probeMcp,
  probeMcpByConfig,
  type Mcp,
  type CreateMcpInput,
  type UpdateMcpInput,
  type McpProbeResult,
  type McpProbeInput,
} from "../client/mcp";

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function mcpToRow(m: Mcp): Record<string, unknown> {
  return {
    name: m.name,
    title: m.title || "-",
    description: m.description || "-",
  };
}

function renderMcp(item: Mcp | Mcp[], output: string) {
  if (output === "json") {
    outputJson(item);
  } else if (output === "yaml") {
    outputYaml(item);
  } else {
    const arr = Array.isArray(item) ? item : [item];
    outputTable(arr.map(mcpToRow), ["name", "title", "description"]);
  }
}

function renderProbeResult(result: McpProbeResult, output: string) {
  if (output === "json") {
    outputJson(result);
  } else if (output === "yaml") {
    outputYaml(result);
  } else {
    if (result.status === "unsupported") {
      console.log("状态：暂不支持探测（SSE transport）");
    } else if (result.status === "failed") {
      console.log("状态：探测失败");
      console.log(`错误：${result.error || ""}`);
    } else {
      console.log("状态：探测成功");
      console.log(`tools 数量：${result.tools?.length ?? 0}`);
      result.tools?.forEach((t) => console.log(`  - ${t.name}`));
    }
  }
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
    // Caller must validate before calling loadInput
    return null;
  } catch (err) {
    process.stderr.write("错误：无法读取或解析输入文件/JSON\n");
    return null;
  }
}

export class McpListCommand extends Command {
  static paths = [["mcp", "list"]];
  static usage = Command.Usage({ description: "列出所有 MCP" });

  output = Option.String("--output", "table");

  async execute(): Promise<number> {
    const invalid = validateOutput(this.output);
    if (invalid !== null) {
      return invalid;
    }
    const list = await listMcps();
    renderMcp(list, this.output);
    return 0;
  }
}

export class McpGetCommand extends Command {
  static paths = [["mcp", "get"]];
  static usage = Command.Usage({ description: "查看 MCP 详情" });

  name = Option.String();
  output = Option.String("--output", "yaml");

  async execute(): Promise<number> {
    const invalid = validateOutput(this.output);
    if (invalid !== null) {
      return invalid;
    }
    const m = await getMcp(this.name);
    renderMcp(m, this.output);
    return 0;
  }
}

export class McpCreateCommand extends Command {
  static paths = [["mcp", "create"]];
  static usage = Command.Usage({ description: "创建 MCP" });

  file = Option.String("--file", { description: "mcp.yaml 文件路径" });
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

    // Auto-probe before create
    const probeResult = await probeMcpByConfig(body as unknown as McpProbeInput);
    if (probeResult.status !== "success") {
      process.stderr.write(`探测失败：${probeResult.error || ""}\n`);
      return 1;
    }

    const payload = { ...(body as CreateMcpInput), tools: probeResult.tools };
    const m = await createMcp(payload);
    renderMcp(m, this.output);
    return 0;
  }
}

export class McpUpdateCommand extends Command {
  static paths = [["mcp", "update"]];
  static usage = Command.Usage({ description: "更新 MCP" });

  name = Option.String();
  file = Option.String("--file", { description: "mcp.yaml 文件路径" });
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

    // Check if connection fields changed
    const existing = await getMcp(this.name);
    const configChanged =
      body.url !== existing.url ||
      body.transportType !== existing.transportType;

    if (configChanged) {
      const probeResult = await probeMcp(this.name);
      if (probeResult.status !== "success") {
        process.stderr.write(`探测失败：${probeResult.error || ""}\n`);
        return 1;
      }
    }

    const m = await updateMcp(this.name, body as UpdateMcpInput);
    renderMcp(m, this.output);
    return 0;
  }
}

export class McpDeleteCommand extends Command {
  static paths = [["mcp", "delete"]];
  static usage = Command.Usage({ description: "删除 MCP" });

  name = Option.String();

  async execute(): Promise<number> {
    await deleteMcp(this.name);
    console.log(`已删除 MCP：${this.name}`);
    return 0;
  }
}

export class McpProbeCommand extends Command {
  static paths = [["mcp", "probe"]];
  static usage = Command.Usage({ description: "探测 MCP 并获取 tools 列表" });

  name = Option.String({ required: false });
  file = Option.String("--file", { description: "mcp.yaml 文件路径" });
  json = Option.String("--json", { description: "内联 JSON 定义" });
  output = Option.String("--output", "table");

  async execute(): Promise<number> {
    const invalid = validateOutput(this.output);
    if (invalid !== null) return invalid;

    if (!this.name && !this.file && !this.json) {
      process.stderr.write("错误：必须提供 MCP name 或 --file 或 --json\n");
      return 2;
    }

    let result: McpProbeResult;
    if (this.name) {
      result = await probeMcp(this.name);
    } else {
      const body = loadInput(this.file, this.json);
      if (!body) return 2;
      result = await probeMcpByConfig(body as unknown as McpProbeInput);
    }

    renderProbeResult(result, this.output);
    return result.status === "success" ? 0 : 1;
  }
}
