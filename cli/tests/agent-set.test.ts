import { describe, test, expect } from "bun:test";
import {
  SetPatchError,
  buildSetUpdate,
  parseSetArg,
  parseSetArgs,
  unreflectedSetKeys,
} from "../src/agent-set";

// issue #202 方案 A：agent update --set 增量补丁的纯函数层（不触网）。
// 合并语义契约：
//   - 值按 YAML 解析（数字/布尔/中文字符串/数组/对象）
//   - 多个 --set 依序合并，同路径后者覆盖前者
//   - 平台标志（desktop/mobile/isDefault/guest 及 *Enabled 别名）进顶层布尔
//   - 其余 key 视为 config 路径，可选 config. 前缀；config 顶层 model → modelId
//   - 只设标志时不返回 config 键（后端 req.Config == nil 即不变更）

// 便捷封装：测试以字符串形式书写 --set 参数，内部走 parseSetArgs →
// buildSetUpdate 的完整管道（与命令层一致，解析只发生一次）。
function build(current: Record<string, unknown> | undefined, ...args: string[]) {
  return buildSetUpdate(current, parseSetArgs(args));
}

describe("parseSetArg", () => {
  test("parses key=value with YAML-typed value", () => {
    expect(parseSetArg("maxTurns=100")).toEqual({ key: "maxTurns", value: 100 });
    expect(parseSetArg("guest=true")).toEqual({ key: "guest", value: true });
    expect(parseSetArg("group=桌面端")).toEqual({ key: "group", value: "桌面端" });
  });

  test("keeps = inside the value", () => {
    expect(parseSetArg("systemPrompt=a=b")).toEqual({ key: "systemPrompt", value: "a=b" });
  });

  test("rejects missing = and empty key", () => {
    expect(() => parseSetArg("maxTurns")).toThrow(SetPatchError);
    expect(() => parseSetArg("=100")).toThrow(SetPatchError);
  });

  test("rejects unparseable YAML value", () => {
    expect(() => parseSetArg("title={zh: unclosed")).toThrow(SetPatchError);
  });
});

describe("buildSetUpdate", () => {
  const current = {
    title: { zh: "代码评审" },
    systemPrompt: "你是评审员",
    maxTurns: 15,
    modelId: "qwen3.8-flash",
  };

  test("--set maxTurns=100 仅变更该字段，其余原样保留", () => {
    const update = build(current, "maxTurns=100");
    expect(update.config).toEqual({ ...current, maxTurns: 100 });
    expect(update).not.toHaveProperty("desktopEnabled");
  });

  test("不修改传入的 currentConfig（深拷贝隔离）", () => {
    build(current, "title.zh=改名");
    expect(current.title.zh).toBe("代码评审");
  });

  test("多个 --set 依序合并，同路径后者覆盖前者", () => {
    const update = build(current, "maxTurns=100", "maxTurns=200", "group=桌面端");
    expect(update.config).toMatchObject({ maxTurns: 200, group: "桌面端" });
  });

  test("config. 前缀被剥掉", () => {
    const update = build(current, "config.group=桌面端");
    expect(update.config).toMatchObject({ group: "桌面端" });
    expect(update.config).not.toHaveProperty("config");
  });

  test("嵌套路径：title.zh 定点更新，title.en 不受影响", () => {
    const withEn = { ...current, title: { zh: "旧", en: "old" } };
    const update = build(withEn, "title.zh=新");
    expect(update.config?.title).toEqual({ zh: "新", en: "old" });
  });

  test("不存在的中间路径自动创建对象", () => {
    const update = build(current, "fieldOverrides.base_url=https://x");
    expect(update.config?.fieldOverrides).toEqual({ base_url: "https://x" });
  });

  test("穿过非对象路径报错", () => {
    expect(() => build(current, "systemPrompt.zh=坏")).toThrow(SetPatchError);
  });

  test("空路径分段报错", () => {
    expect(() => build(current, "title..zh=坏")).toThrow(SetPatchError);
    expect(() => build(current, "config.=坏")).toThrow(SetPatchError);
  });

  test("config 顶层 model 别名归一为 modelId", () => {
    const update = build(current, "model=glm-4.6");
    expect(update.config?.modelId).toBe("glm-4.6");
    expect(update.config).not.toHaveProperty("model");
  });

  test("数组与对象值按 YAML 解析", () => {
    const update = build(current,
      "disallowedTools=[Bash, Edit]",
      "title={zh: 评审, en: review}",
    );
    expect(update.config?.disallowedTools).toEqual(["Bash", "Edit"]);
    expect(update.config?.title).toEqual({ zh: "评审", en: "review" });
  });

  test("平台标志进顶层布尔字段，支持别名", () => {
    expect(build(current, "guest=true")).toEqual({ guestEnabled: true });
    expect(build(current, "guestEnabled=true")).toEqual({ guestEnabled: true });
    expect(build(current, "desktop=false")).toEqual({ desktopEnabled: false });
    expect(build(current, "mobile=true")).toEqual({ mobileEnabled: true });
    expect(build(current, "isDefault=true")).toEqual({ isDefault: true });
    expect(build(current, "default=true")).toEqual({ isDefault: true });
  });

  test("纯标志更新不携带 config 键", () => {
    const update = build(current, "guest=true", "mobile=true");
    expect(update).toEqual({ guestEnabled: true, mobileEnabled: true });
  });

  test("标志值非布尔报错", () => {
    // YAML 1.2 core schema：yes/no 是字符串而非布尔，同样应被拒绝
    expect(() => build(current, "guest=yes")).toThrow(SetPatchError);
    expect(() => build(current, "guest=1")).toThrow(SetPatchError);
    expect(() => build(current, "guest=桌面端")).toThrow(SetPatchError);
  });

  test("标志与 config 路径可组合", () => {
    const update = build(current, "guestEnabled=true", "config.group=桌面端");
    expect(update.guestEnabled).toBe(true);
    expect(update.config).toMatchObject({ group: "桌面端" });
  });

  test("currentConfig 缺失时从空对象起步", () => {
    const update = build(undefined, "maxTurns=100");
    expect(update.config).toEqual({ maxTurns: 100 });
  });
});

