import { beforeEach, it, expect, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ConfigProvider } from "antd";
import { MemoryRouter, Route, Routes } from "react-router";
import { normalizeDocument } from "@/api/knowledge";
import {
  KnowledgeDocumentCreate,
  KnowledgeDocumentEditor,
} from "./KnowledgeDocumentEditor";
const h = vi.hoisted(() => ({
  get: vi.fn(),
  update: vi.fn(),
  metadata: vi.fn(),
  create: vi.fn(),
  saved: vi.fn(),
  close: vi.fn(),
}));
vi.mock("@/api/knowledge", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/api/knowledge")>();
  return {
    ...actual,
    knowledgeApi: {
      ...actual.knowledgeApi,
      documents: {
        ...actual.knowledgeApi.documents,
        get: h.get,
        update: h.update,
      },
    },
  };
});
vi.mock("@/api/knowledgeManagement", async (importOriginal) => {
  const actual =
    await importOriginal<typeof import("@/api/knowledgeManagement")>();
  return {
    ...actual,
    knowledgeManagement: {
      ...actual.knowledgeManagement,
      documentMetadataConfig: h.metadata,
      createDocument: h.create,
    },
  };
});
beforeEach(() => {
  h.get.mockReset();
  h.update.mockReset();
  h.metadata.mockReset();
  h.create.mockReset();
  h.saved.mockReset();
  h.close.mockReset();
});
it("does not resubmit unchanged parser settings, metadata values or template", async () => {
  const doc = normalizeDocument({
    id: "d1",
    name: "Notes",
    meta_fields: { author: "Alice" },
    parser_config: {
      chunk_token_num: 512,
      delimiter: "\\n",
      raptor: { use_raptor: true },
      metadata: [{ key: "author" }],
    },
  });
  h.update.mockImplementation(
    (_id: string, _doc: string, patch: Record<string, unknown>) =>
      Promise.resolve(normalizeDocument({ ...doc, ...patch })),
  );
  h.get.mockResolvedValue(doc);
  h.metadata.mockResolvedValue({});
  const user = userEvent.setup();
  render(
    <ConfigProvider>
      <KnowledgeDocumentEditor
        datasetId="kb1"
        document={doc}
        onSaved={h.saved}
        onClose={h.close}
      />
    </ConfigProvider>,
  );
  await user.click(await screen.findByRole("button", { name: /保\s*存/ }));
  await waitFor(() => {
    expect(h.close).toHaveBeenCalledTimes(1);
  });
  expect(h.update).not.toHaveBeenCalled();
  expect(h.metadata).not.toHaveBeenCalled();
});
it("creates a blank document then opens its chunk editor without parsing", async () => {
  h.create.mockResolvedValue(
    normalizeDocument({ id: "blank1", type: "virtual" }),
  );
  const user = userEvent.setup();
  render(
    <ConfigProvider>
      <MemoryRouter initialEntries={["/knowledge/kb1/documents"]}>
        <Routes>
          <Route
            path="/knowledge/:id/documents"
            element={
              <KnowledgeDocumentCreate
                datasetId="kb1"
                onSaved={h.saved}
                onClose={h.close}
              />
            }
          />
          <Route
            path="/knowledge/:id/documents/:doc/chunks"
            element={<div>chunk destination</div>}
          />
        </Routes>
      </MemoryRouter>
    </ConfigProvider>,
  );
  await user.click(screen.getByRole("combobox", { name: "来源" }));
  await user.click(screen.getByText("空白文档"));
  await user.type(screen.getByRole("textbox", { name: "名称" }), "Notes");
  await user.click(screen.getByRole("button", { name: /创\s*建/ }));
  await screen.findByText("chunk destination");
  expect(h.create).toHaveBeenCalledWith("kb1", "empty", "Notes", undefined);
});

