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
    console.log(`✓ connected · ${r.latencyMs}ms`);
  } else {
    console.log(`✗ connection failed · ${r.error ?? "Unknown error"}`);
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
  process.stderr.write("Error: must provide --file or --json\n");
  process.exit(2);
}

// ── List + Get ────────────────────────────────────────────────

export class ProviderListCommand extends Command {
  static paths = [["provider", "list"]];
  static usage = Command.Usage({ description: "List all providers" });

  output = Option.String("--output", "table");

  async execute(): Promise<number> {
    const list = await listProviders();
    renderProvider(list, this.output);
    return 0;
  }
}

export class ProviderGetCommand extends Command {
  static paths = [["provider", "get"]];
  static usage = Command.Usage({ description: "View provider details (with available models)" });

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
  static usage = Command.Usage({ description: "Create a provider" });

  file = Option.String("--file", { description: "Path to provider.yaml" });
  json = Option.String("--json", { description: "Inline JSON definition" });
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
  static usage = Command.Usage({ description: "Update a provider" });

  id = Option.String();
  file = Option.String("--file", { description: "YAML file for the update" });
  json = Option.String("--json", { description: "Inline JSON" });
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
  static usage = Command.Usage({ description: "Delete a provider" });

  id = Option.String();

  async execute(): Promise<number> {
    await deleteProvider(Number(this.id));
    console.log(`Deleted provider: ${this.id}`);
    return 0;
  }
}

// ── Probe ─────────────────────────────────────────────────────

export class ProviderProbeCommand extends Command {
  static paths = [["provider", "probe"]];
  static usage = Command.Usage({
    description: "Probe provider connectivity (stored provider or custom config)",
  });

  // Mode 1: probe <id> — test a stored provider
  id = Option.String({ required: false });
  // Mode 2: probe --base-url X --api-key Y --protocol Z — test custom config
  baseUrl = Option.String("--base-url", { description: "Custom baseUrl (skip stored provider)" });
  apiKey = Option.String("--api-key", { description: "API Key (custom mode only)" });
  protocol = Option.String("--protocol", { description: "Protocol: anthropic | openai (custom mode only)" });
  authStyle = Option.String("--auth-style", { description: "Auth style, defaults to api_key" });
  output = Option.String("--output", "text");

  async execute(): Promise<number> {
    let result;

    if (this.baseUrl) {
      // Mode 2: custom config probe
      if (!this.apiKey || !this.protocol) {
        process.stderr.write("Error: --base-url mode requires both --api-key and --protocol\n");
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
      process.stderr.write("Error: provide a provider ID or --base-url to probe\n");
      return 2;
    }

    renderProbeResult(result, this.output);
    return result.success ? 0 : 1;
  }
}
