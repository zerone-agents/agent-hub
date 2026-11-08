import { useState } from "react";
import type { TableUploadConfig } from "./tableParserConfig";
import { afterEach, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ConfigProvider, Form, theme } from "antd";
import i18next from "@/i18n";
import { normalizeDocument } from "@/api/knowledge";
import { DatasetTableSettings, FileTableSettings } from "./TableColumnSettings";
import GraphExecutionSettings from "./GraphExecutionSettings";
import KnowledgeDocumentUpload from "./KnowledgeDocumentUpload";
import {
  datasetToFormValues,
  DatasetFields,
  formValuesToInput,
} from "./KnowledgeForm";
import { normalizeDataset } from "@/api/knowledge";
afterEach(async () => {
  await i18next.changeLanguage("zh");
});
async function choose(
  user: ReturnType<typeof userEvent.setup>,
  name: string,
  option: string,
) {
  await user.click(screen.getByRole("combobox", { name }));
  const elements = screen.getAllByText(option, {
    selector: ".ant-select-item-option-content",
  });
  await user.click(elements[elements.length - 1]);
}
it("uploads each file with its own config and keeps inherited files free of override fields", async () => {
  const upload = vi.fn(async (file: File) =>
    normalizeDocument({ id: file.name, name: file.name }),
  );
  const parse = vi
    .fn()
    .mockRejectedValueOnce(new Error("parse unavailable"))
    .mockResolvedValue(true);
  const user = userEvent.setup();
  render(
    <ConfigProvider>
      <KnowledgeDocumentUpload
        datasetParserId="table"
        onUpload={upload}
        onParse={parse}
        onClose={vi.fn()}
      />
    </ConfigProvider>,
  );
  const files = [
    new File(["a.b,x\n1,2"], "a.csv"),
    new File(["a.b,x\n3,4"], "b.csv"),
  ];
  await user.upload(
    document.querySelector('input[type="file"]') as HTMLInputElement,
    files,
  );
  await waitFor(() =>
    expect(screen.queryByText("正在读取首条表头记录…")).not.toBeInTheDocument(),
  );
  await choose(user, "a.csv: 列配置模式", "手动指定用途");
  await choose(user, "a.csv: a.b", "仅元数据");
  await user.click(screen.getByRole("button", { name: "开始上传" }));
  await screen.findByText("parse unavailable");
  expect(upload).toHaveBeenNthCalledWith(1, files[0], {
    table_column_mode: "manual",
    table_column_roles: { "a.b": "metadata", x: "both" },
  });
  expect(upload).toHaveBeenNthCalledWith(2, files[1]);
  expect(
    screen.getByRole("combobox", { name: "a.csv: 列配置模式" }),
  ).toBeDisabled();
  await user.click(screen.getAllByRole("button", { name: "重试失败项" })[0]);
  await waitFor(() => {
    expect(screen.getAllByText("解析已受理，等待 worker")).toHaveLength(2);
  });
  expect(upload).toHaveBeenCalledTimes(2);
});
it("leaves unknown or duplicate headers inheritable and supports auto without private markers", async () => {
  const onChange = vi.fn();
  const user = userEvent.setup();
  render(
    <ConfigProvider>
      <FileTableSettings
        file={new File(["a,a\n1,2"], "duplicate.csv")}
        disabled={false}
        onChange={onChange}
      />
    </ConfigProvider>,
  );
  await screen.findByText(
    "无法预览明确表头。可继承知识库配置、使用自动模式，或手动输入已知列名。",
  );
  await user.click(
    screen.getByRole("combobox", { name: "duplicate.csv: 列配置模式" }),
  );
  expect(
    screen.queryByText("手动指定用途", {
      selector: ".ant-select-item-option-content",
    }),
  ).toBeInTheDocument();
  await user.click(
    screen.getByText("自动识别", {
      selector: ".ant-select-item-option-content",
    }),
  );
  expect(onChange).toHaveBeenLastCalledWith({
    table_column_mode: "auto",
    table_column_roles: {},
  });
});
it("edits dataset roles with raw dotted keys and retains names outside the displayed list", async () => {
  const onChange = vi.fn();
  const user = userEvent.setup();
  render(
    <ConfigProvider>
      <DatasetTableSettings
        value={{
          table_column_mode: "manual",
          table_column_names: [" a.b "],
          table_column_roles: { hidden: "metadata", " a.b ": "vectorize" },
        }}
        onChange={onChange}
      />
    </ConfigProvider>,
  );
  await choose(user, "a.b", "索引与元数据");
  expect(onChange).toHaveBeenLastCalledWith({
    table_column_mode: "manual",
    table_column_names: [" a.b "],
    table_column_roles: { hidden: "metadata", " a.b ": "both" },
  });
});
it("shows NER and graph execution fields in the dataset form", async () => {
  const original = normalizeDataset({
    id: "kb",
    name: "Table",
    parser_id: "table",
    parser_config: {
      graphrag: { method: "ner" },
      table_column_mode: "manual",
      table_column_names: ["x.y"],
    },
  });
  let submitted: ReturnType<typeof formValuesToInput> | undefined;
  const user = userEvent.setup();
  function Harness() {
    return (
      <Form
        initialValues={datasetToFormValues(original)}
        onFinish={(values) => {
          submitted = formValuesToInput(
            { ...datasetToFormValues(original), ...values },
            { original },
          );
        }}
      >
        <DatasetFields />
        <button type="submit">Save fixture</button>
      </Form>
    );
  }
  render(
    <ConfigProvider>
      <Harness />
    </ConfigProvider>,
  );
  expect(await screen.findByText("命名实体识别（NER）")).toBeInTheDocument();
  expect(screen.getByRole("combobox", { name: "x.y" })).toBeInTheDocument();
  await user.click(screen.getByText("GraphRAG 配置"));
  await user.click(screen.getByText("图谱执行限制"));
  await user.type(
    screen.getByRole("spinbutton", { name: "合并超时（秒）" }),
    "0",
  );
  await user.click(screen.getByRole("button", { name: "Save fixture" }));
  await waitFor(() => {
    expect(submitted).toEqual({
      parser_config: { graphrag: { merge_timeout_seconds: 0 } },
    });
  });
});
it("keeps graph zero budgets and table role controls usable in English dark theme", async () => {
  await i18next.changeLanguage("en");
  const onChange = vi.fn();
  const user = userEvent.setup();
  render(
    <ConfigProvider theme={{ algorithm: theme.darkAlgorithm }}>
      <GraphExecutionSettings
        value={{ merge_timeout_seconds: 0 }}
        onChange={onChange}
      />
    </ConfigProvider>,
  );
  expect(
    screen.getByRole("spinbutton", { name: "Merge timeout (seconds)" }),
  ).toHaveValue("0");
  await user.type(
    screen.getByRole("spinbutton", { name: "Default attempt count" }),
    "3",
  );
  expect(onChange).toHaveBeenLastCalledWith({
    merge_timeout_seconds: 0,
    retry_attempts: 3,
  });
});

