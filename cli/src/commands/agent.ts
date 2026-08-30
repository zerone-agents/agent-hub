import { Command, Option } from "clipanion";
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
  if (d.deployedAt) console.log(`Deployed:  ${d.deployedAt}`);
  if (d.message) console.log(`Message:   ${d.message}`);
}

// ── List + Get ────────────────────────────────────────────────

export class AgentListCommand extends Command {
  static paths = [["agent", "list"]];
  static usage = Command.Usage({ description: "列出所有 agent" });

  desktop = Option.Boolean("--desktop", false, { description: "只看桌面端代理" });
  mobile = Option.Boolean("--mobile", false, { description: "只看手机端代理" });
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
  static usage = Command.Usage({ description: "查看单个 agent 详情" });

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
  static usage = Command.Usage({ description: "从 YAML 定义文件创建 agent" });

  file = Option.String("--file", { description: "agent.yaml 文件路径" });
  output = Option.String("--output", "yaml");
  desktop = Option.Boolean("--desktop", { description: "上架为桌面端代理；--no-desktop 取消" });
  mobile = Option.Boolean("--mobile", { description: "上架为手机端代理；--no-mobile 取消" });
  default = Option.Boolean("--default", {
    description: "设为默认 agent；使用 --no-default 取消默认",
  });

  async execute(): Promise<number> {
    if (!this.file) {
      process.stderr.write("错误：缺少 --file 参数\n");
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
      });
      renderAgent(agent, this.output);
      return 0;
    } catch (error) {
      if (error instanceof AgentYamlError) {
        process.stderr.write(`错误：${error.message}\n`);
        return 2;
      }
      throw error;
    }
  }
}

export class AgentUpdateCommand extends Command {
  static paths = [["agent", "update"]];
  static usage = Command.Usage({ description: "更新 agent 配置" });

  name = Option.String();
  file = Option.String("--file", { description: "更新用的 agent.yaml 文件路径" });
  output = Option.String("--output", "yaml");

  async execute(): Promise<number> {
    if (!this.file) {
      process.stderr.write("错误：缺少 --file 参数\n");
      return 1;
    }
    try {
      const parsed = parseAgentYaml(this.file, this.name);
      const agent = await updateAgent(this.name, {
        config: parsed.config,
        ...(parsed.desktopEnabled !== undefined ? { desktopEnabled: parsed.desktopEnabled } : {}),
        ...(parsed.mobileEnabled !== undefined ? { mobileEnabled: parsed.mobileEnabled } : {}),
        ...(parsed.isDefault !== undefined ? { isDefault: parsed.isDefault } : {}),
      });
      renderAgent(agent, this.output);
      return 0;
    } catch (error) {
      if (error instanceof AgentYamlError) {
        process.stderr.write(`错误：${error.message}\n`);
        return 2;
      }
      throw error;
    }
  }
}

export class AgentDeleteCommand extends Command {
  static paths = [["agent", "delete"]];
  static usage = Command.Usage({ description: "删除 agent" });

  name = Option.String();
  force = Option.Boolean("--force", false, { description: "跳过确认" });

  async execute(): Promise<number> {
    await deleteAgent(this.name);
    console.log(`已删除 agent：${this.name}`);
    return 0;
  }
}

// ── Relations ─────────────────────────────────────────────────

export class AgentSetSubagentsCommand extends Command {
  static paths = [["agent", "set-subagents"]];
  static usage = Command.Usage({ description: "设置 agent 的子 agent 列表" });

  name = Option.String();
  subagents = Option.Rest({ required: 0 });

  async execute(): Promise<number> {
    await setAgentSubagents(this.name, this.subagents);
    console.log(`已设置 ${this.subagents.length} 个子 agent`);
    return 0;
  }
}

export class AgentSetToolsCommand extends Command {
  static paths = [["agent", "set-tools"]];
  static usage = Command.Usage({ description: "设置 agent 的工具列表" });

  name = Option.String();
  tools = Option.Rest({ required: 0 });

  async execute(): Promise<number> {
    await setAgentTools(this.name, this.tools);
    console.log(`已设置 ${this.tools.length} 个工具`);
    return 0;
  }
}

export class AgentSetMcpsCommand extends Command {
  static paths = [["agent", "set-mcps"]];
  static usage = Command.Usage({ description: "设置 agent 绑定的 MCP 列表" });

  name = Option.String();
  mcpNames = Option.Rest({ required: 0 });

  async execute(): Promise<number> {
    await setAgentMcps(this.name, this.mcpNames);
    console.log(`已设置 ${this.mcpNames.length} 个 MCP`);
    return 0;
  }
}

export class AgentSetSkillsCommand extends Command {
  static paths = [["agent", "set-skills"]];
  static usage = Command.Usage({ description: "设置 agent 的技能列表" });

  name = Option.String();
  skills = Option.Rest({ required: 0 });

  async execute(): Promise<number> {
    await setAgentSkills(this.name, this.skills);
    console.log(`已设置 ${this.skills.length} 个技能`);
    return 0;
  }
}

// ── Deploy lifecycle ──────────────────────────────────────────

export class AgentDeployCommand extends Command {
  static paths = [["agent", "deploy"]];
  static usage = Command.Usage({ description: "部署 agent（创建运行时容器）" });

  name = Option.String();
  force = Option.Boolean("--force", false, { description: "强制重新部署" });
  output = Option.String("--output", "text");

  async execute(): Promise<number> {
    const d = await deployAgent(this.name, this.force);
    await renderDeployment(d, this.output);
    return 0;
  }
}

export class AgentUndeployCommand extends Command {
  static paths = [["agent", "undeploy"]];
  static usage = Command.Usage({ description: "下线 agent（归档，保留数据）" });

  name = Option.String();
  purge = Option.Boolean("--purge", false, { description: "彻底删除，不保留数据" });

  async execute(): Promise<number> {
    await undeployAgent(this.name, this.purge);
    console.log(this.purge ? `已彻底删除部署：${this.name}` : `已归档部署：${this.name}`);
    return 0;
  }
}

export class AgentStartCommand extends Command {
  static paths = [["agent", "start"]];
  static usage = Command.Usage({ description: "启动已停止的 agent" });

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
  static usage = Command.Usage({ description: "停止运行中的 agent" });

  name = Option.String();

  async execute(): Promise<number> {
    await stopAgent(this.name);
    console.log(`已停止 agent：${this.name}`);
    return 0;
  }
}

export class AgentStatusCommand extends Command {
  static paths = [["agent", "status"]];
  static usage = Command.Usage({ description: "查看 agent 部署状态" });

  name = Option.String();
  output = Option.String("--output", "text");

  async execute(): Promise<number> {
    const d = await getDeploymentStatus(this.name);
    await renderDeployment(d, this.output);
    return 0;
  }
}
