import { useEffect, useRef, useState } from "react";
import {
  Alert,
  Checkbox,
  Button,
  Form,
  Input,
  InputNumber,
  Modal,
  Select,
  Tabs,
  Spin,
  Typography,
} from "antd";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router";
import { knowledgeApi, type KnowledgeDocument } from "@/api/knowledge";
import { knowledgeManagement } from "@/api/knowledgeManagement";
import { parseApiError } from "@/api/client";
import MetadataValuesEditor from "./MetadataValuesEditor";
import MetadataTemplateEditor from "./MetadataTemplateEditor";
import { parseMetadataDefinition, metadataFields } from "./metadata";
import {
  buildDocumentPatch,
  advanceDocumentDraft,
  rebaseDocumentMetadataPatch,
  documentPatchReadbackMatches,
  documentTemplateReadbackMatches,
  buildDocumentRetryPatch,
  sameDocumentMode,
  UnconfirmedDocumentModeChange,
  documentMetadataValues,
  documentParsers,
  editableDocumentConfig,
  requiredDocumentParser,
  sameDocumentValue,
  type DocumentEditorValues,
} from "./documentSettings";
import { useDocumentWorkflowText } from "./documentWorkflowText";
import { usePrimaryButtonStyle } from "@/components/PrimaryButton";