function ControlledFile({
  file,
  onChange,
  disabled = false,
}: {
  file: File;
  onChange: (value?: TableUploadConfig) => void;
  disabled?: boolean;
}) {
  const [value, setValue] = useState<TableUploadConfig>();
  return (
    <FileTableSettings
      file={file}
      value={value}
      disabled={disabled}
      onChange={(next) => {
        setValue(next);
        onChange(next);
      }}
    />
  );
}
it.each(["known.xlsx", "known.xls", "known.txt", "ambiguous.csv"])(
  "supports explicit manual columns without preview for %s",
  async (name) => {
    const changed = vi.fn();
    const user = userEvent.setup();
    render(
      <ConfigProvider>
        <ControlledFile
          file={new File(["a,a\n1,2"], name)}
          onChange={changed}
        />
      </ConfigProvider>,
    );
    await screen.findByText(
      "无法预览明确表头。可继承知识库配置、使用自动模式，或手动输入已知列名。",
    );
    await choose(user, `${name}: 列配置模式`, "手动指定用途");
    expect(changed).toHaveBeenLastCalledWith({
      table_column_mode: "manual",
      table_column_roles: {},
    });
    changed.mockClear();
    await user.type(
      screen.getByRole("textbox", { name: `${name}: 原始列名` }),
      " a.b ",
    );
    expect(changed).not.toHaveBeenCalled();
    expect(
      screen.getByText("当前输入须点击“添加列”后才会纳入本文件配置。"),
    ).toBeInTheDocument();
    await user.keyboard("{Enter}");
    expect(changed).not.toHaveBeenCalled();
    await user.click(screen.getByRole("button", { name: "添加列" }));
    await choose(user, `${name}: a.b`, "仅元数据");
    expect(changed).toHaveBeenLastCalledWith({
      table_column_mode: "manual",
      table_column_roles: { " a.b ": "metadata" },
    });
    await user.click(
      screen.getByRole("button", { name: `${name}: 移除列: a.b` }),
    );
    expect(changed).toHaveBeenLastCalledWith({
      table_column_mode: "manual",
      table_column_roles: {},
    });
  },
);
it("rejects blank and exact duplicate names while preserving distinct whitespace and prototype-like keys", async () => {
  const changed = vi.fn();
  const user = userEvent.setup();
  render(
    <ConfigProvider>
      <ControlledFile
        file={new File(["binary"], "known.xlsx")}
        onChange={changed}
      />
    </ConfigProvider>,
  );
  await screen.findByText(
    "无法预览明确表头。可继承知识库配置、使用自动模式，或手动输入已知列名。",
  );
  await choose(user, "known.xlsx: 列配置模式", "手动指定用途");
  const input = screen.getByRole("textbox", { name: "known.xlsx: 原始列名" });
  const add = screen.getByRole("button", { name: "添加列" });
  changed.mockClear();
  await user.click(add);
  expect(screen.getByRole("alert")).toHaveTextContent(
    "列名不能为空或仅包含空白。",
  );
  await user.type(input, "   ");
  await user.click(add);
  expect(changed).not.toHaveBeenCalled();
  await user.clear(input);
  await user.type(input, " a.b ");
  await user.click(add);
  const count = changed.mock.calls.length;
  await user.type(input, " a.b ");
  await user.click(add);
  expect(screen.getByRole("alert")).toHaveTextContent(
    "该原始列名已添加，请勿重复添加。",
  );
  expect(changed).toHaveBeenCalledTimes(count);
  await user.clear(input);
  await user.type(input, "a.b");
  await user.click(add);
  await user.type(input, "__proto__");
  await user.click(add);
  expect(changed).toHaveBeenLastCalledWith({
    table_column_mode: "manual",
    table_column_roles: {
      " a.b ": "both",
      "a.b": "both",
      ["__proto__"]: "both",
    },
  });
  await choose(user, "known.xlsx: 列配置模式", "自动识别");
  await choose(user, "known.xlsx: 列配置模式", "手动指定用途");
  expect(changed).toHaveBeenLastCalledWith({
    table_column_mode: "manual",
    table_column_roles: {
      " a.b ": "both",
      "a.b": "both",
      ["__proto__"]: "both",
    },
  });
});
it("uploads XLSX manual roles only after explicit addition and locks drafts, roles and removal after upload", async () => {
  const user = userEvent.setup();
  const upload = vi.fn(async () =>
    normalizeDocument({ id: "excel1", name: "known.xlsx" }),
  );
  const parse = vi.fn().mockRejectedValueOnce(new Error("retry parsing"));
  render(
    <ConfigProvider>
      <KnowledgeDocumentUpload
        datasetParserId="table"
        onUpload={upload}
        onParse={parse}
        onClose={vi.fn()}
      />
    </ConfigProvider>,
  );
  const file = new File(["binary"], "known.xlsx");
  await user.upload(
    document.querySelector('input[type="file"]') as HTMLInputElement,
    file,
  );
  await screen.findByText(
    "无法预览明确表头。可继承知识库配置、使用自动模式，或手动输入已知列名。",
  );
  await choose(user, "known.xlsx: 列配置模式", "手动指定用途");
  const input = screen.getByRole("textbox", { name: "known.xlsx: 原始列名" });
  await user.type(input, " a.b ");
  await user.click(screen.getByRole("button", { name: "添加列" }));
  await choose(user, "known.xlsx: a.b", "仅元数据");
  await user.type(input, "pending-not-added");
  await user.click(screen.getByRole("button", { name: "开始上传" }));
  await screen.findByText("retry parsing");
  expect(upload).toHaveBeenCalledExactlyOnceWith(file, {
    table_column_mode: "manual",
    table_column_roles: { " a.b ": "metadata" },
  });
  expect(input).toBeDisabled();
  expect(screen.getByRole("button", { name: "添加列" })).toBeDisabled();
  expect(
    screen.getByRole("combobox", { name: "known.xlsx: a.b" }),
  ).toBeDisabled();
  expect(
    screen.getByRole("button", { name: "known.xlsx: 移除列: a.b" }),
  ).toBeDisabled();
});
it("can upload an empty manual map without applying an unadded column draft", async () => {
  const user = userEvent.setup();
  const upload = vi.fn(async () => normalizeDocument({ id: "empty1" }));
  render(
    <ConfigProvider>
      <KnowledgeDocumentUpload
        datasetParserId="table"
        onUpload={upload}
        onParse={vi.fn().mockResolvedValue(true)}
        onClose={vi.fn()}
      />
    </ConfigProvider>,
  );
  const file = new File(["binary"], "empty.xlsx");
  await user.upload(
    document.querySelector('input[type="file"]') as HTMLInputElement,
    file,
  );
  await screen.findByText(
    "无法预览明确表头。可继承知识库配置、使用自动模式，或手动输入已知列名。",
  );
  await choose(user, "empty.xlsx: 列配置模式", "手动指定用途");
  await user.type(
    screen.getByRole("textbox", { name: "empty.xlsx: 原始列名" }),
    "draft",
  );
  await user.click(screen.getByRole("button", { name: "开始上传" }));
  await screen.findByText("解析已受理，等待 worker");
  expect(upload).toHaveBeenCalledExactlyOnceWith(file, {
    table_column_mode: "manual",
    table_column_roles: {},
  });
});
