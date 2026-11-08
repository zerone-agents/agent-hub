import KnowledgeSourceOAuth from "./KnowledgeSourceOAuth";
import { useEffect, useState } from "react";
import {
  Alert,
  Button,
  Descriptions,
  Form,
  Input,
  InputNumber,
  Modal,
  Popconfirm,
  Select,
  Space,
  Switch,
  Table,
  Tag,
  Tooltip,
  Typography,
} from "antd";
import { useTranslation } from "react-i18next";
import {
  useKnowledgeAction,
  useKnowledgeResource,
} from "@/queries/useKnowledgeManagement";
import {
  containsKnowledgeFields,
  knowledgeRequest,
  parseObjectJSON,
  type KnowledgeObject,
} from "@/api/knowledgeManagement";
import { parseApiError } from "@/api/client";
import PrimaryButton, {
  usePrimaryButtonStyle,
} from "@/components/PrimaryButton";

import {
  CONNECTOR_SOURCES,
  CONNECTOR_REQUIRED_FIELDS,
  CONNECTOR_EXAMPLES,
  connectorConfigIssue,
  isConnectorSchedule,
  mergeConnectorConfig,
  supportsDeletionSync,
  writableConnectorConfig,
} from "./connectorConfig";
import { useConnectorTranslation } from "./useConnectorTranslation";
const BLOB_SYNC_SOURCES = new Set([
  "s3",
  "r2",
  "google_cloud_storage",
  "oci_storage",
]);
function blobSyncScope(config: KnowledgeObject, source: string) {
  const credentials = config.credentials as KnowledgeObject | undefined;
  const prefix = typeof config.prefix === "string" ? config.prefix : "";
  return {
    bucket_type: config.bucket_type ?? source,
    bucket_name:
      typeof config.bucket_name === "string"
        ? config.bucket_name.trim()
        : config.bucket_name,
    prefix: prefix && !prefix.endsWith("/") ? `${prefix}/` : prefix,
    allow_images: config.allow_images ?? false,
    credentials: Object.fromEntries(
      ["endpoint_url", "account_id", "namespace", "region", "aws_role_arn"].map(
        (key) => [key, credentials?.[key]],
      ),
    ),
  };
}
function DeletionSyncStatus({ id, row }: { id: string; row: ConnectorRow }) {
  const { t } = useTranslation();
  const supported = supportsDeletionSync(row.source);
  const query = useKnowledgeResource<KnowledgeObject>(
    supported ? id : "",
    `sources/${encodeURIComponent(row.id)}`,
  );
  if (!supported)
    return <Tag>{t("knowledge.manage.deletionSyncUnsupported")}</Tag>;
  if (query.error || !query.data)
    return (
      <Tooltip title={query.error ? parseApiError(query.error) : undefined}>
        <Tag>
          {t(
            query.isLoading
              ? "knowledge.manage.readingConfig"
              : "knowledge.manage.configUnconfirmed",
          )}
        </Tag>
      </Tooltip>
    );
  const config = query.data.config;
  if (
    query.data.id !== row.id ||
    !config ||
    typeof config !== "object" ||
    Array.isArray(config)
  )
    return <Tag color="warning">{t("knowledge.manage.configUnconfirmed")}</Tag>;
  if (!supportsDeletionSync(row.source, config as KnowledgeObject))
    return <Tag>{t("knowledge.manage.deletionSyncUnsupported")}</Tag>;
  const value = (config as KnowledgeObject).sync_deleted_files;
  if (value !== undefined && typeof value !== "boolean")
    return <Tag color="warning">{t("knowledge.manage.configUnconfirmed")}</Tag>;
  return (
    <Tag color={value === true ? "success" : "default"}>
      {t(value === true ? "knowledge.docs.enabled" : "knowledge.docs.disabled")}
    </Tag>
  );
}

