import { beforeEach, expect, it, vi } from "vitest";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ConfigProvider } from "antd";
import MetadataBatchEditor from "./MetadataBatchEditor";
import { setAuthRole } from "@/test/auth-store-mock";

const h = vi.hoisted(() => ({
  list: vi.fn(),
  get: vi.fn(),
  request: vi.fn(),
  meta: { author: "Alice", revision: 0, flags: false } as Record<
    string,
    unknown
  >,
  incomplete: false,
  unmatched: false,
  stale: false,
  readFailure: false,
}));
vi.mock("@/stores/auth", async () =>
  (await import("@/test/auth-store-mock")).createAuthStoreMock(),
);
vi.mock("@/api/knowledge", () => ({
  knowledgeApi: { documents: { list: h.list, get: h.get } },
}));
vi.mock("@/api/knowledgeManagement", async (original) => ({
  ...(await original<typeof import("@/api/knowledgeManagement")>()),
  knowledgeRequest: h.request,
}));
vi.mock("@/queries/useKnowledgeManagement", () => ({
  useKnowledgeMetadataInventory: () => ({
    data: {
      keys: ["author"],
      flattened: { author: { Alice: ["d1"], Bob: ["d2"] } },
    },
  }),
}));
const doc = (id = "d1") => ({
  id,
  name: `${id}.txt`,
  meta_fields: Object.entries(h.meta).map(([key, value]) => ({ key, value })),
});
beforeEach(() => {
  setAuthRole("admin");
  h.meta = { author: "Alice", revision: 0, flags: false };
  h.incomplete = false;
  h.unmatched = false;
  h.stale = false;
  h.readFailure = false;
  h.list.mockReset();
  h.get.mockReset();
  h.request.mockReset();
  h.list.mockImplementation(async (_id, params) => {
    if (params.ids)
      return {
        total: h.unmatched ? 0 : h.incomplete ? 2 : params.ids.length,
        documents:
          h.unmatched || (h.incomplete && params.page > 1)
            ? []
            : params.ids.map((id: string) => doc(id)),
      };
    return { total: 101, documents: [doc(params.page === 2 ? "d2" : "d1")] };
  });
  h.get.mockImplementation(async (_id, id) => {
    if (h.readFailure) throw new Error("read failed");
    return {
      ...doc(id),
      meta_fields: Object.entries(
        h.stale ? { ...h.meta, author: "Concurrent" } : h.meta,
      ).map(([key, value]) => ({ key, value })),
    };
  });
  h.request.mockImplementation(async (_method, _id, _resource, body) => {
    if (body.updates.length)
      h.meta[body.updates[0].key] = body.updates[0].value;
    for (const row of body.deletes) Reflect.deleteProperty(h.meta, row.key);
    return {
      matched_docs: body.selector.document_ids.length,
      updated: body.selector.document_ids.length,
    };
  });
});
function setup(fields = [{ key: "author", type: "string" }]) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <ConfigProvider>
        <MetadataBatchEditor id="kb1" fields={fields} onSaved={vi.fn()} />
      </ConfigProvider>
    </QueryClientProvider>,
  );
}
async function chooseField(
  user: ReturnType<typeof userEvent.setup>,
  name = "author",
) {
  await user.click(screen.getByRole("combobox", { name: "字段" }));
  await user.click(
    screen.getByText(name, { selector: ".ant-select-item-option-content" }),
  );
}
async function selectDocument(user: ReturnType<typeof userEvent.setup>) {
  await screen.findByText("d1.txt");
  await user.click(screen.getAllByRole("checkbox")[1]);
  await chooseField(user);
}
it("keeps cross-page selection, requires review and confirms only the explicit IDs", async () => {
  const user = userEvent.setup();
  setup();
  await selectDocument(user);
  await user.click(screen.getByTitle("2"));
  await screen.findByText("d2.txt");
  await user.click(screen.getAllByRole("checkbox")[1]);
  await user.type(screen.getByRole("textbox", { name: "新值" }), "Bob");
  await user.click(screen.getByRole("button", { name: "核对变化" }));
  const dialog = await screen.findByRole("dialog");
  expect(within(dialog).getByText(/匹配 2 个文档/)).toBeInTheDocument();
  expect(h.request).not.toHaveBeenCalled();
  await user.click(
    within(dialog).getByRole("button", { name: "应用到 2 个文档" }),
  );
  await screen.findByText("已读回确认 2 个文档。");
  expect(h.request).toHaveBeenCalledWith(
    "patch",
    "kb1",
    "documents/metadatas",
    {
      selector: { document_ids: ["d1", "d2"] },
      updates: [{ key: "author", value: "Bob" }],
      deletes: [],
    },
  );
  expect(h.list.mock.calls.every(([, params]) => params.page_size <= 100)).toBe(
    true,
  );
});
it("fails closed when a filtered preview is empty or incomplete", async () => {
  const user = userEvent.setup();
  setup();
  await selectDocument(user);
  await user.type(screen.getByRole("textbox", { name: "新值" }), "Bob");
  h.incomplete = true;
  await user.click(screen.getByRole("button", { name: "核对变化" }));
  await screen.findByText("核对后元数据已变化，请重新核对最新值再应用。");
  expect(h.request).not.toHaveBeenCalled();
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
});
it("refuses a concurrent value change after preview", async () => {
  const user = userEvent.setup();
  setup();
  await selectDocument(user);
  await user.type(screen.getByRole("textbox", { name: "新值" }), "Bob");
  await user.click(screen.getByRole("button", { name: "核对变化" }));
  const dialog = await screen.findByRole("dialog");
  h.stale = true;
  await user.click(
    within(dialog).getByRole("button", { name: "应用到 1 个文档" }),
  );
  await within(dialog).findByText(
    "核对后元数据已变化，请重新核对最新值再应用。",
  );
  expect(h.request).not.toHaveBeenCalled();
});
it("retains failed readback scope and retries reads without another write", async () => {
  const user = userEvent.setup();
  setup();
  await selectDocument(user);
  await user.type(screen.getByRole("textbox", { name: "新值" }), "Bob");
  await user.click(screen.getByRole("button", { name: "核对变化" }));
  const dialog = await screen.findByRole("dialog");
  h.request.mockImplementation(async () => {
    h.meta.author = "Bob";
    h.readFailure = true;
    return { matched_docs: 1, updated: 1 };
  });
  await user.click(
    within(dialog).getByRole("button", { name: "应用到 1 个文档" }),
  );
  await screen.findByText(/1 个文档未能确认/);
  expect(screen.getByText("已选 1 个文档")).toBeInTheDocument();
  h.readFailure = false;
  await user.click(screen.getByRole("button", { name: "仅重试读回" }));
  await screen.findByText("已读回确认 1 个文档。");
  expect(h.request).toHaveBeenCalledTimes(1);
});
it("intersects the condition with selected IDs and never writes on zero matches", async () => {
  const user = userEvent.setup();
  setup();
  await selectDocument(user);
  await user.type(screen.getByRole("textbox", { name: "新值" }), "Bob");
  await user.click(
    screen.getByRole("checkbox", { name: "用元数据缩小已选文档范围" }),
  );
  await user.click(
    screen.getByRole("combobox", { name: "用元数据缩小已选文档范围" }),
  );
  await user.click(
    screen
      .getAllByText("author", { selector: ".ant-select-item-option-content" })
      .at(-1)!,
  );
  await user.click(screen.getByRole("combobox", { name: "已有值" }));
  await user.click(
    screen.getByText("Bob", { selector: ".ant-select-item-option-content" }),
  );
  h.unmatched = true;
  await user.click(screen.getByRole("button", { name: "核对变化" }));
  await screen.findByText("已选文档均不匹配条件，未发送修改。");
  expect(h.list).toHaveBeenCalledWith(
    "kb1",
    expect.objectContaining({
      ids: ["d1"],
      metadata_condition: JSON.stringify({
        logic: "and",
        conditions: [
          { name: "author", comparison_operator: "=", value: "Bob" },
        ],
      }),
    }),
  );
  expect(h.request).not.toHaveBeenCalled();
});
it("blocks writes for members", async () => {
  setAuthRole("member");
  setup();
  await screen.findByText("d1.txt");
  expect(screen.getByRole("button", { name: "核对变化" })).toBeDisabled();
  expect(screen.getAllByRole("checkbox")[1]).toBeDisabled();
});
it("confirms multiple update and delete operations in one bounded request", async () => {
  const user = userEvent.setup();
  setup();
  await selectDocument(user);
  await user.type(screen.getByRole("textbox", { name: "新值" }), "Bob");
  await user.click(screen.getByRole("button", { name: "添加另一字段操作" }));
  await user.type(
    screen.getByRole("combobox", { name: "字段" }),
    "flags{Enter}",
  );
  await user.click(screen.getByRole("combobox", { name: "操作" }));
  await user.click(
    screen.getByText("删除整个字段", {
      selector: ".ant-select-item-option-content",
    }),
  );
  await user.click(screen.getByRole("button", { name: "核对变化" }));
  const dialog = await screen.findByRole("dialog");
  await user.click(
    within(dialog).getByRole("button", { name: "应用到 1 个文档" }),
  );
  await screen.findByText("已读回确认 1 个文档。");
  expect(h.request).toHaveBeenCalledWith(
    "patch",
    "kb1",
    "documents/metadatas",
    {
      selector: { document_ids: ["d1"] },
      updates: [{ key: "author", value: "Bob" }],
      deletes: [{ key: "flags" }],
    },
  );
});
it.each(["same page", "across pages"])(
  "rejects duplicate preview IDs %s without reading or writing a partial selection",
  async (scenario) => {
    const user = userEvent.setup();
    setup();
    await selectDocument(user);
    await user.click(screen.getByTitle("2"));
    await screen.findByText("d2.txt");
    await user.click(screen.getAllByRole("checkbox")[1]);
    await user.type(screen.getByRole("textbox", { name: "新值" }), "Bob");
    h.list.mockImplementation(async (_id, params) =>
      params.ids
        ? {
            total: 2,
            documents:
              scenario === "same page" ? [doc("d1"), doc("d1")] : [doc("d1")],
          }
        : { total: 101, documents: [doc("d2")] },
    );
    await user.click(screen.getByRole("button", { name: "核对变化" }));
    await screen.findByText("核对后元数据已变化，请重新核对最新值再应用。");
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(h.get).not.toHaveBeenCalled();
    expect(h.request).not.toHaveBeenCalled();
    expect(screen.getByText("已选 2 个文档")).toBeInTheDocument();
  },
);

