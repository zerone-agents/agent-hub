import { Command, Option } from "clipanion";
import { SetPatchError, buildSetUpdate, parseSetArgs, unreflectedSetKeys } from "../agent-set";
import { AgentYamlError, parseAgentYaml } from "../agent-yaml";
import { getActiveProfile } from "../config";
import { outputJson } from "../output/json";
import { outputYaml } from "../output/yaml";
import { outputTable } from "../output/table";
import {
  listAgents,
  getAgent,
  createAgent,
  updateAgent,
  deleteAgent,
  setAgentSubagents,
  setAgentTools,
  setAgentSkills,
  setAgentMcps,
  deployAgent,
  undeployAgent,
  startAgent,
  stopAgent,
  getDeploymentStatus,
  type Agent,
  type DeploymentInfo,
} from "../client/agent";

// ── Helpers ───────────────────────────────────────────────────

function agentToRow(a: Agent): Record<string, unknown> {
  return {
    name: a.name,
    title: a.config?.title?.zh ?? "-",
    model: a.config?.modelId ?? a.config?.model ?? "-",
    desktop: a.desktopEnabled ?? false,
    mobile: a.mobileEnabled ?? false,
    default: a.isDefault ?? false,
    guest: a.guestEnabled ?? false,
  };
}

function renderAgentList(agents: Agent[], output: string) {
  if (output === "json") {
    outputJson(agents);
  } else if (output === "yaml") {
    outputYaml(agents);
  } else {
    outputTable(
      agents.map(agentToRow),
      ["name", "title", "model", "desktop", "mobile", "default"],
    );
  }
}

function renderAgent(agent: Agent, output: string) {
  if (output === "table") {
    outputTable([agentToRow(agent)], ["name", "title", "model", "desktop", "mobile", "default"]);
  } else {
    outputYaml(agent);
  }
}

