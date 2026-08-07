import { Command, Option } from "clipanion";
import { readFileSync } from "node:fs";
import { parse } from "yaml";
import { outputJson } from "../output/json";
import { outputYaml } from "../output/yaml";
import { outputTable } from "../output/table";
import {
  listProviders,
  getProvider,
  createProvider,
  updateProvider,
  deleteProvider,
  probeProvider,
  probeProviderConfig,
  type Provider,
} from "../client/provider";

// ── Helpers ───────────────────────────────────────────────────

function providerToRow(p: Provider): Record<string, unknown> {
  return {
    id: p.id,
    name: p.name,
    protocol: p.protocol,
    baseUrl: p.baseUrl,
    models: p.defaultModels?.length ?? 0,
    hasKey: !!p.lockedApiKey,
  };
}

function renderProvider(item: Provider | Provider[], output: string) {
  if (output === "json") {
    outputJson(item);
  } else if (output === "yaml") {
    outputYaml(item);
  } else {
    const arr = Array.isArray(item) ? item : [item];
    outputTable(
      arr.map(providerToRow),
      ["id", "name", "protocol", "baseUrl", "models", "hasKey"],
    );
  }
}

function renderProbeResult(r: { success: boolean; latencyMs: number; error?: string }, output: string) {
  if (output === "json") {
    outputJson(r);
    return;
  }
  if (r.success) {
    console.log(`✓ 连接成功 · ${r.latencyMs}ms`);
  } else {
    console.log(`✗ 连接失败 · ${r.error ?? "未知错误"}`);
  }
}

function loadInput(file?: string, json?: string): Record<string, unknown> {
  if (file) {
    const raw = readFileSync(file, "utf-8");
    return parse(raw) as Record<string, unknown>;
  }
  if (json) {
    return JSON.parse(json);
  }
  process.stderr.write("错误：必须提供 --file 或 --json\n");
  process.exit(2);
}

// ── List + Get ────────────────────────────────────────────────

export class ProviderListCommand extends Command {
  static paths = [["provider", "list"]];
  static usage = Command.Usage({ description: "列出所有 provider" });

  output = Option.String("--output", "table");

  async execute(): Promise<number> {
    const list = await listProviders();
    renderProvider(list, this.output);
    return 0;
  }
}

export class ProviderGetCommand extends Command {
  static paths = [["provider", "get"]];
  static usage = Command.Usage({ description: "查看 provider 详情（含可用模型）" });

  id = Option.String();
  output = Option.String("--output", "yaml");

  async execute(): Promise<number> {
    const p = await getProvider(Number(this.id));
    renderProvider(p, this.output);
    return 0;
  }
}

// ── CRUD ──────────────────────────────────────────────────────

export class ProviderCreateCommand extends Command {
  static paths = [["provider", "create"]];
  static usage = Command.Usage({ description: "创建 provider" });

  file = Option.String("--file", { description: "provider.yaml 文件路径" });
  json = Option.String("--json", { description: "内联 JSON 定义" });
  output = Option.String("--output", "yaml");

  async execute(): Promise<number> {
    const body = loadInput(this.file, this.json);
    const p = await createProvider(body);
    renderProvider(p, this.output);
    return 0;
  }
}

export class ProviderUpdateCommand extends Command {
  static paths = [["provider", "update"]];
  static usage = Command.Usage({ description: "更新 provider" });

  id = Option.String();
  file = Option.String("--file", { description: "更新用的 YAML 文件" });
  json = Option.String("--json", { description: "内联 JSON" });
  output = Option.String("--output", "yaml");

  async execute(): Promise<number> {
    const body = loadInput(this.file, this.json);
    const p = await updateProvider(Number(this.id), body);
    renderProvider(p, this.output);
    return 0;
  }
}

export class ProviderDeleteCommand extends Command {
  static paths = [["provider", "delete"]];
  static usage = Command.Usage({ description: "删除 provider" });

  id = Option.String();

  async execute(): Promise<number> {
    await deleteProvider(Number(this.id));
    console.log(`已删除 provider：${this.id}`);
    return 0;
  }
}

// ── Probe ─────────────────────────────────────────────────────

export class ProviderProbeCommand extends Command {
  static paths = [["provider", "probe"]];
  static usage = Command.Usage({
    description: "探测 provider 连通性（支持探测已保存的 provider 或自定义配置）",
  });

  // Mode 1: probe <id> — test a stored provider
  id = Option.String({ required: false });
  // Mode 2: probe --base-url X --api-key Y --protocol Z — test custom config
  baseUrl = Option.String("--base-url", { description: "自定义 baseUrl（不使用已保存 provider）" });
  apiKey = Option.String("--api-key", { description: "API Key（仅自定义模式）" });
  protocol = Option.String("--protocol", { description: "协议：anthropic | openai（仅自定义模式）" });
  authStyle = Option.String("--auth-style", { description: "认证风格，默认 api_key" });
  output = Option.String("--output", "text");

  async execute(): Promise<number> {
    let result;

    if (this.baseUrl) {
      // Mode 2: custom config probe
      if (!this.apiKey || !this.protocol) {
        process.stderr.write("错误：--base-url 模式必须同时提供 --api-key 和 --protocol\n");
        return 2;
      }
      result = await probeProviderConfig({
        baseUrl: this.baseUrl,
        apiKey: this.apiKey,
        protocol: this.protocol,
        authStyle: this.authStyle ?? "api_key",
      });
    } else if (this.id) {
      // Mode 1: stored provider probe
      result = await probeProvider(Number(this.id));
    } else {
      process.stderr.write("错误：请提供 provider ID 或 --base-url 进行探测\n");
      return 2;
    }

    renderProbeResult(result, this.output);
    return result.success ? 0 : 1;
  }
}