it("writes an untouched boolean field as explicit false instead of empty text", async () => {
  const user = userEvent.setup();
  setup([{ key: "approved", type: "boolean" }]);
  await screen.findByText("d1.txt");
  await user.click(screen.getAllByRole("checkbox")[1]);
  await chooseField(user, "approved");
  expect(screen.getByRole("switch", { name: "新值" })).not.toBeChecked();
  await user.click(screen.getByRole("button", { name: "核对变化" }));
  const dialog = await screen.findByRole("dialog");
  await user.click(
    within(dialog).getByRole("button", { name: "应用到 1 个文档" }),
  );
  await screen.findByText("已读回确认 1 个文档。");
  expect(h.request).toHaveBeenCalledWith(
    "patch",
    "kb1",
    "documents/metadatas",
    {
      selector: { document_ids: ["d1"] },
      updates: [{ key: "approved", value: false }],
      deletes: [],
    },
  );
});
it("treats an existing false value as a no-op when the boolean switch remains off", async () => {
  const user = userEvent.setup();
  setup([{ key: "flags", type: "boolean" }]);
  await screen.findByText("d1.txt");
  await user.click(screen.getAllByRole("checkbox")[1]);
  await chooseField(user, "flags");
  expect(screen.getByRole("switch", { name: "新值" })).not.toBeChecked();
  await user.click(screen.getByRole("button", { name: "核对变化" }));
  await screen.findByText("此操作不会改变现有值，未发送修改。");
  expect(h.request).not.toHaveBeenCalled();
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
});
async function chooseTextCondition(
  user: ReturnType<typeof userEvent.setup>,
  value: string,
) {
  await user.click(
    screen.getByRole("checkbox", { name: "用元数据缩小已选文档范围" }),
  );
  await user.click(
    screen.getByRole("combobox", { name: "用元数据缩小已选文档范围" }),
  );
  await user.click(
    screen
      .getAllByText("author", { selector: ".ant-select-item-option-content" })
      .at(-1)!,
  );
  await user.type(
    screen.getByRole("combobox", { name: "已有值" }),
    `${value}{Enter}`,
  );
}
it("blocks ambiguous numeric text conditions before contacting the batch selector", async () => {
  const user = userEvent.setup();
  setup();
  await selectDocument(user);
  await user.type(screen.getByRole("textbox", { name: "新值" }), "Bob");
  await chooseTextCondition(user, "0");
  await user.click(screen.getByRole("button", { name: "核对变化" }));
  expect(screen.getAllByText(/此处不提供排除条件/)).toHaveLength(2);
  expect(h.list.mock.calls.some(([, params]) => params.ids)).toBe(false);
  expect(h.request).not.toHaveBeenCalled();
});
it("fails closed when server text-filter matches contain a historical boolean value", async () => {
  const user = userEvent.setup();
  setup();
  await selectDocument(user);
  await user.type(screen.getByRole("textbox", { name: "新值" }), "Bob");
  await chooseTextCondition(user, "Alice");
  h.meta.author = false;
  await user.click(screen.getByRole("button", { name: "核对变化" }));
  await screen.findAllByText(/此处不提供排除条件/);
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  expect(h.request).not.toHaveBeenCalled();
});
it("blocks a false-to-zero type-only batch that the committed service would skip", async () => {
  const user = userEvent.setup();
  setup();
  await screen.findByText("d1.txt");
  await user.click(screen.getAllByRole("checkbox")[1]);
  await user.type(
    screen.getByRole("combobox", { name: "字段" }),
    "flags{Enter}",
  );
  await user.click(screen.getByRole("combobox", { name: "值类型" }));
  await user.click(
    screen.getByText("数字", { selector: ".ant-select-item-option-content" }),
  );
  await user.click(screen.getByRole("button", { name: "核对变化" }));
  await screen.findByText(/当前批量服务会将其视为未变化/);
  expect(h.request).not.toHaveBeenCalled();
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
});
it("rejects the 0x0 comparison before the backend can alias a 0b0 document", async () => {
  const user = userEvent.setup();
  setup();
  await selectDocument(user);
  await user.click(screen.getByTitle("2"));
  await screen.findByText("d2.txt");
  await user.click(screen.getAllByRole("checkbox")[1]);
  await user.type(screen.getByRole("textbox", { name: "新值" }), "Carol");
  await chooseTextCondition(user, "0x0");
  h.list.mockImplementation(async (_id, params) =>
    params.ids
      ? { total: 2, documents: [doc("d1"), doc("d2")] }
      : { total: 101, documents: [doc("d2")] },
  );
  await user.click(screen.getByRole("button", { name: "核对变化" }));
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  expect(h.list.mock.calls.some(([, params]) => params.ids)).toBe(false);
  expect(h.request).not.toHaveBeenCalled();
});
it("does not offer negative conditions that would include a mixed matching list", async () => {
  const user = userEvent.setup();
  setup();
  await selectDocument(user);
  await chooseTextCondition(user, "Alice");
  await user.click(screen.getByRole("combobox", { name: "比较方式" }));
  for (const label of ["不在列表中", "不包含", "不等于"])
    expect(
      screen.queryByText(label, {
        selector: ".ant-select-item-option-content",
      }),
    ).not.toBeInTheDocument();
  expect(
    screen.getByText("在列表中", {
      selector: ".ant-select-item-option-content",
    }),
  ).toBeInTheDocument();
  expect(
    screen.getByText("包含", { selector: ".ant-select-item-option-content" }),
  ).toBeInTheDocument();
  expect(h.request).not.toHaveBeenCalled();
});
it("retains positive text membership for 0x0 and freezes only its actual matching document", async () => {
  const user = userEvent.setup();
  h.meta.author = "0x0";
  setup();
  await selectDocument(user);
  await user.click(screen.getByTitle("2"));
  await screen.findByText("d2.txt");
  await user.click(screen.getAllByRole("checkbox")[1]);
  await user.type(screen.getByRole("textbox", { name: "新值" }), "Carol");
  await chooseTextCondition(user, "0x0");
  await user.click(screen.getByRole("combobox", { name: "比较方式" }));
  await user.click(
    screen.getByText("在列表中", {
      selector: ".ant-select-item-option-content",
    }),
  );
  await user.type(
    screen.getByRole("combobox", { name: "已有值" }),
    "0x0{Enter}",
  );
  h.list.mockImplementation(async (_id, params) =>
    params.ids
      ? { total: 1, documents: [doc("d1")] }
      : { total: 101, documents: [doc("d2")] },
  );
  await user.click(screen.getByRole("button", { name: "核对变化" }));
  const dialog = await screen.findByRole("dialog");
  await user.click(
    within(dialog).getByRole("button", { name: "应用到 1 个文档" }),
  );
  await screen.findByText("已读回确认 1 个文档。");
  expect(h.request).toHaveBeenCalledWith(
    "patch",
    "kb1",
    "documents/metadatas",
    {
      selector: {
        document_ids: ["d1"],
        metadata_condition: {
          logic: "and",
          conditions: [
            { name: "author", comparison_operator: "in", value: ["0x0"] },
          ],
        },
      },
      updates: [{ key: "author", value: "Carol" }],
      deletes: [],
    },
  );
});
it("rejects a returned plain-text document that does not satisfy the requested predicate", async () => {
  const user = userEvent.setup();
  setup();
  await selectDocument(user);
  await user.type(screen.getByRole("textbox", { name: "新值" }), "Carol");
  await chooseTextCondition(user, "Bob");
  await user.click(screen.getByRole("button", { name: "核对变化" }));
  await screen.findAllByText(/此处不提供排除条件/);
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  expect(h.request).not.toHaveBeenCalled();
});
