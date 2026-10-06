import { build } from "esbuild";
import { mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { pathToFileURL } from "node:url";
import { test, beforeEach, after } from "node:test";
import assert from "node:assert/strict";

// Exercise the real adapter with controlled HTTP responses; no live credentials.
const directory = await mkdtemp(join(tmpdir(), "hub-h5-knowledge-test-"));
const bundle = join(directory, "knowledge.mjs");
await build({
  entryPoints: [new URL("../src/api/knowledge.ts", import.meta.url).pathname],
  outfile: bundle,
  bundle: true,
  platform: "node",
  format: "esm",
});
const api = await import(pathToFileURL(bundle));
const stateBundle = join(directory, "mobile-state.mjs");
await build({
  entryPoints: [
    new URL("../src/components/knowledgeMobileState.ts", import.meta.url)
      .pathname,
  ],
  outfile: stateBundle,
  bundle: true,
  platform: "node",
  format: "esm",
});
const mobileState = await import(pathToFileURL(stateBundle));
const originalFetch = globalThis.fetch;
const originalStorage = globalThis.localStorage;
const storage = new Map();
globalThis.localStorage = { getItem: (key) => storage.get(key) ?? null };
beforeEach(() =>
  storage.set(
    "zerone_auth",
    JSON.stringify({ token: "controlled-token", role: "admin" }),
  ),
);
after(async () => {
  globalThis.fetch = originalFetch;
  globalThis.localStorage = originalStorage;
  await rm(directory, { recursive: true, force: true });
});
const response = (data, status = 200) =>
  new Response(JSON.stringify(data), {
    status,
    headers: { "content-type": "application/json" },
  });

test("multipart upload carries current Bearer and preserves FormData boundary", async () => {
  globalThis.fetch = async (url, init) => {
    assert.equal(url, "/api/v1/admin/knowledge/datasets/kb%2F1/documents");
    assert.equal(init.headers.Authorization, "Bearer controlled-token");
    assert.equal(init.headers["Content-Type"], undefined);
    assert.equal(init.body.getAll("files")[0].name, "test.txt");
    return response({
      success: true,
      data: [{ id: "doc1", name: "test.txt", run: "0", chunk_num: 0 }],
    });
  };
  const docs = await api.uploadDocuments("kb/1", "Library", [
    new File(["test"], "test.txt"),
  ]);
  assert.equal(docs[0].summary, "未开始解析 · 0 分块");
});
test("maps all frozen states and prioritizes failure over progress completion", () => {
  for (const [run, status] of [
    [0, "未开始解析"],
    [1, "解析中"],
    [2, "已取消解析"],
    [3, "已完成解析"],
    [4, "解析失败"],
    ["DONE", "已完成解析"],
    ["new-state", "状态未知"],
  ]) {
    const doc = api.remoteDocToKnowledgeDoc(
      { id: "doc1", name: "test.txt", run, chunk_num: 7 },
      "kb1",
      "Library",
    );
    assert.equal(doc.summary, `${status} · 7 分块`);
  }
  assert.equal(
    api.remoteDocToKnowledgeDoc(
      { id: "doc1", name: "x", run: 4, progress: 1 },
      "kb1",
      "L",
    ).tags[0],
    "解析失败",
  );
});
test("partial create retains durable id and recovery updates the same library", async () => {
  const methods = [];
  globalThis.fetch = async (url, init) => {
    methods.push(init.method || "GET");
    if (init.method === "POST")
      return response(
        {
          success: false,
          code: "knowledge_configuration_pending",
          error: "configuration pending",
          data: { dataset: { id: "created1", name: "Library" } },
        },
        502,
      );
    assert.ok(url.endsWith("/datasets/created1"));
    if (init.method === "PUT")
      assert.equal(JSON.parse(init.body).parser_config.chunk_token_num, 512);
    return response({
      success: true,
      data: { id: "created1", name: "Library", description: "Description" },
    });
  };
  let id;
  await assert.rejects(api.createDataset("Library", "Description"), (error) => {
    assert.ok(error instanceof api.KnowledgeConfigurationPendingError);
    id = error.folder.id;
    return true;
  });
  assert.equal(
    (await api.createDataset("Library", "Description", id)).id,
    "created1",
  );
  assert.deepEqual(methods, ["POST", "PUT", "GET"]);
});
test("document pagination does not silently truncate after ten pages", async () => {
  let count = 0;
  globalThis.fetch = async (url) => {
    const page = Number(
      new URL(url, "http://controlled.test").searchParams.get("page"),
    );
    count++;
    const docs = Array.from({ length: page === 11 ? 1 : 100 }, (_, i) => ({
      id: `doc-${page}-${i}`,
      name: "x",
      run: "3",
    }));
    return response({ success: true, data: { total: 1001, documents: docs } });
  };
  assert.equal((await api.listDocuments("kb1", "Library")).length, 1001);
  assert.equal(count, 11);
});
test("detects a non-advancing page instead of inventing completion", async () => {
  globalThis.fetch = async () =>
    response({
      success: true,
      data: {
        total: 201,
        documents: Array.from({ length: 100 }, (_, i) => ({
          id: `doc-${i}`,
          name: "x",
        })),
      },
    });
  await assert.rejects(api.listDocuments("kb1", "Library"), /分页未前进/);
});
test("parse sends exact acknowledged ids and HTTP 200 business failures reject", async () => {
  globalThis.fetch = async (url, init) => {
    assert.ok(url.endsWith("/datasets/kb1/documents/parse"));
    assert.equal(init.method, "POST");
    assert.deepEqual(JSON.parse(init.body), { document_ids: ["doc1"] });
    return response({ success: false, error: "partial parse failure" });
  };
  await assert.rejects(
    api.parseDocuments("kb1", ["doc1"]),
    /partial parse failure/,
  );
});
test("failed delete remains a failed operation with useful feedback", async () => {
  globalThis.fetch = async () =>
    response({ success: false, error: "not authorized" }, 403);
  await assert.rejects(api.deleteDocuments("kb1", ["doc1"]), /not authorized/);
});

test("stop uses the existing Hub route and waits for business acknowledgement", async () => {
  globalThis.fetch = async (url, init) => {
    assert.ok(url.endsWith("/datasets/kb1/documents/parse"));
    assert.equal(init.method, "DELETE");
    assert.deepEqual(JSON.parse(init.body), { document_ids: ["doc1"] });
    return response({ success: true, data: null });
  };
  await api.stopParsingDocuments("kb1", ["doc1"]);
});
test("rejects stale responses after identity changes, including same-role switches", async () => {
  globalThis.fetch = async () => {
    storage.set(
      "zerone_auth",
      JSON.stringify({ token: "different-token", role: "admin" }),
    );
    return response({ success: true, data: { total: 0, datasets: [] } });
  };
  await assert.rejects(api.listDatasets(), /登录身份已变化/);
});

test("parse failure resume never duplicates uploaded files", async () => {
  const calls = [];
  let parseCalls = 0;
  globalThis.fetch = async (url, init) => {
    calls.push(url);
    if (url.endsWith("/parse")) {
      parseCalls++;
      return response(
        parseCalls === 1
          ? { success: false, error: "parse failed" }
          : { success: true, data: null },
      );
    }
    return response({
      success: true,
      data: [{ id: "uploaded1", name: "x", run: "0" }],
    });
  };
  let resume;
  let visible = [];
  const onUploaded = (docs) => {
    visible = docs;
  };
  let refreshed = false;
  const refresh = async () => {
    refreshed = true;
  };
  await assert.rejects(
    api.completeUpload(
      "kb1",
      "L",
      [new File(["x"], "x")],
      true,
      onUploaded,
      refresh,
    ),
    (err) => {
      assert.ok(err instanceof api.KnowledgeUploadPendingError);
      resume = err.resume;
      return true;
    },
  );
  assert.equal(visible[0].id, "uploaded1");
  assert.equal(refreshed, false);
  assert.equal(resume.stage, "parse");
  await api.completeUpload("kb1", "L", [], true, onUploaded, refresh, resume);
  assert.equal(calls.filter((url) => url.endsWith("/documents")).length, 1);
  assert.equal(parseCalls, 2);
  assert.equal(refreshed, true);
});

test("readback failure resumes only readback after parsing was accepted", async () => {
  let requests = 0;
  globalThis.fetch = async (url) => {
    requests++;
    return response({
      success: true,
      data: url.endsWith("/parse") ? null : [{ id: "uploaded1", name: "x" }],
    });
  };
  let resume;
  await assert.rejects(
    api.completeUpload(
      "kb1",
      "L",
      [new File(["x"], "x")],
      true,
      () => {},
      async () => {
        throw new Error("readback failed");
      },
    ),
    (err) => {
      assert.ok(err instanceof api.KnowledgeUploadPendingError);
      resume = err.resume;
      return true;
    },
  );
  assert.equal(resume.stage, "refresh");
  await api.completeUpload(
    "kb1",
    "L",
    [],
    true,
    () => {},
    async () => {},
    resume,
  );
  assert.equal(requests, 2);
});

test("reparse uses the authenticated global ingest route and strict acceptance", async () => {
  globalThis.fetch = async (url, init) => {
    assert.equal(url, "/api/v1/admin/knowledge/documents/ingest");
    assert.equal(init.headers.Authorization, "Bearer controlled-token");
    assert.deepEqual(JSON.parse(init.body), {
      doc_ids: ["doc1"],
      run: 1,
      delete: false,
      apply_kb: true,
    });
    return response({ success: true, data: true });
  };
  await api.ingestDocuments(["doc1"], { delete: false, apply_kb: true });
  globalThis.fetch = async () => response({ success: true, data: null });
  await assert.rejects(
    api.ingestDocuments(["doc1"], { delete: false, apply_kb: false }),
    /未确认受理/,
  );
});

test("creation uses selected available configuration and verifies it independently", async () => {
  const calls = [];
  globalThis.fetch = async (url, init) => {
    calls.push([url, init.method || "GET"]);
    if (init.method === "POST") {
      const body = JSON.parse(init.body);
      assert.equal(body.embd_id, "Embed@Provider");
      assert.equal(body.parser_id, "qa");
      return response({ success: true, data: { id: "kb1", name: "Library" } });
    }
    return response({
      success: true,
      data: {
        id: "kb1",
        name: "Library",
        embd_id: "Embed@Provider",
        parser_id: "qa",
      },
    });
  };
  const folder = await api.createDataset("Library", "", undefined, {
    embeddingModel: "Embed@Provider",
    parseMethod: "qa",
  });
  assert.equal(folder.parseMethod, "qa");
  assert.deepEqual(
    calls.map((call) => call[1]),
    ["POST", "GET"],
  );
});

test("create readback mismatch preserves the durable library id", async () => {
  globalThis.fetch = async (_url, init) =>
    response({
      success: true,
      data: {
        id: "kb1",
        name: "Library",
        embd_id: "Other",
        parser_id: "naive",
      },
    });
  await assert.rejects(
    api.createDataset("Library", "", undefined, {
      embeddingModel: "Embed@Provider",
      parseMethod: "qa",
    }),
    (error) => {
      assert.ok(error instanceof api.KnowledgeConfigurationPendingError);
      assert.equal(error.folder.id, "kb1");
      assert.match(error.message, /尚未保存/);
      return true;
    },
  );
});

test("only enabled embedding candidates are available on mobile", async () => {
  globalThis.fetch = async (url, init) => {
    assert.ok(url.endsWith("/multirag/models?type=embedding"));
    assert.equal(init.headers.Authorization, "Bearer controlled-token");
    return response({
      success: true,
      data: [
        { fullId: "Enabled@P", status: "1", type: "embedding" },
        { fullId: "Disabled@P", status: "0", type: "embedding" },
        { fullId: "Chat@P", status: "1", type: "chat" },
      ],
    });
  };
  assert.deepEqual(
    (await api.listEmbeddingModels()).map((model) => model.fullId),
    ["Enabled@P"],
  );
});

test("zero chunks after completion is never presented as retrieval ready", () => {
  const doc = api.remoteDocToKnowledgeDoc(
    { id: "doc1", name: "empty.pdf", run: "DONE", chunk_num: 0 },
    "kb1",
    "Library",
  );
  assert.match(doc.summary, /未提取到可检索内容/);
  assert.equal(doc.run, "3");
  const failed = api.remoteDocToKnowledgeDoc(
    {
      id: "doc2",
      name: "error.pdf",
      run: "RUNNING",
      progress: -1,
      progress_msg: "provider unavailable",
    },
    "kb1",
    "Library",
  );
  assert.equal(failed.run, "4");
  assert.match(failed.summary, /provider unavailable/);
});

test("short list pages with unfulfilled totals fail closed", async () => {
  globalThis.fetch = async () =>
    response({
      success: true,
      data: { total: 120, datasets: [{ id: "kb1", name: "Library" }] },
    });
  await assert.rejects(api.listDatasets(), /分页结果不完整/);
});

test("chunk paging normalizes source, disabled state, image key and PDF positions", async () => {
  globalThis.fetch = async (url, init) => {
    assert.equal(
      url,
      "/api/v1/admin/knowledge/datasets/kb%2F1/documents/doc%2F1/chunks?page=2&page_size=20",
    );
    assert.equal(init.headers.Authorization, "Bearer controlled-token");
    return response({
      success: true,
      data: {
        total: 21,
        chunks: [
          {
            chunk_id: "c1",
            content_with_weight: "Sample",
            doc_id: "doc/1",
            available_int: 0,
            img_id: "kb/1-image/key",
            position_int: [[2, 1, 2, 3, 4], [2, 2, 3, 4, 5], [0], ["3"]],
          },
        ],
      },
    });
  };
  const data = await api.listDocumentChunks("kb/1", "doc/1", 2);
  assert.equal(data.chunks[0].content, "Sample");
  assert.equal(data.chunks[0].documentId, "doc/1");
  assert.equal(data.chunks[0].available, false);
  assert.deepEqual(api.chunkPageNumbers(data.chunks[0].positions), [2]);
});

test("mobile retrieval fixes scope and reuses exact page with no graph widening", async () => {
  globalThis.fetch = async (url, init) => {
    assert.ok(url.endsWith("/retrieval"));
    const body = JSON.parse(init.body);
    assert.deepEqual(body.dataset_ids, ["kb1"]);
    assert.deepEqual(body.document_ids, ["doc1"]);
    assert.equal(body.page, 2);
    assert.equal(body.page_size, 10);
    assert.equal(body.use_kg, false);
    assert.equal(body.question, "sample question");
    return response({ success: true, data: { total: 0, chunks: [] } });
  };
  assert.equal(
    (await api.searchKnowledge("kb1", " sample question ", 2, "doc1")).total,
    0,
  );
  globalThis.fetch = async () =>
    response({ success: false, error: "model unavailable" });
  await assert.rejects(api.searchKnowledge("kb1", "q"), /model unavailable/);
  globalThis.fetch = async () => response({ success: true, data: {} });
  await assert.rejects(api.searchKnowledge("kb1", "q"), /结果不完整/);
});

test("authorized source bytes use no-store and reject identity switches during body reads", async () => {
  globalThis.fetch = async (url, init) => {
    assert.ok(url.endsWith("/datasets/kb%2F1/documents/doc%2F1/download"));
    assert.equal(init.headers.Authorization, "Bearer controlled-token");
    assert.equal(init.cache, "no-store");
    return new Response("Original source", {
      headers: { "content-type": "text/plain" },
    });
  };
  assert.equal(
    await (await api.downloadOriginal("kb/1", "doc/1")).text(),
    "Original source",
  );
  globalThis.fetch = async () => ({
    ok: true,
    blob: async () => {
      storage.set(
        "zerone_auth",
        JSON.stringify({ token: "new-identity", role: "admin" }),
      );
      return new Blob(["secret"]);
    },
  });
  await assert.rejects(api.downloadOriginal("kb1", "doc1"), /登录身份已变化/);
});

test("protected image path is encoded and HTML never becomes an image URL", async () => {
  globalThis.fetch = async (url, init) => {
    assert.ok(
      url.endsWith("/datasets/kb1/images/kb1-%E4%B8%AD%E6%96%87%2F%252F"),
    );
    assert.equal(init.headers.Authorization, "Bearer controlled-token");
    return new Response("<html>login</html>", {
      headers: { "content-type": "text/html" },
    });
  };
  await assert.rejects(
    api.loadChunkImage("kb1", "kb1-中文/%2F"),
    /图片格式不支持/,
  );
});

test("identity changes during envelope parsing also reject stale responses", async () => {
  globalThis.fetch = async () => ({
    ok: true,
    json: async () => {
      storage.set(
        "zerone_auth",
        JSON.stringify({ token: "new-identity", role: "admin" }),
      );
      return { success: true, data: { total: 0, datasets: [] } };
    },
  });
  await assert.rejects(api.listDatasets(), /登录身份已变化/);
});

test("closing and reopening drafts retains File objects and recovery ids within one identity", () => {
  const drafts = mobileState.createKnowledgeDraftStore();
  const file = new File(["sample"], "sample.txt");
  drafts.write("user-a", "create-library", {
    name: "Draft",
    existingId: "created1",
  });
  drafts.write("user-a", "upload-documents", {
    files: [file],
    targetFolderId: "kb1",
    resume: { ids: ["uploaded1"], stage: "parse" },
  });
  assert.equal(
    drafts.read("user-a", "create-library", () => null).existingId,
    "created1",
  );
  const resumed = drafts.read("user-a", "upload-documents", () => null);
  assert.equal(resumed.files[0], file);
  assert.deepEqual(resumed.resume, { ids: ["uploaded1"], stage: "parse" });
  assert.equal(
    drafts.read("user-b", "create-library", () => null),
    null,
  );
  assert.equal(
    drafts.read("user-a", "upload-documents", () => null),
    null,
  );
});

test("completion removes only its draft; logout clears all drafts", () => {
  const drafts = mobileState.createKnowledgeDraftStore();
  drafts.write("user-a", "create", { existingId: "created1" });
  drafts.write("user-a", "upload", { ids: ["doc1"] });
  drafts.remove("user-a", "create");
  assert.equal(
    drafts.read("user-a", "create", () => null),
    null,
  );
  assert.deepEqual(drafts.read("user-a", "upload", () => null).ids, ["doc1"]);
  drafts.clear();
  assert.equal(
    drafts.read("user-a", "upload", () => null),
    null,
  );
});

test("desktop document links require an explicit HTTP management base and retain encoded context", () => {
  assert.equal(
    mobileState.desktopDocumentUrl(undefined, "kb1", "doc1"),
    undefined,
  );
  assert.equal(
    mobileState.desktopDocumentUrl("javascript:alert(1)", "kb1", "doc1"),
    undefined,
  );
  assert.equal(
    mobileState.desktopDocumentUrl(
      "https://user:secret@example.test",
      "kb1",
      "doc1",
    ),
    undefined,
  );
  assert.equal(
    mobileState.desktopDocumentUrl(
      "https://console.example.test/",
      "kb/中文",
      "doc/1",
    ),
    "https://console.example.test/knowledge/kb%2F%E4%B8%AD%E6%96%87/documents/doc%2F1/chunks",
  );
});
