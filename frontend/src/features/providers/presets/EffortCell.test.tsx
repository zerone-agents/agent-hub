import { useState } from "react";
import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ConfigProvider } from "antd";
import { antdTheme } from "@/lib/antd-theme";
import EffortCell from "./EffortCell";

function Harness({ initial }: { initial?: string[] }) {
  const [value, setValue] = useState<string[] | undefined>(initial);
  return (
    <ConfigProvider theme={antdTheme}>
      <EffortCell value={value} onChange={setValue} />
    </ConfigProvider>
  );
}

function activate(user: ReturnType<typeof userEvent.setup>) {
  // Click the idle readOnly input to enter active (editing) mode.
  const idle = screen.getByRole("textbox");
  return user.click(idle);
}

describe("EffortCell", () => {
  it("shows idle label '不涉及' when no efforts configured", () => {
    render(<Harness />);
    expect(screen.getByRole("textbox", { name: "不涉及" })).toHaveValue(
      "不涉及",
    );
  });

  it("shows '已配置 N 档' in idle state", () => {
    render(<Harness initial={["low", "high"]} />);
    expect(screen.getByRole("textbox", { name: "已配置 2 档" })).toHaveValue(
      "已配置 2 档",
    );
  });

  it("adds an effort via the 添加 button", async () => {
    render(<Harness />);
    const user = userEvent.setup();
    await activate(user);

    const input = screen.getByRole("textbox");
    await user.type(input, "low");
    await user.click(screen.getByLabelText("添加 effort"));

    expect(screen.getByText("low")).toBeInTheDocument();
    expect(input).toHaveValue("");
  });

  it("adds an effort via Enter", async () => {
    render(<Harness />);
    const user = userEvent.setup();
    await activate(user);

    await user.type(screen.getByRole("textbox"), "high{Enter}");
    expect(screen.getByText("high")).toBeInTheDocument();
  });

  it("trims, ignores empty input, and dedupes", async () => {
    render(<Harness initial={["low"]} />);
    const user = userEvent.setup();
    await activate(user);

    const input = screen.getByRole("textbox");
    // 空值忽略：输入全空格时添加按钮隐藏
    await user.type(input, "   ");
    const addBtn = screen.getByLabelText("添加 effort");
    expect(addBtn).toHaveStyle("visibility: hidden");
    expect(screen.getAllByRole("button", { name: /^删除 / })).toHaveLength(1);

    // trim 后重复，静默忽略
    await user.clear(input);
    await user.type(input, "  low  ");
    await user.click(addBtn);

    expect(screen.getAllByText("low")).toHaveLength(1);
    expect(screen.getAllByRole("button", { name: /^删除 / })).toHaveLength(1);
  });

  it("removes an effort via its × button", async () => {
    render(<Harness initial={["low", "high"]} />);
    const user = userEvent.setup();
    await activate(user);

    await user.click(screen.getByRole("button", { name: "删除 low" }));
    expect(screen.queryByText("low")).not.toBeInTheDocument();
    expect(screen.getByText("high")).toBeInTheDocument();
  });
});
