import { describe, test, expect } from "bun:test";
import { previewText } from "../src/output/preview";

describe("previewText", () => {
  test("parts JSON：只取 text 段并按顺序拼接", () => {
    const c = JSON.stringify([
      { type: "reasoning", reasoning: "想了想" },
      { type: "text", text: "第一行" },
      { type: "tool_use", name: "Bash", input: {} },
      { type: "text", text: "第二行" },
    ]);
    expect(previewText(c, 80)).toBe("第一行 第二行 [tool:Bash]");
  });

  test("非 JSON 的存量行按原文处理", () => {
    expect(previewText("plain legacy text", 80)).toBe("plain legacy text");
  });

  test("截断按字符而非字节，中文不被切碎", () => {
    const out = previewText("中".repeat(50), 10);
    expect(out.startsWith("中".repeat(10))).toBe(true);
    expect(out.length).toBeLessThanOrEqual(11); // 10 + 省略号
  });

  test("空内容不报错", () => {
    expect(previewText("", 80)).toBe("");
    expect(previewText("[]", 80)).toBe("");
  });

  test("非法 JSON 前缀不抛异常", () => {
    expect(previewText("{broken", 80)).toBe("{broken");
  });
});