it("keeps the editor open when the server accepts settings without persisting the chunk length", async () => {
  const doc = normalizeDocument({
    id: "d1",
    name: "Notes",
    parser_config: { chunk_token_num: 512, metadata: [] },
  });
  h.update.mockResolvedValue(doc);
  h.metadata.mockResolvedValue({});
  h.get.mockResolvedValue(doc);
  render(
    <ConfigProvider>
      <KnowledgeDocumentEditor
        datasetId="kb1"
        document={doc}
        onSaved={h.saved}
        onClose={h.close}
      />
    </ConfigProvider>,
  );
  const user = userEvent.setup();
  const input = await screen.findByRole("spinbutton");
  await user.clear(input);
  await user.type(input, "256");
  await user.click(await screen.findByRole("button", { name: /保\s*存/ }));
  expect(
    await screen.findByText(/保存后回读未确认，请重试/),
  ).toBeInTheDocument();
  expect(h.close).not.toHaveBeenCalled();
  expect(h.get).toHaveBeenCalledWith("kb1", "d1");
});
it("checks the final metadata template readback and preserves the draft on mismatch", async () => {
  const doc = normalizeDocument({
    id: "d1",
    name: "Notes",
    parser_config: { chunk_token_num: 512, delimiter: "\n", metadata: [] },
  });
  h.update.mockResolvedValue(doc);
  h.metadata.mockResolvedValue({});
  h.get.mockResolvedValue(doc);
  const user = userEvent.setup();
  render(
    <ConfigProvider>
      <KnowledgeDocumentEditor
        datasetId="kb1"
        document={doc}
        onSaved={h.saved}
        onClose={h.close}
      />
    </ConfigProvider>,
  );
  await user.click(await screen.findByRole("tab", { name: "元数据" }));
  await user.click(
    screen
      .getAllByRole("button", { name: "添加字段" })
      .find((button) => !button.hasAttribute("disabled"))!,
  );
  const template = screen.getByRole("textbox", { name: "字段 1 的名称" });
  await user.type(template, "version");
  await user.click(await screen.findByRole("button", { name: /保\s*存/ }));
  await screen.findByText(/保存后回读未确认，请重试/);
  expect(h.metadata).toHaveBeenCalledWith("kb1", "d1", [
    { key: "version", type: "string" },
  ]);
  expect(h.close).not.toHaveBeenCalled();
  expect(template).toHaveValue("version");
});

it("sends only a changed field and preserves a DataFlow and historical settings", async () => {
  const doc = normalizeDocument({
    id: "d1",
    name: "notes.txt",
    pipeline_id: "flow",
    parser_config: {
      chunk_token_num: 512,
      control_panel: { display_name: "keep" },
      raptor: { use_raptor: true, legacy: 1 },
      metadata: [],
    },
  });
  h.get
    .mockResolvedValueOnce(doc)
    .mockResolvedValueOnce(doc)
    .mockResolvedValue({
      ...doc,
      parser_config: { ...doc.parser_config, chunk_token_num: 256 },
    });
  h.update.mockResolvedValue(doc);
  const user = userEvent.setup();
  render(
    <ConfigProvider>
      <KnowledgeDocumentEditor
        datasetId="kb1"
        document={doc}
        onSaved={h.saved}
        onClose={h.close}
      />
    </ConfigProvider>,
  );
  const input = await screen.findByRole("spinbutton");
  await user.clear(input);
  await user.type(input, "256");
  await user.click(screen.getByRole("button", { name: /保\s*存/ }));
  await waitFor(() => {
    expect(h.close).toHaveBeenCalled();
  });
  expect(h.update).toHaveBeenCalledWith("kb1", "d1", {
    parser_config: { chunk_token_num: 256 },
  });
  expect(h.metadata).not.toHaveBeenCalled();
});
it("saves metadata independently after a parser failure and retries only the remaining write", async () => {
  const doc = normalizeDocument({
    id: "d1",
    name: "notes.txt",
    parser_config: { chunk_token_num: 512, metadata: [] },
  });
  h.get.mockResolvedValue(doc);
  h.update.mockRejectedValueOnce(new Error("parser unavailable"));
  h.metadata.mockImplementation(() => {
    h.get.mockResolvedValue({
      ...doc,
      parser_config: {
        ...doc.parser_config,
        metadata: [{ key: "version", type: "string" }],
      },
    });
    return Promise.resolve({});
  });
  const user = userEvent.setup();
  render(
    <ConfigProvider>
      <KnowledgeDocumentEditor
        datasetId="kb1"
        document={doc}
        onSaved={h.saved}
        onClose={h.close}
      />
    </ConfigProvider>,
  );
  const input = await screen.findByRole("spinbutton");
  await user.clear(input);
  await user.type(input, "256");
  await user.click(screen.getByRole("tab", { name: "元数据" }));
  await user.click(
    screen
      .getAllByRole("button", { name: "添加字段" })
      .find((button) => !button.hasAttribute("disabled"))!,
  );
  await user.type(
    screen.getByRole("textbox", { name: "字段 1 的名称" }),
    "version",
  );
  await user.click(screen.getByRole("button", { name: /保\s*存/ }));
  await screen.findByText(/parser unavailable/);
  expect(h.metadata).toHaveBeenCalledTimes(1);
  expect(h.close).not.toHaveBeenCalled();
  h.update.mockResolvedValue({});
  h.get.mockResolvedValue({
    ...doc,
    parser_config: {
      chunk_token_num: 256,
      metadata: [{ key: "version", type: "string" }],
    },
  });
  await user.click(screen.getByRole("button", { name: /保\s*存/ }));
  await waitFor(() => {
    expect(h.close).toHaveBeenCalled();
  });
  expect(h.update).toHaveBeenCalledTimes(1);
  expect(h.metadata).toHaveBeenCalledTimes(1);
});
it("blocks submission after a latest-document load failure and offers retry", async () => {
  h.get.mockRejectedValueOnce(new Error("load denied"));
  const doc = normalizeDocument({ id: "d1" });
  const user = userEvent.setup();
  render(
    <ConfigProvider>
      <KnowledgeDocumentEditor
        datasetId="kb1"
        document={doc}
        onSaved={h.saved}
        onClose={h.close}
      />
    </ConfigProvider>,
  );
  await screen.findByText("load denied");
  expect(screen.queryByRole("button", { name: /保\s*存/ })).toBeNull();
  h.get.mockResolvedValue(doc);
  await user.click(screen.getByRole("button", { name: "重试失败项" }));
  expect(
    await screen.findByRole("button", { name: /保\s*存/ }),
  ).toBeInTheDocument();
});