interface EditorProps {
  datasetId: string;
  document: KnowledgeDocument;
  onClose: () => void;
  onSaved: () => void;
  onReparse?: (document: KnowledgeDocument) => void;
}
export function KnowledgeDocumentEditor(props: EditorProps) {
  const text = useDocumentWorkflowText();
  const [loaded, setLoaded] = useState<KnowledgeDocument>();
  const [error, setError] = useState("");
  const [attempt, setAttempt] = useState(0);
  useEffect(() => {
    let alive = true;
    knowledgeApi.documents
      .get(props.datasetId, props.document.id)
      .then((doc) => {
        if (alive) setLoaded(doc);
      })
      .catch((err: unknown) => {
        if (alive) setError(parseApiError(err));
      });
    return () => {
      alive = false;
    };
  }, [props.datasetId, props.document.id, attempt]);
  if (loaded)
    return <DocumentEditorForm key={loaded.id} {...props} document={loaded} />;
  return (
    <Modal
      open
      title={props.document.name}
      onCancel={props.onClose}
      footer={null}
    >
      {error ? (
        <Alert
          type="error"
          showIcon
          title={text("load")}
          description={error}
          action={
            <Button
              onClick={() => {
                setError("");
                setAttempt((n) => n + 1);
              }}
            >
              {text("retry")}
            </Button>
          }
        />
      ) : (
        <Spin description={text("loading")}>
          <div style={{ height: 80 }} />
        </Spin>
      )}
    </Modal>
  );
}
function DocumentEditorForm({
  datasetId,
  document: doc,
  onClose,
  onSaved,
  onReparse,
}: EditorProps) {
  const { t } = useTranslation();
  const style = usePrimaryButtonStyle();
  const [form] = Form.useForm<DocumentEditorValues>();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const text = useDocumentWorkflowText();
  const [baseline, setBaseline] = useState(doc);
  const templateBaseline = useRef(
    JSON.stringify(doc.parser_config.metadata ?? []),
  );
  const saveLock = useRef(false);
  const pendingGeneral = useRef<Record<string, unknown> | undefined>(undefined);
  const pendingTemplate = useRef<
    ReturnType<typeof parseMetadataDefinition> | undefined
  >(undefined);
  const metadata = documentMetadataValues(doc);
  const config = doc.parser_config;
  const parser = Form.useWatch("parser_id", form) as string | undefined;
  const requiredParser = requiredDocumentParser(doc);
  const leavePipeline = Form.useWatch("leave_pipeline", form);
  const save = async (reparse = false) => {
    if (saveLock.current) return;
    saveLock.current = true;
    let changed = false;
    try {
      const values = await form.validateFields();
      const patch = buildDocumentPatch(baseline, values);
      const template = parseMetadataDefinition(values.template);
      const templateChanged =
        pendingTemplate.current !== undefined ||
        !sameDocumentValue(
          JSON.parse(values.template),
          JSON.parse(templateBaseline.current),
        );
      setBusy(true);
      setError("");
      const failures: string[] = [];
      const verified: string[] = [];
      let saved = baseline;
      if (Object.keys(patch).length || pendingGeneral.current) {
        try {
          const latest = await knowledgeApi.documents.get(datasetId, doc.id);
          const intended = buildDocumentRetryPatch(
            baseline,
            values,
            latest,
            pendingGeneral.current,
          );
          const requestPatch = rebaseDocumentMetadataPatch(
            baseline,
            intended,
            latest,
            pendingGeneral.current,
          );
          let readback = latest;
          // A failed acknowledgement can follow a successful write. Verify the
          // requested fields before retrying, never compare untouched form values
          // with a newer server snapshot to decide what to send.
          if (!documentPatchReadbackMatches(latest, requestPatch)) {
            changed = true;
            pendingGeneral.current = intended;
            await knowledgeApi.documents.update(
              datasetId,
              doc.id,
              requestPatch,
            );
            readback = await knowledgeApi.documents.get(datasetId, doc.id);
          }
          if (
            !documentPatchReadbackMatches(readback, requestPatch) ||
            (!("parser_id" in requestPatch) &&
              !sameDocumentMode(readback, latest))
          )
            throw new Error(t("knowledge.manage.readbackFailed"));
          verified.push(text("parserPhase"));
          saved = readback;
          setBaseline((current) => advanceDocumentDraft(current, intended));
          pendingGeneral.current = undefined;
        } catch (err) {
          failures.push(
            `${text("parserPhase")}: ${err instanceof UnconfirmedDocumentModeChange ? text("modeRecovery") : parseApiError(err)}`,
          );
        }
      }
      // Dirty intent is relative to the form's original/confirmed template,
      // not to unrelated metadata returned by the general document readback.
      if (templateChanged) {
        try {
          const latest = await knowledgeApi.documents.get(datasetId, doc.id);
          let readback = latest;
          if (
            !documentTemplateReadbackMatches(
              latest.parser_config.metadata,
              template,
            )
          ) {
            if (
              !documentTemplateReadbackMatches(
                latest.parser_config.metadata,
                parseMetadataDefinition(templateBaseline.current),
              ) &&
              !(
                pendingTemplate.current &&
                documentTemplateReadbackMatches(
                  latest.parser_config.metadata,
                  pendingTemplate.current,
                )
              )
            )
              throw new Error(text("templateConflict"));
            changed = true;
            pendingTemplate.current = template;
            await knowledgeManagement.documentMetadataConfig(
              datasetId,
              doc.id,
              template,
            );
            readback = await knowledgeApi.documents.get(datasetId, doc.id);
          }
          if (
            !documentTemplateReadbackMatches(
              readback.parser_config.metadata,
              template,
            ) ||
            !sameDocumentMode(readback, latest)
          )
            throw new Error(t("knowledge.manage.readbackFailed"));
          verified.push(text("templatePhase"));
          saved = readback;
          templateBaseline.current = values.template;
          pendingTemplate.current = undefined;
          setBaseline((current) => advanceDocumentDraft(current, {}, template));
        } catch (err) {
          failures.push(`${text("templatePhase")}: ${parseApiError(err)}`);
        }
      }
      if (failures.length) {
        setError(
          [
            ...failures,
            ...verified.map((phase) => `${phase}: ${text("verified")}`),
          ].join("\n"),
        );
        return;
      }
      onClose();
      if (reparse) onReparse?.(saved);
    } catch (err) {
      setError(parseApiError(err));
    } finally {
      if (changed) onSaved();
      setBusy(false);
      saveLock.current = false;
    }
  };
  return (
    <Modal
      open
      title={t("knowledge.manage.documentSettings", { name: doc.name })}
      onCancel={() => {
        if (!busy) onClose();
      }}
      onOk={() => save()}
      footer={(_, { OkBtn, CancelBtn }) => (
        <>
          <CancelBtn />
          {onReparse && (
            <Button disabled={busy} onClick={() => void save(true)}>
              {t("knowledge.manage.saveAndReparse")}
            </Button>
          )}
          <OkBtn />
        </>
      )}
      width={760}
      mask={{ closable: !busy }}
      closable={!busy}
      cancelButtonProps={{ disabled: busy }}
      confirmLoading={busy}
      okButtonProps={{ className: style.root }}
      okText={t("knowledge.docs.saveBtn")}
    >
      {error && (
        <Alert
          type="error"
          showIcon
          title={error}
          style={{ whiteSpace: "pre-line", marginBottom: 12 }}
        />
      )}
      <Alert
        type="info"
        title={t("knowledge.manage.reparseHint")}
        style={{ marginBottom: 12 }}
      />
      <Form
        form={form}
        layout="vertical"
        initialValues={{
          parser_id: doc.parser_id,
          enable_metadata:
            typeof config.enable_metadata === "boolean"
              ? config.enable_metadata
              : undefined,
          chunk_token_num: config.chunk_token_num,
          delimiter: config.delimiter,
          metadata: JSON.stringify(metadata, null, 2),
          template: JSON.stringify(config.metadata ?? [], null, 2),
          advanced: JSON.stringify(
            Object.fromEntries(
              Object.entries(editableDocumentConfig(config)).filter(
                ([key, value]) =>
                  !["chunk_token_num", "delimiter", "metadata"].includes(key) &&
                  !(key === "enable_metadata" && typeof value === "boolean"),
              ),
            ),
            null,
            2,
          ),
        }}
        disabled={busy}
      >
        <Tabs
          items={[
            {
              key: "parser",
              label: t("knowledge.manage.parserSettings"),
              forceRender: true,
              children: (
                <>
                  {!!doc.pipeline_id && (
                    <Alert
                      type={
                        leavePipeline || (parser && parser !== doc.parser_id)
                          ? "warning"
                          : "info"
                      }
                      showIcon
                      title={text("pipeline")}
                      style={{ marginBottom: 12 }}
                    />
                  )}
                  {!!doc.pipeline_id && (
                    <Form.Item name="leave_pipeline" valuePropName="checked">
                      <Checkbox>{text("switchToBuiltin")}</Checkbox>
                    </Form.Item>
                  )}
                  {requiredParser && (
                    <Typography.Text type="secondary">
                      {text("source")}
                      {requiredParser}
                    </Typography.Text>
                  )}
                  <Form.Item
                    name="parser_id"
                    label={t("knowledge.form.parserLabel")}
                  >
                    <Select
                      options={Array.from(
                        new Set([doc.parser_id, ...documentParsers]),
                      ).map((value) => ({
                        value,
                        label: value,
                        disabled:
                          value !== doc.parser_id &&
                          !!requiredParser &&
                          requiredParser !== value,
                      }))}
                    />
                  </Form.Item>
                  <Form.Item
                    name="chunk_token_num"
                    label={t("knowledge.form.chunkToken")}
                  >
                    <InputNumber min={1} max={8192} style={{ width: "100%" }} />
                  </Form.Item>
                  <Form.Item
                    name="delimiter"
                    label={t("knowledge.form.delimiter")}
                  >
                    <Input />
                  </Form.Item>
                </>
              ),
            },
            {
              key: "metadata",
              label: t("knowledge.manage.metadataManagement"),
              forceRender: true,
              children: (
                <>
                  {config.enable_metadata !== undefined &&
                  typeof config.enable_metadata !== "boolean" ? (
                    <Alert
                      type="warning"
                      title={text("metadataEnabledInvalid")}
                    />
                  ) : (
                    <Form.Item
                      name="enable_metadata"
                      valuePropName="checked"
                      extra={text("metadataEnabledHint")}
                    >
                      <Checkbox>{text("metadataEnabled")}</Checkbox>
                    </Form.Item>
                  )}
                  <Form.Item
                    name="metadata"
                    label={t("knowledge.manage.metadataValues")}
                    extra={t("knowledge.manage.metadataExample")}
                  >
                    <MetadataValuesEditor
                      disabled={busy}
                      fields={metadataFields(
                        config.metadata as ReturnType<
                          typeof parseMetadataDefinition
                        >,
                      )}
                    />
                  </Form.Item>
                  <Form.Item
                    name="template"
                    label={t("knowledge.manage.metadataTemplate")}
                    extra={t("knowledge.manage.templateHint")}
                    rules={[
                      {
                        validator: (_, value: string) => {
                          try {
                            parseMetadataDefinition(value);
                            return Promise.resolve();
                          } catch {
                            return Promise.reject(
                              new Error(
                                t("knowledge.manage.metadataDefinitionInvalid"),
                              ),
                            );
                          }
                        },
                      },
                    ]}
                  >
                    <MetadataTemplateEditor disabled={busy} />
                  </Form.Item>
                </>
              ),
            },
            {
              key: "advanced",
              label: t("knowledge.form.advJson"),
              forceRender: true,
              children: (
                <>
                  <Form.Item
                    name="advanced"
                    label={t("knowledge.form.advJson")}
                    extra={text("advanced")}
                  >
                    <Input.TextArea rows={5} />
                  </Form.Item>
                </>
              ),
            },
          ]}
        />
      </Form>
    </Modal>
  );
}

