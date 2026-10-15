import { Command, Option } from "clipanion";
import { validateOutput } from "../output/validate";
import { outputJson } from "../output/json";
import { outputTable } from "../output/table";
import { outputYaml } from "../output/yaml";
import { previewText } from "../output/preview";
import {
  listChatSessions,
  getChatSession,
  listChatMessages,
  type ChatPage,
  type ChatSession,
  type ChatMessage,
} from "../client/chat";

function shortId(id: string): string {
  return id.length > 8 ? id.slice(0, 8) : id;
}

function sessionRow(s: ChatSession): Record<string, unknown> {
  return {
    id: shortId(s.id),
    title: s.title || "-",
    agent: s.agent_id || "-",
    user: s.display_name || s.user_name || shortId(s.user_id),
    // hidden 会话不会被本端点过滤（与网页一致），因此列出来，避免"多出来的行"无解释
    hidden: s.hidden ? "yes" : "-",
    updated: s.updated_at,
  };
}

function messageRow(m: ChatMessage, width: number): Record<string, unknown> {
  return { role: m.role, created: m.created_at, content: previewText(m.content, width) };
}

// 分页提示走 stderr：stdout 保持可被管道消费（json/yaml 的机器可读性不受影响）。
function writePager(page: ChatPage<unknown>): void {
  process.stderr.write(`(第 ${page.page}/${Math.max(page.total_pages, 1)} 页 · 共 ${page.total} 条 · 每页 ${page.page_size})\n`);
}

function localIso(value: string | undefined): string | null {
  if (!value) return null;
  const t = Date.parse(value);
  if (Number.isNaN(t)) {
    process.stderr.write(`Error: invalid RFC3339 timestamp: ${value}\n`);
    return null;
  }
  return new Date(t).toISOString();
}

export class ChatListCommand extends Command {
  static paths = [["chat", "list"]];
  static usage = Command.Usage({
    description: "List chat sessions with server-side filters",
    details: [
      "--from/--to 作用于 updated_at（最后活动时间），不是创建时间。",
      "--text 是跨表扫描消息正文，可能较慢：非 member 角色需搭配 --agent/--user/--from/--to 之一，且至少 2 个字符。",
    ].join(" "),
  });

  agent = Option.String("--agent", { required: false });
  user = Option.String("--user", { required: false });
  keyword = Option.String("--keyword", { required: false });
  text = Option.String("--text", { required: false });
  from = Option.String("--from", { required: false });
  to = Option.String("--to", { required: false });
  page = Option.String("--page", { required: false });
  pageSize = Option.String("--page-size", { required: false });
  output = Option.String("--output", "table");

  async execute(): Promise<number> {
    const invalid = validateOutput(this.output);
    if (invalid !== null) return invalid;

    // 本地只做与角色无关、且能确定非法的判断：时间顺序与 --text 最小长度。
    // 「是否需搭配收窄条件」取决于调用者角色，只有服务端知道，故不本地拦截。
    if (this.text !== undefined && Array.from(this.text.trim()).length < 2) {
      process.stderr.write("Error: --text needs at least 2 characters\n");
      return 2;
    }
    const from = localIso(this.from);
    const to = localIso(this.to);
    if (this.from && from === null) return 2;
    if (this.to && to === null) return 2;
    if (from && to && Date.parse(from) > Date.parse(to)) {
      process.stderr.write("Error: --from must not be later than --to\n");
      return 2;
    }

    const page = this.page === undefined ? undefined : Number(this.page);
    const pageSize = this.pageSize === undefined ? undefined : Number(this.pageSize);
    if (page !== undefined && (!Number.isInteger(page) || page < 1)) {
      process.stderr.write("Error: --page must be a positive integer\n");
      return 2;
    }
    if (pageSize !== undefined && (!Number.isInteger(pageSize) || pageSize < 1)) {
      process.stderr.write("Error: --page-size must be a positive integer\n");
      return 2;
    }

    const res = await listChatSessions({
      agentId: this.agent, user: this.user, keyword: this.keyword, messageContains: this.text,
      from: from ?? undefined, to: to ?? undefined, page, pageSize,
    });
    if (this.output === "json") {
      outputJson(res.items ?? [], { page: res.page, pageSize: res.page_size, total: res.total, totalPages: res.total_pages });
      return 0;
    }
    const rows = (res.items ?? []).map((s) => sessionRow(s));
    if (this.output === "yaml") {
      outputYaml(res.items ?? []);
    } else {
      outputTable(rows, ["id", "title", "agent", "user", "hidden", "updated"]);
    }
    writePager(res);
    return 0;
  }
}

export class ChatGetCommand extends Command {
  static paths = [["chat", "get"]];
  static usage = Command.Usage({ description: "View one chat session" });

  id = Option.String();
  output = Option.String("--output", "yaml");

  async execute(): Promise<number> {
    const invalid = validateOutput(this.output);
    if (invalid !== null) return invalid;
    const s = await getChatSession(this.id);
    if (this.output === "json") {
      outputJson(s);
      return 0;
    }
    if (this.output === "yaml") {
      outputYaml(s);
      return 0;
    }
    outputTable([sessionRow(s)], ["id", "title", "agent", "user", "hidden", "updated"]);
    return 0;
  }
}

export class ChatMessagesCommand extends Command {
  static paths = [["chat", "messages"]];
  static usage = Command.Usage({ description: "List messages of one chat session (paginated)" });

  id = Option.String();
  page = Option.String("--page", { required: false });
  pageSize = Option.String("--page-size", { required: false });
  output = Option.String("--output", "table");
  full = Option.Boolean("--full", false, { description: "Do not truncate message previews" });

  async execute(): Promise<number> {
    const invalid = validateOutput(this.output);
    if (invalid !== null) return invalid;
    const page = this.page === undefined ? undefined : Number(this.page);
    const pageSize = this.pageSize === undefined ? undefined : Number(this.pageSize);
    if (page !== undefined && (!Number.isInteger(page) || page < 1)) {
      process.stderr.write("Error: --page must be a positive integer\n");
      return 2;
    }
    if (pageSize !== undefined && (!Number.isInteger(pageSize) || pageSize < 1)) {
      process.stderr.write("Error: --page-size must be a positive integer\n");
      return 2;
    }
    const res = await listChatMessages(this.id, { page, pageSize });
    if (this.output === "json") {
      outputJson(res.items ?? [], { page: res.page, pageSize: res.page_size, total: res.total, totalPages: res.total_pages });
      return 0;
    }
    if (this.output === "yaml") {
      outputYaml(res.items ?? []);
      writePager(res);
      return 0;
    }
    const width = this.full ? 4000 : 80;
    outputTable((res.items ?? []).map((m) => messageRow(m, width)), ["role", "created", "content"]);
    writePager(res);
    return 0;
  }
}