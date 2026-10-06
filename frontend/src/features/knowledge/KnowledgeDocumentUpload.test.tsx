import { AxiosError } from "axios";
import { afterEach, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ConfigProvider, theme } from "antd";
import i18next from "@/i18n";
import { normalizeDocument } from "@/api/knowledge";
import KnowledgeDocumentUpload from "./KnowledgeDocumentUpload";
import KnowledgeIngestModal from "./KnowledgeIngestModal";
afterEach(async () => {
  await i18next.changeLanguage("zh");
});
it("retries parsing with the uploaded ID and never uploads accepted files twice", async () => {
  const upload = vi.fn(async (file: File) =>
    normalizeDocument({ id: file.name, name: file.name }),
  );
  const parse = vi
    .fn()
    .mockResolvedValueOnce(true)
    .mockRejectedValueOnce(new Error("parsing unavailable"))
    .mockResolvedValueOnce(true);
  const close = vi.fn();
  const user = userEvent.setup();
  render(
    <ConfigProvider>
      <KnowledgeDocumentUpload
        onUpload={upload}
        onParse={parse}
        onClose={close}
      />
    </ConfigProvider>,
  );
  await user.upload(
    document.querySelector('input[type="file"]') as HTMLInputElement,
    [new File(["a"], "a.txt"), new File(["b"], "b.txt")],
  );
  await user.click(screen.getByRole("button", { name: "开始上传" }));
  await screen.findByText("parsing unavailable");
  expect(upload).toHaveBeenCalledTimes(2);
  expect(parse).toHaveBeenCalledTimes(2);
  expect(close).not.toHaveBeenCalled();
  await user.click(screen.getAllByRole("button", { name: "重试失败项" })[0]);
  await waitFor(() => {
    expect(screen.getAllByText("解析已受理，等待 worker")).toHaveLength(2);
  });
  expect(upload).toHaveBeenCalledTimes(2);
  expect(parse).toHaveBeenLastCalledWith(
    expect.objectContaining({ id: "b.txt" }),
  );
});
it("keeps unknown upload results out of automatic retry until the user checks the list", async () => {
  const upload = vi
    .fn()
    .mockRejectedValueOnce(new Error("connection lost"))
    .mockResolvedValueOnce(normalizeDocument({ id: "a" }));
  const parse = vi.fn().mockResolvedValue(true);
  const user = userEvent.setup();
  render(
    <ConfigProvider>
      <KnowledgeDocumentUpload
        onUpload={upload}
        onParse={parse}
        onClose={vi.fn()}
      />
    </ConfigProvider>,
  );
  await user.upload(
    document.querySelector('input[type="file"]') as HTMLInputElement,
    new File(["a"], "a.txt"),
  );
  await user.click(screen.getByRole("button", { name: "开始上传" }));
  await screen.findByText("上传结果待核对");
  expect(screen.getByRole("button", { name: "重试失败项" })).toBeDisabled();
  expect(upload).toHaveBeenCalledTimes(1);
  expect(parse).not.toHaveBeenCalled();
  await user.click(screen.getByRole("button", { name: "已核对，重新上传" }));
  await user.click(screen.getByRole("button", { name: "开始上传" }));
  await screen.findByText("解析已受理，等待 worker");
  expect(upload).toHaveBeenCalledTimes(2);
});
it("supports English, dark theme and keyboard submission while upload-only stays unparsed", async () => {
  await i18next.changeLanguage("en");
  const upload = vi.fn().mockResolvedValue(normalizeDocument({ id: "d" }));
  const parse = vi.fn();
  const user = userEvent.setup();
  render(
    <ConfigProvider theme={{ algorithm: theme.darkAlgorithm }}>
      <KnowledgeDocumentUpload
        onUpload={upload}
        onParse={parse}
        onClose={vi.fn()}
      />
    </ConfigProvider>,
  );
  await user.upload(
    document.querySelector('input[type="file"]') as HTMLInputElement,
    new File(["a"], "a.txt"),
  );
  await user.click(screen.getByRole("switch"));
  screen.getByRole("button", { name: /Start Upload/i }).focus();
  await user.keyboard("{Enter}");
  await screen.findByText("Uploaded; parsing not submitted");
  expect(parse).not.toHaveBeenCalled();
});
it("keeps accepted documents out of a partial batch retry", async () => {
  const submit = vi
    .fn()
    .mockResolvedValueOnce(true)
    .mockRejectedValueOnce(new Error("denied"))
    .mockResolvedValueOnce(true);
  const close = vi.fn();
  const user = userEvent.setup();
  render(
    <ConfigProvider>
      <KnowledgeIngestModal
        docIds={["a", "b"]}
        names={{ a: "a.txt", b: "b.txt" }}
        onSubmit={submit}
        onClose={close}
      />
    </ConfigProvider>,
  );
  await user.click(screen.getByRole("button", { name: "提交解析请求" }));
  await screen.findByText("denied");
  expect(close).not.toHaveBeenCalled();
  expect(screen.getByText("a.txt")).toBeInTheDocument();
  expect(screen.getByText("b.txt")).toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "重试失败项" }));
  await waitFor(() => {
    expect(close).toHaveBeenCalledTimes(1);
  });
  expect(submit).toHaveBeenCalledTimes(3);
  expect(submit).toHaveBeenLastCalledWith({
    doc_ids: ["b"],
    run: 1,
    delete: false,
    apply_kb: false,
  });
});
it("has complete Chinese and English resources for long workflow hints", () => {
  for (const lng of ["zh", "en"])
    for (const key of [
      "pipeline",
      "advanced",
      "hint",
      "missing",
      "allAccepted",
      "verifyUpload",
      "uploadSummary",
    ]) {
      expect(i18next.exists(`knowledge.documentWorkflow.${key}`, { lng })).toBe(
        true,
      );
    }
});

it("retries only a rejected upload within a partially successful file batch", async () => {
  const rejection = Object.assign(new AxiosError("file rejected"), {
    response: { status: 400, data: { error: "file rejected" } },
  });
  const upload = vi
    .fn()
    .mockRejectedValueOnce(rejection)
    .mockResolvedValueOnce(normalizeDocument({ id: "b" }))
    .mockResolvedValueOnce(normalizeDocument({ id: "a" }));
  const parse = vi.fn().mockResolvedValue(true);
  const user = userEvent.setup();
  render(
    <ConfigProvider>
      <KnowledgeDocumentUpload
        onUpload={upload}
        onParse={parse}
        onClose={vi.fn()}
      />
    </ConfigProvider>,
  );
  await user.upload(
    document.querySelector('input[type="file"]') as HTMLInputElement,
    [new File(["a"], "a.txt"), new File(["b"], "b.txt")],
  );
  await user.click(screen.getByRole("button", { name: "开始上传" }));
  await screen.findByText("file rejected");
  await screen.findByText("解析已受理，等待 worker");
  await user.click(screen.getAllByRole("button", { name: "重试失败项" })[0]);
  await waitFor(() => {
    expect(screen.getAllByText("解析已受理，等待 worker")).toHaveLength(2);
  });
  expect(upload).toHaveBeenCalledTimes(3);
  expect(upload).toHaveBeenLastCalledWith(
    expect.objectContaining({ name: "a.txt" }),
  );
  expect(parse).toHaveBeenCalledTimes(2);
});
