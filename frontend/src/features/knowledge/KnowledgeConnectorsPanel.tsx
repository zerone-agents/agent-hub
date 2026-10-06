import KnowledgeSourceOAuth from "./KnowledgeSourceOAuth";
import { useState } from "react";
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

const SOURCES = [
  "rss",
  "s3",
  "notion",
  "r2",
  "google_cloud_storage",
  "oci_storage",
  "slack",
  "confluence",
  "jira",
  "google_drive",
  "gmail",
  "discord",
  "webdav",
  "moodle",
  "s3_compatible",
  "dropbox",
  "box",
  "airtable",
  "asana",
  "github",
  "gitlab",
  "imap",
  "bitbucket",
  "zendesk",
  "seafile",
  "mysql",
  "postgresql",
  "dingtalk_ai_table",
];
// Match MultiRAG's trusted full-snapshot deletion sync sources.
const DELETION_SYNC_SOURCES = new Set([
  "rss",
  "webdav",
  "airtable",
  "google_drive",
  "gmail",
  "bitbucket",
  "s3",
  "r2",
  "oci_storage",
  "google_cloud_storage",
  "confluence",
  "notion",
  "jira",
  "box",
  "github",
  "gitlab",
  "dropbox",
  "seafile",
  "asana",
  "zendesk",
]);
function DeletionSyncStatus({ id, row }: { id: string; row: ConnectorRow }) {
  const { t } = useTranslation();
  const supported = DELETION_SYNC_SOURCES.has(row.source);
  const query = useKnowledgeResource<KnowledgeObject>(
    supported ? id : "",
    `sources/${encodeURIComponent(row.id)}`,
  );
  if (!supported)
    return <Tag>{t("knowledge.manage.deletionSyncUnsupported")}</Tag>;
  if (!query.data)
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
  if (!config || typeof config !== 'object' || Array.isArray(config)) return <Tag color="warning">{t('knowledge.manage.configUnconfirmed')}</Tag>;
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
          { title: t("knowledge.docs.statusCol"), dataIndex: "status" },
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
            "id",
            "status",
            "new_docs_indexed",
            "error_count",
            "update_date",
          ].map((key) => ({
            key,
            title: key,
            render: (_, row: KnowledgeObject) =>
              key === "id" ? (
                <Button
                  type="link"
                  onClick={() => {
                    setLogDetail(row);
                  }}
                >
                  {text(row.id)}
                </Button>
              ) : (
                text(row[key])
              ),
          }))}
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
  const styles = usePrimaryButtonStyle();
  const existing = useKnowledgeResource<KnowledgeObject>(
    connectorID ? id : "",
    `sources/${encodeURIComponent(connectorID)}`,
  );
  const action = useKnowledgeAction(id);
  const [form] = Form.useForm<ConnectorValues>();
  const source = Form.useWatch("source", form) as string | undefined;
  const configDraft = Form.useWatch("config", form) as string | undefined;
  let draft: KnowledgeObject | undefined;
  try {
    draft = parseObjectJSON(configDraft ?? "{}");
  } catch {
    /* retain invalid JSON for correction */
  }
  const deletionSupported = DELETION_SYNC_SOURCES.has(source ?? "");
  const [createdID, setCreatedID] = useState("");
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);
  const save = async () => {
    try {
      const values = await form.validateFields();
      setSaving(true);
      setError("");
      const config = parseObjectJSON(values.config);
      if (DELETION_SYNC_SOURCES.has(values.source)) {
        if (
          config.sync_deleted_files !== undefined &&
          typeof config.sync_deleted_files !== "boolean"
        )
          throw new Error(t("knowledge.manage.deletionSyncBoolean"));
        config.sync_deleted_files = config.sync_deleted_files ?? false;
      }
      const body = {
        config,
        refresh_freq: values.refresh_freq,
        prune_freq: values.prune_freq,
        timeout_secs: values.timeout_secs,
      };
      let target = connectorID || createdID;
      if (target)
        await action.mutateAsync({
          method: "patch",
          resource: `sources/${encodeURIComponent(target)}`,
          data: body,
        });
      else {
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
      if (!connectorID)
        await action.mutateAsync({
          method: "put",
          resource: `connectors/${encodeURIComponent(target)}`,
          data: { auto_parse: true },
        });
      onClose();
    } catch (err) {
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
      okButtonProps={{ className: styles.root }}
    >
      {(error || existing.error) && (
        <Alert type="error" title={error || parseApiError(existing.error)} />
      )}
      {createdID && (
        <Alert
          type="warning"
          title={t("knowledge.manage.connectorCreatedPending")}
        />
      )}
      {(!connectorID || existing.data) && (
        <Form
          form={form}
          key={connectorID}
          layout="vertical"
          disabled={saving}
          initialValues={{
            name: existing.data?.name ?? "",
            source: existing.data?.source ?? "s3",
            config: JSON.stringify(existing.data?.config ?? {}, null, 2),
            refresh_freq: existing.data?.refresh_freq ?? 60,
            prune_freq: existing.data?.prune_freq ?? 0,
            timeout_secs: existing.data?.timeout_secs ?? 3600,
          }}
        >
          <Form.Item
            name="name"
            label={t("knowledge.manage.name")}
            rules={[{ required: true, whitespace: true }]}
          >
            <Input disabled={Boolean(connectorID || createdID)} />
          </Form.Item>
          <Form.Item name="source" label={t("knowledge.manage.source")}>
            <Select
              disabled={Boolean(connectorID || createdID)}
              options={SOURCES.map((value) => ({ value, label: value }))}
            />
          </Form.Item>
          {deletionSupported ? (
            <Form.Item
              label={t("knowledge.manage.deletionSync")}
              extra={t("knowledge.manage.deletionSyncHint")}
            >
              <Space orientation="vertical">
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
                  {t(
                    draft?.sync_deleted_files === true
                      ? "knowledge.manage.deletionSyncOn"
                      : "knowledge.manage.deletionSyncOff",
                  )}
                </Typography.Text>
              </Space>
            </Form.Item>
          ) : (
            <Alert
              style={{ marginBottom: 16 }}
              type="info"
              showIcon
              title={t("knowledge.manage.deletionSyncUnsupportedHint")}
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
          >
            <InputNumber min={1} />
          </Form.Item>
          <Form.Item
            name="prune_freq"
            label={t("knowledge.manage.pruneMinutes")}
          >
            <InputNumber min={0} />
          </Form.Item>
          <Form.Item
            name="timeout_secs"
            label={t("knowledge.manage.timeoutSeconds")}
          >
            <InputNumber min={1} />
          </Form.Item>
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