// no-Kong 模式下 runtimeUrl 是 hub 相对路径（/runtime/{org}/{agent}），直接
// 打印对终端用户不可用——人类可读输出需按 profile serverUrl 解析为绝对 URL。
// 拼接语义必须与 API client 自身一致（base.ts 是字符串拼接 `${serverUrl}${path}`，
// API 流量实际打到 {serverUrl}/api/...）：WHATWG new URL 对根相对路径会整体
// 替换 base 的 path（profile https://example.com/hub 会丢掉 /hub），故此处
// 同样用字符串拼接——剥掉 serverUrl 尾部斜杠后直接连接；绝对 URL 原样返回；
// serverUrl 缺失时同样原样返回，避免误报。
export function resolveRuntimeUrl(runtimeUrl: string, serverUrl: string): string {
  if (/^https?:\/\//i.test(runtimeUrl)) return runtimeUrl;
  if (!serverUrl) return runtimeUrl;
  return serverUrl.replace(/\/+$/, "") + runtimeUrl;
}

async function renderDeployment(d: DeploymentInfo, output: string) {
  if (output === "json") {
    // JSON 输出原样镜像 API payload（供脚本消费），相对 runtimeUrl 不做解析。
    outputJson(d);
    return;
  }
  console.log(`Status:    ${d.status}`);
  if (d.health) console.log(`Health:    ${d.health}`);
  if (d.runtimeUrl) {
    const { serverUrl } = await getActiveProfile();
    console.log(`Runtime:   ${resolveRuntimeUrl(d.runtimeUrl, serverUrl)}`);
  }
  if (d.hostPort) console.log(`Port:      ${d.hostPort}`);
  if (d.deployedAt) console.log(`Deployed: ${d.deployedAt}`);
  if (d.message) console.log(`Message:   ${d.message}`);
}

// ── List + Get ────────────────────────────────────────────────

export class AgentListCommand extends Command {
  static paths = [["agent", "list"]];
  static usage = Command.Usage({ description: "List all agents" });

  desktop = Option.Boolean("--desktop", false, { description: "Show desktop agents only" });
  mobile = Option.Boolean("--mobile", false, { description: "Show mobile agents only" });
  output = Option.String("--output", "table");

  async execute(): Promise<number> {
    let agents = await listAgents();
    if (this.desktop) agents = agents.filter((a) => a.desktopEnabled);
    if (this.mobile) agents = agents.filter((a) => a.mobileEnabled);
    renderAgentList(agents, this.output);
    return 0;
  }
}

export class AgentGetCommand extends Command {
  static paths = [["agent", "get"]];
  static usage = Command.Usage({ description: "View a single agent's details" });

  name = Option.String();
  output = Option.String("--output", "yaml");

  async execute(): Promise<number> {
    const agent = await getAgent(this.name);
    renderAgent(agent, this.output);
    return 0;
  }
}

// ── CRUD ──────────────────────────────────────────────────────

export class AgentCreateCommand extends Command {
  static paths = [["agent", "create"]];
  static usage = Command.Usage({ description: "Create an agent from a YAML definition file" });

  file = Option.String("--file", { description: "Path to agent.yaml" });
  output = Option.String("--output", "yaml");
  desktop = Option.Boolean("--desktop", { description: "Publish as desktop agent; --no-desktop to disable" });
  mobile = Option.Boolean("--mobile", { description: "Publish as mobile agent; --no-mobile to disable" });
  default = Option.Boolean("--default", {
    description: "Set as default agent; use --no-default to unset",
  });
  guest = Option.Boolean("--guest", {
    description: "Open agent to guests; use --no-guest to disable",
  });

  async execute(): Promise<number> {
    if (!this.file) {
      process.stderr.write("Error: missing --file parameter\n");
      return 1;
    }
    try {
      const body = parseAgentYaml(this.file);
      const agent = await createAgent({
        name: body.name,
        config: body.config,
        desktopEnabled: this.desktop ?? body.desktopEnabled ?? false,
        mobileEnabled: this.mobile ?? body.mobileEnabled ?? false,
        isDefault: this.default ?? body.isDefault ?? false,
        guestEnabled: this.guest ?? body.guestEnabled ?? false,
      });
      renderAgent(agent, this.output);
      return 0;
    } catch (error) {
      if (error instanceof AgentYamlError) {
        process.stderr.write(`Error: ${error.message}\n`);
        return 2;
      }
      throw error;
    }
  }
}

export class AgentUpdateCommand extends Command {
  static paths = [["agent", "update"]];
  static usage = Command.Usage({ description: "Update agent configuration" });

  name = Option.String();
  file = Option.String("--file", { description: "Path to agent.yaml for the update" });
  set = Option.Array("--set", {
    description: "Patch a single field, e.g. --set maxTurns=100 --set guest=true (repeatable)",
  });
  output = Option.String("--output", "yaml");

  async execute(): Promise<number> {
    if (this.file && this.set && this.set.length > 0) {
      process.stderr.write("Error: --file and --set are mutually exclusive\n");
      return 1;
    }
    if (this.file) {
      try {
        const parsed = parseAgentYaml(this.file, this.name);
        const agent = await updateAgent(this.name, {
          config: parsed.config,
          ...(parsed.desktopEnabled !== undefined ? { desktopEnabled: parsed.desktopEnabled } : {}),
          ...(parsed.mobileEnabled !== undefined ? { mobileEnabled: parsed.mobileEnabled } : {}),
          ...(parsed.isDefault !== undefined ? { isDefault: parsed.isDefault } : {}),
          ...(parsed.guestEnabled !== undefined ? { guestEnabled: parsed.guestEnabled } : {}),
        });
        renderAgent(agent, this.output);
        return 0;
      } catch (error) {
        if (error instanceof AgentYamlError) {
          process.stderr.write(`Error: ${error.message}\n`);
          return 2;
        }
        throw error;
      }
    }
    if (this.set && this.set.length > 0) {
      // 增量补丁（issue #202 方案 A）：先解析全部 --set（格式错误在零
      // 网络调用下拒绝），再 read-modify-write——GET 读回当前 config，
      // 依序合并后整体 PUT。纯标志更新不携带 config 键（后端
      // req.Config == nil 即不变更），规避空对象全量清空风险；GET 读回的
      // 掩码 secret（fieldOverrides.api_key）由后端解包时按掩码形态还原，
      // 不会因 RMW 回写被销毁。
      try {
        const pairs = parseSetArgs(this.set);
        const current = await getAgent(this.name);
        const update = buildSetUpdate(
          (current.config ?? {}) as Record<string, unknown>,
          pairs,
        );
        const agent = await updateAgent(this.name, update);
        // 后端只识别固定 schema 的 config key，未知 key 静默丢弃——把
        // 响应与 --set 逐条比对，无效果时显式警告（exit code 仍为 0，
        // 更新本身已成功）。
        const unreflected = unreflectedSetKeys(
          (agent.config ?? {}) as Record<string, unknown>,
          pairs,
        );
        if (unreflected.length > 0) {
          process.stderr.write(
            `Warning: --set key(s) not reflected in server config (unknown key?): ${unreflected.join(", ")}\n`,
          );
        }
        renderAgent(agent, this.output);
        return 0;
      } catch (error) {
        if (error instanceof SetPatchError) {
          process.stderr.write(`Error: ${error.message}\n`);
          return 2;
        }
        throw error;
      }
    }
    process.stderr.write("Error: missing --file or --set parameter\n");
    return 1;
  }
}

export class AgentDeleteCommand extends Command {
  static paths = [["agent", "delete"]];
  static usage = Command.Usage({ description: "Delete an agent" });

  name = Option.String();
  force = Option.Boolean("--force", false, { description: "Skip confirmation" });

  async execute(): Promise<number> {
    await deleteAgent(this.name);
    console.log(`Deleted agent: ${this.name}`);
    return 0;
  }
}

// ── Relations ─────────────────────────────────────────────────

export class AgentSetSubagentsCommand extends Command {
  static paths = [["agent", "set-subagents"]];
  static usage = Command.Usage({ description: "Set agent subagent list" });

  name = Option.String();
  subagents = Option.Rest({ required: 0 });

  async execute(): Promise<number> {
    await setAgentSubagents(this.name, this.subagents);
    console.log(`Set ${this.subagents.length} subagents`);
    return 0;
  }
}

export class AgentSetToolsCommand extends Command {
  static paths = [["agent", "set-tools"]];
  static usage = Command.Usage({ description: "Set agent tool list" });

  name = Option.String();
  tools = Option.Rest({ required: 0 });

  async execute(): Promise<number> {
    await setAgentTools(this.name, this.tools);
    console.log(`Set ${this.tools.length} tools`);
    return 0;
  }
}

export class AgentSetMcpsCommand extends Command {
  static paths = [["agent", "set-mcps"]];
  static usage = Command.Usage({ description: "Set agent MCP list" });

  name = Option.String();
  mcpNames = Option.Rest({ required: 0 });

  async execute(): Promise<number> {
    await setAgentMcps(this.name, this.mcpNames);
    console.log(`Set ${this.mcpNames.length} MCPs`);
    return 0;
  }
}

export class AgentSetSkillsCommand extends Command {
  static paths = [["agent", "set-skills"]];
  static usage = Command.Usage({ description: "Set agent skill list" });

  name = Option.String();
  skills = Option.Rest({ required: 0 });

  async execute(): Promise<number> {
    await setAgentSkills(this.name, this.skills);
    console.log(`Set ${this.skills.length} skills`);
    return 0;
  }
}

// ── Deploy lifecycle ──────────────────────────────────────────

export class AgentDeployCommand extends Command {
  static paths = [["agent", "deploy"]];
  static usage = Command.Usage({ description: "Deploy agent (create runtime container)" });

  name = Option.String();
  force = Option.Boolean("--force", false, { description: "Force redeploy" });
  output = Option.String("--output", "text");

  async execute(): Promise<number> {
    const d = await deployAgent(this.name, this.force);
    await renderDeployment(d, this.output);
    return 0;
  }
}

export class AgentUndeployCommand extends Command {
  static paths = [["agent", "undeploy"]];
  static usage = Command.Usage({ description: "Undeploy agent (archive, keep data)" });

  name = Option.String();
  purge = Option.Boolean("--purge", false, { description: "Permanently delete, no data retention" });

  async execute(): Promise<number> {
    await undeployAgent(this.name, this.purge);
    console.log(this.purge ? `Permanently deleted deployment: ${this.name}` : `Archived deployment: ${this.name}`);
    return 0;
  }
}

export class AgentStartCommand extends Command {
  static paths = [["agent", "start"]];
  static usage = Command.Usage({ description: "Start a stopped agent" });

  name = Option.String();
  output = Option.String("--output", "text");

  async execute(): Promise<number> {
    const d = await startAgent(this.name);
    await renderDeployment(d, this.output);
    return 0;
  }
}

export class AgentStopCommand extends Command {
  static paths = [["agent", "stop"]];
  static usage = Command.Usage({ description: "Stop a running agent" });

  name = Option.String();

  async execute(): Promise<number> {
    await stopAgent(this.name);
    console.log(`Stopped agent: ${this.name}`);
    return 0;
  }
}

export class AgentStatusCommand extends Command {
  static paths = [["agent", "status"]];
  static usage = Command.Usage({ description: "View agent deployment status" });

  name = Option.String();
  output = Option.String("--output", "text");

  async execute(): Promise<number> {
    const d = await getDeploymentStatus(this.name);
    await renderDeployment(d, this.output);
    return 0;
  }
}