it("does not overwrite an untouched template returned by the parser readback", async () => {
  const initial = normalizeDocument({
    id: "d1",
    name: "notes.txt",
    parser_config: {
      chunk_token_num: 512,
      metadata: [{ key: "old", type: "string" }],
    },
  });
  let server = initial;
  h.get.mockImplementation(() => Promise.resolve(server));
  h.update.mockImplementation((_kb, _id, patch) => {
    server = normalizeDocument({
      ...initial,
      parser_config: {
        chunk_token_num: patch.parser_config.chunk_token_num,
        metadata: [{ key: "concurrent", type: "string" }],
      },
    });
    return Promise.resolve(server);
  });
  const user = userEvent.setup();
  render(
    <ConfigProvider>
      <KnowledgeDocumentEditor
        datasetId="kb1"
        document={initial}
        onSaved={h.saved}
        onClose={h.close}
      />
    </ConfigProvider>,
  );
  const input = await screen.findByRole("spinbutton");
  await user.clear(input);
  await user.type(input, "256");
  await user.click(screen.getByRole("button", { name: /保\s*存/ }));
  await waitFor(() => {
    expect(h.close).toHaveBeenCalled();
  });
  expect(h.update).toHaveBeenCalledWith("kb1", "d1", {
    parser_config: { chunk_token_num: 256 },
  });
  expect(h.metadata).not.toHaveBeenCalled();
  expect(server.parser_config.metadata).toEqual([
    { key: "concurrent", type: "string" },
  ]);
});
it("does not replay untouched parser, values or advanced fields after a partial save", async () => {
  const initial = normalizeDocument({
    id: "d1",
    name: "notes.txt",
    parser_id: "naive",
    meta_fields: { author: "Alice" },
    parser_config: { chunk_token_num: 512, auto_questions: 1, metadata: [] },
  });
  let server = initial;
  h.get.mockImplementation(() => Promise.resolve(server));
  h.update.mockImplementation((_kb, _id, patch) => {
    server = normalizeDocument({
      ...initial,
      parser_id: "manual",
      meta_fields: { author: "Bob" },
      parser_config: {
        chunk_token_num: patch.parser_config.chunk_token_num,
        auto_questions: 2,
        metadata: [],
      },
    });
    return Promise.resolve(server);
  });
  h.metadata
    .mockRejectedValueOnce(new Error("template unavailable"))
    .mockImplementation((_kb, _id, template) => {
      server = {
        ...server,
        parser_config: { ...server.parser_config, metadata: template },
      };
      return Promise.resolve({});
    });
  const user = userEvent.setup();
  render(
    <ConfigProvider>
      <KnowledgeDocumentEditor
        datasetId="kb1"
        document={initial}
        onSaved={h.saved}
        onClose={h.close}
      />
    </ConfigProvider>,
  );
  const input = await screen.findByRole("spinbutton");
  await user.clear(input);
  await user.type(input, "256");
  await user.click(screen.getByRole("tab", { name: "元数据" }));
  await user.click(
    screen
      .getAllByRole("button", { name: "添加字段" })
      .find((button) => !button.hasAttribute("disabled"))!,
  );
  await user.type(
    screen.getByRole("textbox", { name: "字段 1 的名称" }),
    "version",
  );
  await user.click(screen.getByRole("button", { name: /保\s*存/ }));
  await screen.findByText(/template unavailable/);
  await user.click(screen.getByRole("button", { name: /保\s*存/ }));
  await waitFor(() => {
    expect(h.close).toHaveBeenCalled();
  });
  expect(h.update).toHaveBeenCalledTimes(1);
  expect(server.parser_id).toBe("manual");
  expect(server.meta_fields).toEqual([{ key: "author", value: "Bob" }]);
  expect(server.parser_config.auto_questions).toBe(2);
});
it("accepts a normalized parent-child disable readback", async () => {
  const initial = normalizeDocument({
    id: "d1",
    name: "notes.txt",
    parser_config: {
      parent_child: { use_parent_child: true, children_delimiter: "x" },
      metadata: [],
    },
  });
  let server = initial;
  h.get.mockImplementation(() => Promise.resolve(server));
  h.update.mockImplementation(() => {
    server = {
      ...server,
      parser_config: {
        parent_child: {},
        enable_children: false,
        children_delimiter: "",
        metadata: [],
      },
    };
    return Promise.resolve(server);
  });
  const user = userEvent.setup();
  render(
    <ConfigProvider>
      <KnowledgeDocumentEditor
        datasetId="kb1"
        document={initial}
        onSaved={h.saved}
        onClose={h.close}
      />
    </ConfigProvider>,
  );
  await user.click(await screen.findByRole("tab", { name: "高级 JSON" }));
  const advanced = screen.getByRole("textbox", {
    name: "高级 JSON",
    hidden: false,
  });
  await user.clear(advanced);
  await user.click(advanced);
  await user.paste(
    '{"parent_child":{"use_parent_child":false,"children_delimiter":"x"}}',
  );
  await user.click(screen.getByRole("button", { name: /保\s*存/ }));
  await waitFor(() => {
    expect(h.close).toHaveBeenCalled();
  });
});
it("confirms a legacy template using the actual schema returned by document GET", async () => {
  const initial = normalizeDocument({
    id: "d1",
    name: "notes.txt",
    parser_config: { metadata: [] },
  });
  let server = initial;
  h.get.mockImplementation(() => Promise.resolve(server));
  h.metadata.mockImplementation(() => {
    server = {
      ...server,
      parser_config: {
        metadata: {
          type: "object",
          properties: { version: { description: "", type: "string" } },
          additionalProperties: false,
        },
      },
    };
    return Promise.resolve({});
  });
  const user = userEvent.setup();
  render(
    <ConfigProvider>
      <KnowledgeDocumentEditor
        datasetId="kb1"
        document={initial}
        onSaved={h.saved}
        onClose={h.close}
      />
    </ConfigProvider>,
  );
  await user.click(await screen.findByRole("tab", { name: "元数据" }));
  await user.click(
    screen
      .getAllByRole("button", { name: "添加字段" })
      .find((button) => !button.hasAttribute("disabled"))!,
  );
  await user.type(
    screen.getByRole("textbox", { name: "字段 1 的名称" }),
    "version",
  );
  await user.click(screen.getByRole("button", { name: /保\s*存/ }));
  await waitFor(() => {
    expect(h.close).toHaveBeenCalledTimes(1);
  });
  expect(h.metadata).toHaveBeenCalledTimes(1);
  expect(h.update).not.toHaveBeenCalled();
});
it("preserves a changed template draft when another writer changed the template", async () => {
  const initial = normalizeDocument({
    id: "d1",
    name: "notes.txt",
    parser_config: { metadata: [] },
  });
  h.get.mockResolvedValueOnce(initial).mockResolvedValue({
    ...initial,
    parser_config: { metadata: [{ key: "concurrent", type: "string" }] },
  });
  const user = userEvent.setup();
  render(
    <ConfigProvider>
      <KnowledgeDocumentEditor
        datasetId="kb1"
        document={initial}
        onSaved={h.saved}
        onClose={h.close}
      />
    </ConfigProvider>,
  );
  await user.click(await screen.findByRole("tab", { name: "元数据" }));
  await user.click(
    screen
      .getAllByRole("button", { name: "添加字段" })
      .find((button) => !button.hasAttribute("disabled"))!,
  );
  await user.type(
    screen.getByRole("textbox", { name: "字段 1 的名称" }),
    "version",
  );
  await user.click(screen.getByRole("button", { name: /保\s*存/ }));
  await waitFor(() => {
    expect(h.get).toHaveBeenCalledTimes(2);
  });
  expect(h.metadata).not.toHaveBeenCalled();
  expect(h.close).not.toHaveBeenCalled();
  expect(screen.getByRole("textbox", { name: "字段 1 的名称" })).toHaveValue(
    "version",
  );
});
it("restores an explicitly reverted parameter after its previous write could not be read back", async () => {
  const initial = normalizeDocument({
    id: "d1",
    name: "notes.txt",
    parser_config: { chunk_token_num: 512, metadata: [] },
  });
  let server = initial;
  let reads = 0;
  h.get.mockImplementation(() => {
    reads++;
    return reads === 3
      ? Promise.reject(new Error("read unavailable"))
      : Promise.resolve(server);
  });
  h.update.mockImplementation((_kb, _id, patch) => {
    server = {
      ...server,
      parser_config: { ...server.parser_config, ...patch.parser_config },
    };
    return Promise.resolve(server);
  });
  const user = userEvent.setup();
  render(
    <ConfigProvider>
      <KnowledgeDocumentEditor
        datasetId="kb1"
        document={initial}
        onSaved={h.saved}
        onClose={h.close}
      />
    </ConfigProvider>,
  );
  const input = await screen.findByRole("spinbutton");
  await user.clear(input);
  await user.type(input, "256");
  await user.click(screen.getByRole("button", { name: /保\s*存/ }));
  await screen.findByText(/read unavailable/);
  await user.clear(input);
  await user.type(input, "512");
  await user.click(screen.getByRole("button", { name: /保\s*存/ }));
  await waitFor(() => {
    expect(h.close).toHaveBeenCalledTimes(1);
  });
  expect(server.parser_config.chunk_token_num).toBe(512);
  expect(h.update).toHaveBeenCalledTimes(2);
});
it("clears an explicitly reverted template after a previous write succeeded but readback failed", async () => {
  const initial = normalizeDocument({
    id: "d1",
    name: "notes.txt",
    parser_config: { metadata: [] },
  });
  let server = initial;
  let reads = 0;
  h.get.mockImplementation(() => {
    reads++;
    return reads === 3
      ? Promise.reject(new Error("read unavailable"))
      : Promise.resolve(server);
  });
  h.metadata.mockImplementation((_kb, _id, template) => {
    server = {
      ...server,
      parser_config: {
        metadata: template.length
          ? {
              type: "object",
              properties: { version: { description: "", type: "string" } },
              additionalProperties: false,
            }
          : [],
      },
    };
    return Promise.resolve({});
  });
  const user = userEvent.setup();
  render(
    <ConfigProvider>
      <KnowledgeDocumentEditor
        datasetId="kb1"
        document={initial}
        onSaved={h.saved}
        onClose={h.close}
      />
    </ConfigProvider>,
  );
  await user.click(await screen.findByRole("tab", { name: "元数据" }));
  await user.click(
    screen
      .getAllByRole("button", { name: "添加字段" })
      .find((button) => !button.hasAttribute("disabled"))!,
  );
  await user.type(
    screen.getByRole("textbox", { name: "字段 1 的名称" }),
    "version",
  );
  await user.click(screen.getByRole("button", { name: /保\s*存/ }));
  await screen.findByText(/read unavailable/);
  await user.click(screen.getByRole("button", { name: "删除字段 1" }));
  await user.click(screen.getByRole("button", { name: /保\s*存/ }));
  await waitFor(() => {
    expect(h.close).toHaveBeenCalledTimes(1);
  });
  expect(server.parser_config.metadata).toEqual([]);
  expect(h.metadata).toHaveBeenCalledTimes(2);
});
it("keeps a reverted unconfirmed flow exit open instead of resetting it again", async () => {
  const initial = normalizeDocument({
    id: "d1",
    name: "notes.txt",
    pipeline_id: "flow",
    parser_id: "naive",
    parser_config: { metadata: [] },
  });
  let server = initial;
  let reads = 0;
  h.get.mockImplementation(() => {
    reads++;
    return reads === 3
      ? Promise.reject(new Error("read unavailable"))
      : Promise.resolve(server);
  });
  h.update.mockImplementation(() => {
    server = { ...server, parser_id: "manual", pipeline_id: "" };
    return Promise.resolve(server);
  });
  const user = userEvent.setup();
  render(
    <ConfigProvider>
      <KnowledgeDocumentEditor
        datasetId="kb1"
        document={initial}
        onSaved={h.saved}
        onClose={h.close}
      />
    </ConfigProvider>,
  );
  const picker = await screen.findByRole("combobox", { name: "解析方法" });
  await user.click(picker);
  await user.click(
    screen.getByText("manual", { selector: ".ant-select-item-option-content" }),
  );
  await user.click(screen.getByRole("button", { name: /保\s*存/ }));
  await screen.findByText(/read unavailable/);
  await user.click(picker);
  await user.click(
    screen.getByText("naive", { selector: ".ant-select-item-option-content" }),
  );
  await user.click(screen.getByRole("button", { name: /保\s*存/ }));
  await waitFor(() => {
    expect(h.get).toHaveBeenCalledTimes(4);
  });
  expect(h.update).toHaveBeenCalledTimes(1);
  expect(h.close).not.toHaveBeenCalled();
  expect(server.parser_id).toBe("manual");
});
it("saves extraction enablement through document PATCH and independently confirms false", async () => {
  const initial = normalizeDocument({
    id: "d1",
    name: "notes.txt",
    parser_config: { enable_metadata: true, metadata: [] },
  });
  let server = initial;
  h.get.mockImplementation(() => Promise.resolve(server));
  h.update.mockImplementation((_kb, _id, patch) => {
    server = {
      ...server,
      parser_config: { ...server.parser_config, ...patch.parser_config },
    };
    return Promise.resolve(server);
  });
  const user = userEvent.setup();
  render(
    <ConfigProvider>
      <KnowledgeDocumentEditor
        datasetId="kb1"
        document={initial}
        onSaved={h.saved}
        onClose={h.close}
      />
    </ConfigProvider>,
  );
  await user.click(await screen.findByRole("tab", { name: "元数据" }));
  await user.click(
    screen.getByRole("checkbox", { name: "启用该文档自动提取元数据" }),
  );
  await user.click(screen.getByRole("button", { name: /保\s*存/ }));
  await waitFor(() => {
    expect(h.close).toHaveBeenCalled();
  });
  expect(h.update).toHaveBeenCalledWith("kb1", "d1", {
    parser_config: { enable_metadata: false },
  });
  expect(h.metadata).not.toHaveBeenCalled();
});
it("does not invent an enablement default when only the template is edited", async () => {
  const initial = normalizeDocument({
    id: "d1",
    name: "notes.txt",
    parser_config: { metadata: [] },
  });
  let server = initial;
  h.get.mockImplementation(() => Promise.resolve(server));
  h.metadata.mockImplementation((_kb, _id, template) => {
    server = { ...server, parser_config: { metadata: template } };
    return Promise.resolve({});
  });
  const user = userEvent.setup();
  render(
    <ConfigProvider>
      <KnowledgeDocumentEditor
        datasetId="kb1"
        document={initial}
        onSaved={h.saved}
        onClose={h.close}
      />
    </ConfigProvider>,
  );
  await user.click(await screen.findByRole("tab", { name: "元数据" }));
  await user.click(
    screen
      .getAllByRole("button", { name: "添加字段" })
      .find((button) => !button.hasAttribute("disabled"))!,
  );
  await user.type(
    screen.getByRole("textbox", { name: "字段 1 的名称" }),
    "version",
  );
  await user.click(screen.getByRole("button", { name: /保\s*存/ }));
  await waitFor(() => {
    expect(h.close).toHaveBeenCalled();
  });
  expect(h.update).not.toHaveBeenCalled();
  expect(h.metadata).toHaveBeenCalledWith("kb1", "d1", [
    { key: "version", type: "string" },
  ]);
  expect(server.parser_config).not.toHaveProperty("enable_metadata");
});
