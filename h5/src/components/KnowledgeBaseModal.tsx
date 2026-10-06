import React, { useState, useEffect } from "react";
import { KnowledgeFolder } from "../types";
import {
  KnowledgeConfigurationPendingError,
  listEmbeddingModels,
  MOBILE_PARSERS,
  type DatasetCreationConfiguration,
} from "../api/knowledge";
import { KnowledgeDialog } from "./KnowledgeDialog";
import { useKnowledgeDraft } from "./useKnowledgeDraft";

export interface KnowledgeFolderFormInput {
  existingId?: string;
  name: string;
  description: string;
  parseMethod: string;
  category: "mine" | "team";
  configuration?: DatasetCreationConfiguration;
}

interface KnowledgeBaseModalProps {
  isOpen: boolean;
  mode: "create" | "edit";
  folder: KnowledgeFolder | null;
  onClose: () => void;
  onSubmit: (data: KnowledgeFolderFormInput) => Promise<void>;
}

export const KnowledgeBaseModal: React.FC<KnowledgeBaseModalProps> = ({
  isOpen,
  mode,
  folder,
  onClose,
  onSubmit,
}) => {
  const [draft, setDraft, clearDraft] = useKnowledgeDraft(
    mode === "create" ? "create-library" : `edit-library:${folder?.id || ""}`,
    () => ({
      name: mode === "edit" ? folder?.name || "" : "",
      description: mode === "edit" ? folder?.description || "" : "",
      existingId: undefined as string | undefined,
      model: "",
      parser: "naive",
    }),
  );
  const { name, description, existingId, model, parser } = draft;
  const setName = (name: string) =>
    setDraft((previous) => ({ ...previous, name }));
  const setDescription = (description: string) =>
    setDraft((previous) => ({ ...previous, description }));
  const setExistingId = (existingId: string) =>
    setDraft((previous) => ({ ...previous, existingId }));
  const setModel = (model: string | ((current: string) => string)) =>
    setDraft((previous) => ({
      ...previous,
      model: typeof model === "function" ? model(previous.model) : model,
    }));
  const setParser = (parser: string) =>
    setDraft((previous) => ({ ...previous, parser }));
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [models, setModels] = useState<
    { fullId: string; name: string; factory: string }[]
  >([]);
  const [modelLoading, setModelLoading] = useState(false);
  const [modelError, setModelError] = useState("");
  const [retry, setRetry] = useState(0);
  useEffect(() => {
    if (!isOpen || mode !== "create") return;
    const controller = new AbortController();
    setModelLoading(true);
    setModelError("");
    setModels([]);
    listEmbeddingModels(controller.signal)
      .then((data) => {
        if (controller.signal.aborted) return;
        setModels(data);
        setModel((current) => current || data[0]?.fullId || "");
      })
      .catch((err) => {
        if (!controller.signal.aborted) setModelError(err.message);
      })
      .finally(() => {
        if (!controller.signal.aborted) setModelLoading(false);
      });
    return () => controller.abort();
  }, [isOpen, mode, retry]);
  if (!isOpen) return null;
  const unavailable =
    mode === "create" &&
    (modelLoading ||
      !!modelError ||
      !models.some((item) => item.fullId === model));
  const handleSubmit = async (event: React.FormEvent) => {
    event.preventDefault();
    if (!name.trim() || busy || unavailable) return;
    setBusy(true);
    setError("");
    try {
      await onSubmit({
        existingId,
        name: name.trim(),
        description: description.trim(),
        parseMethod:
          mode === "create" ? parser : folder?.parseMethod || "naive",
        category: folder?.category || "mine",
        ...(mode === "create"
          ? { configuration: { embeddingModel: model, parseMethod: parser } }
          : {}),
      });
      clearDraft();
      setError("");
      onClose();
    } catch (err) {
      if (err instanceof KnowledgeConfigurationPendingError)
        setExistingId(err.folder.id);
      setError(err instanceof Error ? err.message : "保存失败，请重试");
    } finally {
      setBusy(false);
    }
  };
  return (
    <KnowledgeDialog
      title={mode === "create" ? "新建知识库" : "编辑知识库"}
      busy={busy}
      onClose={onClose}
    >
      <form onSubmit={handleSubmit} className="p-4 space-y-4 text-sm">
        <p className="text-xs text-gray-500">
          关闭会保留本次草稿，重新打开可继续；退出登录或刷新页面后清除。
        </p>
        {error && (
          <p role="alert" className="text-red-700">
            {error}
          </p>
        )}
        {existingId && (
          <p className="text-amber-700">
            知识库已创建，重试保存配置将继续使用已有知识库。关闭后可从“新建知识库”继续，避免重复创建。
          </p>
        )}
        <label className="block">
          知识库名称
          <input
            required
            disabled={busy}
            value={name}
            onChange={(event) => setName(event.target.value)}
            placeholder="例如：产品手册库"
            className="mt-1 w-full border border-gray-200 rounded-xl p-3 bg-white"
          />
        </label>
        <label className="block">
          描述说明
          <textarea
            disabled={busy}
            rows={2}
            value={description}
            onChange={(event) => setDescription(event.target.value)}
            placeholder="说明收录范围…"
            className="mt-1 w-full border border-gray-200 rounded-xl p-3 bg-white"
          />
        </label>
        {mode === "create" ? (
          <>
            <label className="block">
              嵌入模型
              <select
                disabled={busy || modelLoading || !!modelError}
                value={model}
                onChange={(event) => setModel(event.target.value)}
                className="mt-1 w-full border border-gray-200 rounded-xl p-2 bg-white"
              >
                <option value="">
                  {modelLoading ? "加载可用模型…" : "选择可用模型"}
                </option>
                {models.map((item) => (
                  <option key={item.fullId} value={item.fullId}>
                    {item.name} · {item.factory}
                  </option>
                ))}
              </select>
            </label>
            {modelError && (
              <p role="alert" className="text-red-700">
                {modelError}
              </p>
            )}
            {!modelLoading && (modelError || !models.length) && (
              <div>
                <p className="text-gray-500">
                  请先在桌面配置可用的嵌入模型，再重试。
                </p>
                <button
                  type="button"
                  onClick={() => setRetry((value) => value + 1)}
                  className="text-emerald-700"
                >
                  重试模型列表
                </button>
              </div>
            )}
            <label className="block">
              资料解析方式
              <select
                disabled={busy}
                value={parser}
                onChange={(event) => setParser(event.target.value)}
                className="mt-1 w-full border border-gray-200 rounded-xl p-2 bg-white"
              >
                {MOBILE_PARSERS.map((item) => (
                  <option key={item.id} value={item.id}>
                    {item.label}
                  </option>
                ))}
              </select>
            </label>
            <p className="text-gray-500">
              {MOBILE_PARSERS.find((item) => item.id === parser)?.description}
              。创建后可直接上传资料；更多参数可在桌面设置。
            </p>
          </>
        ) : (
          <p className="text-gray-500">
            本次仅修改名称与说明。当前解析方式：
            {MOBILE_PARSERS.find((item) => item.id === folder?.parseMethod)
              ?.label || folder?.parseMethod}
            ；模型和详细解析参数可在桌面设置。
          </p>
        )}
        <div className="flex justify-end gap-2">
          <button
            type="button"
            disabled={busy}
            onClick={onClose}
            className="px-4 border border-gray-200 rounded-xl"
          >
            取消
          </button>
          <button
            type="submit"
            disabled={busy || !name.trim() || unavailable}
            className="px-4 rounded-xl bg-neutral-900 text-white"
          >
            {busy
              ? "保存中…"
              : existingId
                ? "重试保存配置"
                : mode === "create"
                  ? "创建知识库"
                  : "保存修改"}
          </button>
        </div>
      </form>
    </KnowledgeDialog>
  );
};