function text(value: unknown): string {
  return typeof value === "string" || typeof value === "number"
    ? String(value)
    : "-";
}
interface ConnectorRow extends KnowledgeObject {
  id: string;
  name: string;
  source: string;
  auto_parse: string;
  status: string;
}
export default function KnowledgeConnectorsPanel({ id }: { id: string }) {
  const { t } = useTranslation();
  const { t: tc } = useConnectorTranslation();
  const linked = useKnowledgeResource<ConnectorRow[]>(id, "connectors");
  const sources = useKnowledgeResource<ConnectorRow[]>(id, "sources");
  const action = useKnowledgeAction(id);
  const [sourceID, setSourceID] = useState<string>();
  const [autoParse, setAutoParse] = useState(true);
  const [editor, setEditor] = useState<string | null>(null);
  const [logSource, setLogSource] = useState("");
  const [logPage, setLogPage] = useState(1);
  const [logDetail, setLogDetail] = useState<KnowledgeObject | null>(null);
  const logs = useKnowledgeResource<{ total: number; logs: KnowledgeObject[] }>(
    logSource ? id : "",
    `sources/${encodeURIComponent(logSource)}/logs`,
    { page: logPage, page_size: 15 },
  );
  return (
    <Space orientation="vertical" size="middle" style={{ width: "100%" }}>
      {(linked.error ?? sources.error) && (
        <Alert
          type="error"
          title={parseApiError(linked.error ?? sources.error)}
        />
      )}
      <Space wrap>
        <Select
          value={sourceID}
          onChange={setSourceID}
          placeholder={t("knowledge.manage.selectConnector")}
          style={{ minWidth: 240 }}
          options={(sources.data ?? []).map((source) => ({
            value: source.id,
            label: `${source.name} (${source.source})`,
          }))}
        />
        <Switch
          checked={autoParse}
          onChange={setAutoParse}
          checkedChildren={t("knowledge.manage.autoParse")}
          unCheckedChildren={t("knowledge.docs.uploadOnly")}
        />
        <PrimaryButton
          disabled={!sourceID}
          loading={action.isPending}
          onClick={() => {
            if (sourceID)
              action.mutate({
                method: "put",
                resource: `connectors/${encodeURIComponent(sourceID)}`,
                data: { auto_parse: autoParse },
              });
          }}
        >
          {t("knowledge.manage.linkConnector")}
        </PrimaryButton>
        <Button
          onClick={() => {
            setEditor("");
          }}
        >
          {t("knowledge.manage.createConnector")}
        </Button>
        <Button
          onClick={() => {
            void linked.refetch();
            void sources.refetch();
          }}
        >
          {t("knowledge.docs.refresh")}
        </Button>
      </Space>
      <Table
        rowKey="id"
        size="small"
        scroll={{ x: 900 }}
        dataSource={linked.data ?? []}
        loading={linked.isFetching}
        columns={[
          { title: t("knowledge.manage.name"), dataIndex: "name" },
          { title: t("knowledge.manage.source"), dataIndex: "source" },
          {
            title: tc("connectorStatus"),
            render: (_, row) =>
              tc(`status.${row.status}`, { defaultValue: text(row.status) }),
          },
          {
            title: t("knowledge.manage.deletionSync"),
            render: (_, row) => <DeletionSyncStatus id={id} row={row} />,
          },
          {
            title: t("knowledge.manage.autoParse"),
            render: (_, row) => (
              <Switch
                checked={row.auto_parse === "1"}
                disabled={action.isPending}
                onChange={(value) => {
                  action.mutate({
                    method: "put",
                    resource: `connectors/${encodeURIComponent(row.id)}`,
                    data: { auto_parse: value },
                  });
                }}
              />
            ),
          },
          {
            title: t("knowledge.docs.actions"),
            render: (_, row) => (
              <Space wrap>
                <Button
                  type="link"
                  onClick={() => {
                    setEditor(row.id);
                  }}
                >
                  {t("knowledge.manage.settings")}
                </Button>
                <Button
                  type="link"
                  onClick={() => {
                    setLogSource(row.id);
                    setLogPage(1);
                  }}
                >
                  {t("knowledge.manage.syncLogs")}
                </Button>
                <Button
                  type="link"
                  onClick={() => {
                    action.mutate({
                      method: "post",
                      resource: `sources/${encodeURIComponent(row.id)}/resume`,
                      data: { resume: false },
                    });
                  }}
                >
                  {t("knowledge.manage.pauseSync")}
                </Button>
                <Button
                  type="link"
                  onClick={() => {
                    action.mutate({
                      method: "post",
                      resource: `sources/${encodeURIComponent(row.id)}/resume`,
                      data: { resume: true },
                    });
                  }}
                >
                  {t("knowledge.manage.resumeSync")}
                </Button>
                <Popconfirm
                  title={t("knowledge.manage.rebuildConfirm")}
                  onConfirm={() => {
                    action.mutate({
                      method: "post",
                      resource: `sources/${encodeURIComponent(row.id)}/rebuild`,
                    });
                  }}
                >
                  <Button type="link">
                    {t("knowledge.manage.rebuildSync")}
                  </Button>
                </Popconfirm>
                <Popconfirm
                  title={t("knowledge.manage.unlinkConfirm")}
                  onConfirm={() => {
                    action.mutate({
                      method: "delete",
                      resource: `connectors/${encodeURIComponent(row.id)}`,
                    });
                  }}
                >
                  <Button type="link" danger>
                    {t("knowledge.manage.unlinkConnector")}
                  </Button>
                </Popconfirm>
              </Space>
            ),
          },
        ]}
      />
      {editor !== null && (
        <ConnectorEditor
          key={`${id}/${editor}`}
          id={id}
          connectorID={editor}
          onClose={() => {
            setEditor(null);
          }}
        />
      )}
      <Modal
        open={Boolean(logSource)}
        title={t("knowledge.manage.syncLogs")}
        footer={null}
        width={900}
        onCancel={() => {
          setLogSource("");
        }}
      >
        <Alert
          type="info"
          showIcon
          style={{ marginBottom: 16 }}
          title={tc("logsHint")}
        />
        {logs.error && <Alert type="error" title={parseApiError(logs.error)} />}
        <Table
          rowKey={(row) => text(row.id)}
          size="small"
          loading={logs.isFetching}
          dataSource={logs.data?.logs ?? []}
          pagination={{
            current: logPage,
            pageSize: 15,
            total: logs.data?.total ?? 0,
            showSizeChanger: false,
            onChange: setLogPage,
          }}
          columns={[
            {
              key: "id",
              title: "ID",
              render: (_, row: KnowledgeObject) => (
                <Button
                  type="link"
                  onClick={() => {
                    setLogDetail(row);
                  }}
                >
                  {text(row.id)}
                </Button>
              ),
            },
            {
              key: "task_type",
              title: tc("logTask"),
              render: (_, row: KnowledgeObject) => (
                <Tag
                  color={
                    row.task_type === "prune"
                      ? "orange"
                      : row.task_type === "sync"
                        ? "blue"
                        : "default"
                  }
                >
                  {row.task_type === "prune"
                    ? tc("prune")
                    : row.task_type === "sync"
                      ? tc("sync")
                      : tc("unknownTask", { value: text(row.task_type) })}
                </Tag>
              ),
            },
            {
              key: "status",
              title: tc("taskStatus"),
              render: (_, row: KnowledgeObject) =>
                tc(`status.${text(row.status)}`, {
                  defaultValue: text(row.status),
                }),
            },
            {
              key: "new_docs_indexed",
              title: tc("logAdded"),
              render: (_, row: KnowledgeObject) =>
                row.task_type === "sync" ? text(row.new_docs_indexed) : "-",
            },
            {
              key: "docs_removed_from_index",
              title: tc("logRemoved"),
              render: (_, row: KnowledgeObject) =>
                row.task_type === "prune"
                  ? text(row.docs_removed_from_index)
                  : "-",
            },
            {
              key: "error_count",
              title: tc("logErrors"),
              render: (_, row: KnowledgeObject) => text(row.error_count),
            },
            {
              key: "update_date",
              title: "update_date",
              render: (_, row: KnowledgeObject) => text(row.update_date),
            },
          ]}
        />
      </Modal>
      <Modal
        open={Boolean(logDetail)}
        title={t("knowledge.manage.logDetail")}
        footer={null}
        onCancel={() => {
          setLogDetail(null);
        }}
      >
        <Descriptions
          column={1}
          items={Object.entries(logDetail ?? {}).map(([key, value]) => ({
            key,
            label: key,
            children: (
              <span
                style={{ whiteSpace: "pre-wrap", overflowWrap: "anywhere" }}
              >
                {typeof value === "object"
                  ? JSON.stringify(value)
                  : text(value)}
              </span>
            ),
          }))}
        />
      </Modal>
    </Space>
  );
}
interface ConnectorValues {
  name: string;
  source: string;
  config: string;
  refresh_freq: number;
  prune_freq: number;
  timeout_secs: number;
  reschedule: boolean;
}
function ConnectorEditor({
  id,
  connectorID,
  onClose,
}: {
  id: string;
  connectorID: string;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const { t: tc } = useConnectorTranslation();
  const styles = usePrimaryButtonStyle();
  const [initialRead, setInitialRead] = useState<{
    snapshot?: KnowledgeObject;
    error?: unknown;
    unconfirmed?: boolean;
    loading: boolean;
  }>({ loading: Boolean(connectorID) });
  const [readAttempt, setReadAttempt] = useState(0);
  const initialData = initialRead.snapshot;
  const action = useKnowledgeAction(id);
  const [form] = Form.useForm<ConnectorValues>();
  useEffect(() => {
    if (!connectorID) return;
    let current = true;
    // Every opening starts from a direct read, never the shared query cache.
    // Mount the form only after this succeeds, then freeze this exact snapshot
    // for both initialValues and the three-way merge baseline.
    void knowledgeRequest<KnowledgeObject>(
      "get",
      id,
      `sources/${encodeURIComponent(connectorID)}`,
    )
      .then((snapshot) => {
        if (!current) return;
        const config = snapshot.config;
        if (
          snapshot.id !== connectorID ||
          typeof snapshot.source !== "string" ||
          !config ||
          typeof config !== "object" ||
          Array.isArray(config)
        ) {
          setInitialRead({ loading: false, unconfirmed: true });
          return;
        }
        setInitialRead({ snapshot, loading: false });
      })
      .catch((error: unknown) => {
        if (current) setInitialRead({ error, loading: false });
      });
    return () => {
      current = false;
    };
  }, [connectorID, id, readAttempt]);
  const source = Form.useWatch("source", form) as string | undefined;
  const configDraft = Form.useWatch("config", form) as string | undefined;
  let draft: KnowledgeObject | undefined;
  try {
    draft = parseObjectJSON(configDraft ?? "{}");
  } catch {
    /* retain invalid JSON for correction */
  }
  const deletionSupported = supportsDeletionSync(source ?? "", draft);
  const sourceHint = BLOB_SYNC_SOURCES.has(source ?? "")
    ? "knowledge.manage.connectorBlobSync"
    : undefined;
  const [createdID, setCreatedID] = useState("");
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);
  const save = async () => {
    try {
      setError("");
      const values = await form.validateFields();
      setSaving(true);
      let config = writableConnectorConfig(parseObjectJSON(values.config));
      if (connectorID) {
        const fresh = await knowledgeRequest<KnowledgeObject>(
          "get",
          id,
          `sources/${encodeURIComponent(connectorID)}`,
        );
        if (
          fresh.id !== connectorID ||
          fresh.source !== values.source ||
          !fresh.config ||
          typeof fresh.config !== "object" ||
          Array.isArray(fresh.config)
        )
          throw new Error(t("knowledge.manage.readbackFailed"));
        const merged = mergeConnectorConfig(
          writableConnectorConfig(
            (initialData?.config ?? {}) as KnowledgeObject,
          ),
          config,
          writableConnectorConfig(fresh.config as KnowledgeObject),
        );
        if (!merged) throw new Error(tc("configConflict"));
        config = merged;
      }
      for (const [key, minimum] of [
        ["refresh_freq", 0],
        ["prune_freq", 0],
        ["timeout_secs", 1],
      ] as const) {
        if (!isConnectorSchedule(values[key], minimum))
          throw new Error(tc("scheduleInteger", { minimum }));
      }
      if (
        typeof config.sync_deleted_files !== "undefined" &&
        typeof config.sync_deleted_files !== "boolean"
      )
        throw new Error(t("knowledge.manage.deletionSyncBoolean"));
      if (
        !supportsDeletionSync(values.source, config) &&
        config.sync_deleted_files === true
      )
        throw new Error(tc("unsupportedDeletion"));
      const issue = connectorConfigIssue(values.source, config);
      if (issue) throw new Error(tc("configIssue", { field: issue }));
      if (supportsDeletionSync(values.source, config)) {
        config.sync_deleted_files = config.sync_deleted_files ?? false;
      }
      if (
        BLOB_SYNC_SOURCES.has(values.source) &&
        config.sync_deleted_files === true &&
        initialData
      ) {
        const previous = initialData.config;
        if (
          !previous ||
          typeof previous !== "object" ||
          Array.isArray(previous) ||
          !containsKnowledgeFields(
            blobSyncScope(config, values.source),
            blobSyncScope(previous as KnowledgeObject, values.source),
          )
        )
          throw new Error(t("knowledge.manage.connectorBlobScopeChanged"));
      }
      const body = {
        config,
        refresh_freq: values.refresh_freq,
        prune_freq: values.prune_freq,
        timeout_secs: values.timeout_secs,
      };
      let target = connectorID || createdID;
      if (target) {
        const result = await action.mutateAsync({
          method: "patch",
          resource: `sources/${encodeURIComponent(target)}`,
          data: { ...body, ...(values.reschedule ? { reschedule: true } : {}) },
        });
        if (result === false || result === null)
          throw new Error(t("knowledge.manage.readbackFailed"));
      } else {
        const result = await action.mutateAsync({
          method: "post",
          resource: "sources",
          data: { ...body, name: values.name.trim(), source: values.source },
        });
        if (typeof result !== "string" || !result)
          throw new Error(t("knowledge.manage.connectorIDMissing"));
        target = result;
        setCreatedID(result);
      }
      const persisted = await knowledgeRequest<KnowledgeObject>(
        "get",
        id,
        `sources/${encodeURIComponent(target)}`,
      );
      if (persisted.id !== target || !containsKnowledgeFields(persisted, body))
        throw new Error(t("knowledge.manage.readbackFailed"));
      if (!connectorID) {
        const result = await action.mutateAsync({
          method: "put",
          resource: `connectors/${encodeURIComponent(target)}`,
          data: { auto_parse: true },
        });
        if (result === false || result === null)
          throw new Error(t("knowledge.manage.readbackFailed"));
      }
      onClose();
    } catch (err) {
      if (!(err !== null && typeof err === "object" && "errorFields" in err))
        setError(parseApiError(err));
    } finally {
      setSaving(false);
    }
  };
  return (
    <Modal
      open
      title={t(
        connectorID
          ? "knowledge.manage.connectorSettings"
          : "knowledge.manage.createConnector",
      )}
      onCancel={onClose}
      width={720}
      mask={{ closable: !saving }}
      closable={!saving}
      cancelButtonProps={{ disabled: saving }}
      onOk={save}
      okText={t("knowledge.docs.saveBtn")}
      cancelText={t("common.cancel")}
      confirmLoading={saving || action.isPending}
      okButtonProps={{
        className: styles.root,
        disabled: Boolean(connectorID && !initialData),
      }}
    >
      {initialRead.loading && (
        <Typography.Text type="secondary">
          {t("knowledge.manage.readingConfig")}
        </Typography.Text>
      )}
      {(Boolean(initialRead.error) || initialRead.unconfirmed) && (
        <Alert
          type="error"
          title={
            initialRead.unconfirmed
              ? t("knowledge.manage.readbackFailed")
              : parseApiError(initialRead.error)
          }
          action={
            <Button
              onClick={() => {
                setInitialRead({ loading: true });
                setReadAttempt((value) => value + 1);
              }}
            >
              {t("knowledge.docs.refresh")}
            </Button>
          }
        />
      )}
      {createdID && (
        <Alert
          type="warning"
          title={t("knowledge.manage.connectorCreatedPending")}
        />
      )}
      {connectorID && (
        <Alert
          type="info"
          showIcon
          style={{ marginBottom: 16 }}
          title={t("knowledge.manage.connectorSharedSettings")}
        />
      )}
      {(!connectorID || initialData) && (
        <Form
          form={form}
          key={connectorID}
          layout="vertical"
          disabled={saving}
          initialValues={{
            name: initialData?.name ?? "",
            source: initialData?.source ?? "s3",
            config: JSON.stringify(
              writableConnectorConfig(
                (initialData?.config ?? {}) as KnowledgeObject,
              ),
              null,
              2,
            ),
            refresh_freq: initialData?.refresh_freq ?? 60,
            prune_freq: initialData?.prune_freq ?? 0,
            timeout_secs: initialData?.timeout_secs ?? 3600,
            reschedule: false,
          }}
        >
          <Form.Item
            name="name"
            label={t("knowledge.manage.name")}
            rules={[{ required: true, whitespace: true }]}
          >
            <Input disabled={Boolean(connectorID || createdID)} />
          </Form.Item>
          <Form.Item
            name="source"
            label={t("knowledge.manage.source")}
            rules={[
              {
                validator: (_, value: unknown) =>
                  typeof value === "string" &&
                  (CONNECTOR_SOURCES.includes(value) || Boolean(connectorID))
                    ? Promise.resolve()
                    : Promise.reject(new Error(tc("sourceRequired"))),
              },
            ]}
          >
            <Select
              disabled={Boolean(connectorID || createdID)}
              showSearch={{ optionFilterProp: "label" }}
              options={CONNECTOR_SOURCES.map((value) => ({
                value,
                label: value,
              }))}
            />
          </Form.Item>
          <Alert
            type="info"
            showIcon
            style={{ marginBottom: 16 }}
            title={tc("requiredFields")}
            description={
              <Space orientation="vertical" style={{ width: "100%" }}>
                <Typography.Text>{tc("configGuide")}</Typography.Text>
                <Typography.Text code style={{ overflowWrap: "anywhere" }}>
                  {CONNECTOR_REQUIRED_FIELDS[source ?? ""] ?? source}
                </Typography.Text>
                <Typography.Text>
                  {tc(`guide.${source}`, { defaultValue: "" })}
                </Typography.Text>
                {CONNECTOR_EXAMPLES[source ?? ""] && (
                  <details>
                    <summary style={{ cursor: "pointer" }}>
                      {tc("example")}
                    </summary>
                    <pre
                      style={{
                        whiteSpace: "pre-wrap",
                        overflowWrap: "anywhere",
                      }}
                    >
                      {JSON.stringify(
                        CONNECTOR_EXAMPLES[source ?? ""],
                        null,
                        2,
                      )}
                    </pre>
                  </details>
                )}
              </Space>
            }
          />
          {deletionSupported ? (
            <Form.Item
              label={t("knowledge.manage.deletionSync")}
              extra={tc("deletionHint")}
            >
              <Space orientation="vertical">
                <Typography.Paragraph type="secondary" style={{ margin: 0 }}>
                  {tc("deletionLifecycle")}
                </Typography.Paragraph>
                <Typography.Paragraph type="secondary" style={{ margin: 0 }}>
                  {tc("deletionScope")}
                </Typography.Paragraph>
                <Switch
                  aria-label={t("knowledge.manage.deletionSync")}
                  disabled={!draft || saving}
                  checked={draft?.sync_deleted_files === true}
                  onChange={(value) => {
                    if (draft)
                      form.setFieldValue(
                        "config",
                        JSON.stringify(
                          { ...draft, sync_deleted_files: value },
                          null,
                          2,
                        ),
                      );
                  }}
                />
                <Typography.Text type="secondary">
                  {tc(
                    draft?.sync_deleted_files === true
                      ? "deletionOn"
                      : "deletionOff",
                  )}
                </Typography.Text>
              </Space>
            </Form.Item>
          ) : (
            <Alert
              style={{ marginBottom: 16 }}
              type="info"
              showIcon
              title={tc("unsupportedDeletion")}
            />
          )}
          {sourceHint && (
            <Alert
              type="info"
              showIcon
              style={{ marginBottom: 16 }}
              title={t(sourceHint)}
            />
          )}
          <Form.Item
            name="config"
            label={t("knowledge.manage.sourceConfig")}
            extra={t("knowledge.manage.sourceConfigHint")}
            rules={[
              {
                validator: (_, value: string) => {
                  try {
                    parseObjectJSON(value);
                    return Promise.resolve();
                  } catch {
                    return Promise.reject(
                      new Error(t("knowledge.manage.jsonInvalid")),
                    );
                  }
                },
              },
            ]}
          >
            <Input.TextArea autoComplete="off" rows={6} />
          </Form.Item>
          {source && ["google_drive", "gmail", "box"].includes(source) && (
            <KnowledgeSourceOAuth
              key={source}
              id={id}
              source={source}
              onAuthorized={(credentials) => {
                const rawConfig: unknown = form.getFieldValue("config");
                if (typeof rawConfig !== "string")
                  throw new Error(t("knowledge.manage.jsonInvalid"));
                const config = parseObjectJSON(rawConfig);
                const previous = config.credentials;
                config.credentials = {
                  ...(previous !== null &&
                  typeof previous === "object" &&
                  !Array.isArray(previous)
                    ? previous
                    : {}),
                  ...credentials,
                };
                form.setFieldValue("config", JSON.stringify(config, null, 2));
              }}
            />
          )}
          <Form.Item
            name="refresh_freq"
            label={t("knowledge.manage.refreshMinutes")}
            extra={tc("refreshHint")}
            rules={[
              {
                validator: (_, value: unknown) =>
                  isConnectorSchedule(value, 0)
                    ? Promise.resolve()
                    : Promise.reject(
                        new Error(tc("scheduleInteger", { minimum: 0 })),
                      ),
              },
            ]}
          >
            <InputNumber min={0} step={1} changeOnBlur={false} />
          </Form.Item>
          <Form.Item
            name="prune_freq"
            label={t("knowledge.manage.pruneMinutes")}
            extra={tc("pruneHint")}
            rules={[
              {
                validator: (_, value: unknown) =>
                  isConnectorSchedule(value, 0)
                    ? Promise.resolve()
                    : Promise.reject(
                        new Error(tc("scheduleInteger", { minimum: 0 })),
                      ),
              },
            ]}
          >
            <InputNumber
              min={0}
              step={1}
              changeOnBlur={false}
              disabled={
                !deletionSupported ||
                draft?.sync_deleted_files !== true ||
                saving
              }
            />
          </Form.Item>
          <Form.Item
            name="timeout_secs"
            label={tc("timeoutSeconds")}
            rules={[
              {
                validator: (_, value: unknown) =>
                  isConnectorSchedule(value, 1)
                    ? Promise.resolve()
                    : Promise.reject(
                        new Error(tc("scheduleInteger", { minimum: 1 })),
                      ),
              },
            ]}
          >
            <InputNumber min={1} step={1} changeOnBlur={false} />
          </Form.Item>
          {(connectorID || createdID) && (
            <Form.Item
              name="reschedule"
              label={tc("reschedule")}
              extra={tc("rescheduleHint")}
              valuePropName="checked"
            >
              <Switch aria-label={tc("reschedule")} />
            </Form.Item>
          )}
          {error && (
            <Alert type="error" title={error} style={{ marginBottom: 16 }} />
          )}
          {!connectorID && (
            <Alert
              type="info"
              title={t("knowledge.manage.connectorCreationHint")}
            />
          )}
          {connectorID && (
            <Popconfirm
              title={t("knowledge.manage.deleteConnectorConfirm")}
              onConfirm={async () => {
                try {
                  await action.mutateAsync({
                    method: "delete",
                    resource: `sources/${encodeURIComponent(connectorID)}`,
                  });
                  onClose();
                } catch {
                  /* mutation shows error */
                }
              }}
            >
              <Button danger>{t("common.delete")}</Button>
            </Popconfirm>
          )}
        </Form>
      )}
    </Modal>
  );
}
