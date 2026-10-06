import { useState } from "react";
import { expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ConfigProvider } from "antd";
import MetadataValuesEditor from "./MetadataValuesEditor";
import MetadataTemplateEditor from "./MetadataTemplateEditor";
import i18next from "@/i18n";

it("keeps zero, false, null, nested values and explicit empty lists during an unrelated edit", async () => {
  const change = vi.fn();
  function Editor() {
    const [value, setValue] = useState(
      JSON.stringify({
        author: "Alice",
        revision: 0,
        flag: false,
        optional: null,
        empty: [],
        nested: { retain: [1, false] },
      }),
    );
    return (
      <MetadataValuesEditor
        value={value}
        onChange={(value) => {
          setValue(value);
          change(JSON.parse(value));
        }}
      />
    );
  }
  const user = userEvent.setup();
  render(
    <ConfigProvider>
      <Editor />
    </ConfigProvider>,
  );
  expect(change).not.toHaveBeenCalled();
  await user.type(screen.getByRole("textbox", { name: "新值: author" }), "!");
  expect(change).toHaveBeenLastCalledWith({
    author: "Alice!",
    revision: 0,
    flag: false,
    optional: null,
    empty: [],
    nested: { retain: [1, false] },
  });
  await user.click(screen.getByRole("button", { name: "删除: optional" }));
  expect(change.mock.lastCall?.[0]).not.toHaveProperty("optional");
});
it("edits JSON Schema fields visually without dropping schema, required or unknown properties", async () => {
  const change = vi.fn();
  function Editor() {
    const [value, setValue] = useState(
      JSON.stringify({
        type: "object",
        custom: { keep: true },
        properties: {
          author: { type: "string", description: "Person", future: false },
        },
        required: ["author"],
      }),
    );
    return (
      <MetadataTemplateEditor
        value={value}
        onChange={(value) => {
          setValue(value);
          change(JSON.parse(value));
        }}
      />
    );
  }
  const user = userEvent.setup();
  render(
    <ConfigProvider>
      <Editor />
    </ConfigProvider>,
  );
  await user.type(
    screen.getByRole("textbox", { name: "字段 1 的名称" }),
    "_name",
  );
  expect(change).toHaveBeenLastCalledWith({
    type: "object",
    custom: { keep: true },
    properties: {
      author_name: { type: "string", description: "Person", future: false },
    },
    required: ["author_name"],
  });
  await user.click(screen.getByRole("button", { name: "删除字段 1" }));
  expect(change).toHaveBeenLastCalledWith({
    type: "object",
    custom: { keep: true },
    properties: {},
    required: [],
  });
});
it("loads the dedicated English messages and retains incomplete advanced JSON", async () => {
  await i18next.changeLanguage("en");
  const change = vi.fn();
  const user = userEvent.setup();
  render(
    <ConfigProvider>
      <MetadataValuesEditor value="{}" onChange={change} />
    </ConfigProvider>,
  );
  await user.click(screen.getByRole("tab", { name: "Advanced JSON" }));
  await user.type(screen.getByRole("textbox", { name: "Advanced JSON" }), "x");
  await waitFor(() => {
    expect(change).toHaveBeenCalledWith("{}x");
  });
  await i18next.changeLanguage("zh");
});
it("edits numeric and boolean list values without changing their types or losing unknown fields", async () => {
  const change = vi.fn();
  function Editor() {
    const [value, setValue] = useState(
      JSON.stringify({ values: [0, false], nested: { keep: true } }),
    );
    return (
      <MetadataValuesEditor
        value={value}
        onChange={(value) => {
          setValue(value);
          change(JSON.parse(value));
        }}
      />
    );
  }
  const user = userEvent.setup();
  render(
    <ConfigProvider>
      <Editor />
    </ConfigProvider>,
  );
  const number = screen.getByRole("spinbutton", { name: "新值: values: 1" });
  await user.clear(number);
  await user.type(number, "2");
  expect(change).toHaveBeenLastCalledWith({
    values: [2, false],
    nested: { keep: true },
  });
  await user.click(screen.getByRole("switch", { name: "新值: values: 2" }));
  expect(change).toHaveBeenLastCalledWith({
    values: [2, true],
    nested: { keep: true },
  });
});
it("keeps numeric list schema options numeric through a visual candidate edit", async () => {
  const change = vi.fn();
  function Editor() {
    const [value, setValue] = useState(
      JSON.stringify({
        type: "object",
        properties: {
          n: {
            type: "array",
            items: { type: "number", enum: [0, 2], minimum: 0 },
          },
        },
      }),
    );
    return (
      <MetadataTemplateEditor
        value={value}
        onChange={(value) => {
          setValue(value);
          change(JSON.parse(value));
        }}
      />
    );
  }
  const user = userEvent.setup();
  render(
    <ConfigProvider>
      <Editor />
    </ConfigProvider>,
  );
  const input = screen.getByRole("combobox", { name: "字段 1 的可选值" });
  await user.click(input);
  await user.type(input, "3");
  await user.click(
    await screen.findByText("3", {
      selector: ".ant-select-item-option-content",
    }),
  );
  expect(change.mock.lastCall?.[0].properties.n.items).toEqual({
    type: "number",
    enum: [0, 2, 3],
    minimum: 0,
  });
});
it("preserves a committed boolean false item schema instead of replacing it through enum editing", () => {
  render(
    <ConfigProvider>
      <MetadataTemplateEditor
        value={JSON.stringify({
          properties: { values: { type: "array", items: false, custom: 0 } },
        })}
      />
    </ConfigProvider>,
  );
  expect(
    screen.getByRole("combobox", { name: "字段 1 的可选值" }),
  ).toBeDisabled();
  expect(screen.getByText(/此列表模板不允许任何项/)).toBeInTheDocument();
});