describe("unreflectedSetKeys", () => {
  // 后端 update 只识别固定 schema 的 config key；响应比对把「未知 key
  // 静默无效果」变成显式警告。
  test("已生效的 key 不报", () => {
    const pairs = parseSetArgs(["maxTurns=100", "title.zh=新", "guest=true"]);
    const config = { maxTurns: 100, title: { zh: "新", en: "old" } };
    expect(unreflectedSetKeys(config, pairs)).toEqual([]);
  });

  test("响应里缺失或值不符的 key 被列出", () => {
    const pairs = parseSetArgs(["unknownField=bar", "maxTurns=100"]);
    expect(unreflectedSetKeys({ maxTurns: 100 }, pairs)).toEqual(["unknownField"]);
    expect(unreflectedSetKeys({ maxTurns: 15 }, pairs)).toEqual([
      "unknownField",
      "maxTurns",
    ]);
  });

  test("数组与对象按深比较", () => {
    const pairs = parseSetArgs(["disallowedTools=[Bash, Edit]", "title={zh: 评审}"]);
    expect(
      unreflectedSetKeys(
        { disallowedTools: ["Bash", "Edit"], title: { zh: "评审" } },
        pairs,
      ),
    ).toEqual([]);
    expect(unreflectedSetKeys({ disallowedTools: ["Edit", "Bash"] }, pairs)).toEqual([
      "disallowedTools",
      "title",
    ]);
  });

  test("api_key 叶子跳过（读侧掩码无法比对），平台标志不参与", () => {
    const pairs = parseSetArgs(["fieldOverrides.api_key=sk-new", "guest=true"]);
    expect(unreflectedSetKeys({}, pairs)).toEqual([]);
  });

  test("model 别名与 config. 前缀按解析后路径比对", () => {
    const pairs = parseSetArgs(["model=glm-4.6", "config.group=桌面端"]);
    expect(unreflectedSetKeys({ modelId: "glm-4.6", group: "桌面端" }, pairs)).toEqual([]);
    expect(unreflectedSetKeys({}, pairs)).toEqual(["model", "config.group"]);
  });
});