export function KnowledgeDocumentCreate({
  datasetId,
  onClose,
  onSaved,
}: {
  datasetId: string;
  onClose: () => void;
  onSaved: () => void;
}) {
  const { t } = useTranslation();
  const style = usePrimaryButtonStyle();
  const navigate = useNavigate();
  const [form] = Form.useForm<{
    mode: "web" | "empty";
    name: string;
    url?: string;
  }>();
  const mode = Form.useWatch("mode", form) as "web" | "empty" | undefined;
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const create = async () => {
    try {
      const values = await form.validateFields();
      setBusy(true);
      setError("");
      const doc = await knowledgeManagement.createDocument(
        datasetId,
        values.mode,
        values.name.trim(),
        values.url?.trim(),
      );
      onSaved();
      onClose();
      if (values.mode === "empty")
        await navigate(`/knowledge/${datasetId}/documents/${doc.id}/chunks`);
    } catch (err) {
      setError(parseApiError(err));
    } finally {
      setBusy(false);
    }
  };
  return (
    <Modal
      open
      title={t("knowledge.manage.createDocument")}
      onCancel={onClose}
      onOk={create}
      okText={t("knowledge.form.createBtn")}
      cancelText={t("common.cancel")}
      confirmLoading={busy}
      okButtonProps={{ className: style.root }}
    >
      {error && <Alert type="error" title={error} />}
      <Form form={form} layout="vertical" initialValues={{ mode: "web" }}>
        <Form.Item name="mode" label={t("knowledge.manage.source")}>
          <Select
            options={[
              { value: "web", label: t("knowledge.manage.webDocument") },
              { value: "empty", label: t("knowledge.manage.emptyDocument") },
            ]}
          />
        </Form.Item>
        <Form.Item
          name="name"
          label={t("knowledge.manage.name")}
          rules={[{ required: true, whitespace: true }]}
        >
          <Input />
        </Form.Item>
        {mode !== "empty" && (
          <Form.Item
            name="url"
            label="URL"
            rules={[{ required: true }, { type: "url" }]}
          >
            <Input placeholder="https://example.com" />
          </Form.Item>
        )}
        <Alert type="info" title={t("knowledge.manage.creationHint")} />
      </Form>
    </Modal>
  );
}
