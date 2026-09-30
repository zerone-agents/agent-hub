import { describe, test, expect } from "bun:test";
import { SetPatchError, buildSetUpdate, parseSetArg, parseSetArgs } from "../src/agent-set";

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
