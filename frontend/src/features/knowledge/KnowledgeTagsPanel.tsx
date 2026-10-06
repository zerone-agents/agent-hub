import { useEffect, useRef, useState, useSyncExternalStore } from "react";
import {
  Button,
  Collapse,
  Tag,
  Alert,
  Form,
  Input,
  InputNumber,
  Modal,
  Select,
  Space,
  Table,
  Typography,
} from "antd";
import { useTranslation } from "react-i18next";
import { useKnowledgeResource } from "@/queries/useKnowledgeManagement";
import { useKnowledgeDetail, useKnowledgeList } from "@/queries/useKnowledge";
import { knowledgeApi } from "@/api/knowledge";
import {
  knowledgeRequest,
  containsKnowledgeFields,
  knowledgeTagsApi,
  type KnowledgeTagCount,
  createKnowledgeTagOwner,
} from "@/api/knowledgeManagement";
import PrimaryButton, {
  usePrimaryButtonStyle,
} from "@/components/PrimaryButton";
import { useCanWrite } from "@/hooks/useCanWrite";
import { getAccessToken, parseApiError } from "@/api/client";
import { useAuthStore } from "@/stores/auth";
import { Link } from "react-router";
import { useMetadataTranslation } from "./useMetadataTranslation";

interface TagScopeOrigin {
  id: string;
  token: string | null;
  role?: string;
}
interface TagSource {
  id: string;
  name: string;
}
interface TagAggregationResult {
  origin: TagScopeOrigin;
  sources: TagSource[];
  combined: KnowledgeTagCount[];
  libraries: Record<string, KnowledgeTagCount[]>;
}
// Token replacement can precede the auth store update. Observe that gap too.
function subscribeTagCredentials(onChange: () => void) {
  const timer = window.setInterval(onChange, 250);
  window.addEventListener("storage", onChange);
  window.addEventListener("focus", onChange);
  return () => {
    window.clearInterval(timer);
    window.removeEventListener("storage", onChange);
    window.removeEventListener("focus", onChange);
  };
}
function TagAggregation() {
  const user = useAuthStore((state) => state.user);
  const token = useSyncExternalStore(subscribeTagCredentials, getAccessToken);
  const [session, setSession] = useState({
    id: user?.id,
    role: user?.role,
    token,
    generation: 0,
  });
  if (
    session.id !== user?.id ||
    session.role !== user?.role ||
    session.token !== token
  )
    setSession({
      id: user?.id,
      role: user?.role,
      token,
      generation: session.generation + 1,
    });
  // External credential changes discard drafts; a C3-owned identity refresh
  // may retain this mount while its replacement credentials are verified.
  // No credentials enter React Query keys or serialized response data.
  return (
    <TagAggregationContent
      key={session.generation}
      user={user}
      token={token}
      onCredentialRefresh={(origin) => {
        if (
          origin.id !== useAuthStore.getState().user?.id ||
          origin.role !== useAuthStore.getState().user?.role
        )
          return;
        setSession((current) =>
          current.id === origin.id && current.role === origin.role
            ? { ...current, token: origin.token }
            : current,
        );
      }}
    />
  );
}
function TagAggregationContent({
  user,
  token,
  onCredentialRefresh,
}: {
  user: { id: string; role?: string } | null;
  token: string | null;
  onCredentialRefresh: (origin: TagScopeOrigin) => void;
}) {
  const { t: mt } = useMetadataTranslation();
  const [page, setPage] = useState(1);
  const sources = useKnowledgeList({ page, page_size: 10 }, { owned: true });
  const [selection, setSelection] = useState<{
    origin: TagScopeOrigin;
    rows: TagSource[];
  }>();
  const [result, setResult] = useState<TagAggregationResult>();
  const [failure, setFailure] = useState<{
    origin: TagScopeOrigin;
    message: string;
  }>();
  const [search, setSearch] = useState("");
  const [pending, setPending] = useState<TagScopeOrigin>();
  const operation = useRef<AbortController | null>(null);
  useEffect(() => () => operation.current?.abort(), []);
  const isCurrent = (origin?: TagScopeOrigin) =>
    Boolean(
      origin &&
      origin.id === user?.id &&
      origin.token === token &&
      origin.id === useAuthStore.getState().user?.id &&
      origin.token === getAccessToken() &&
      user.role === useAuthStore.getState().user?.role &&
      origin.role === user.role &&
      origin.role !== "guest",
    );
  const sourceCurrent = isCurrent(sources.origin);
  const selected = isCurrent(selection?.origin) ? (selection?.rows ?? []) : [];
  const visibleResult = isCurrent(result?.origin) ? result : undefined;
  const busy = isCurrent(pending);
  const rows =
    sourceCurrent && !sources.error ? (sources.data?.datasets ?? []) : [];
  const aggregate = async () => {
    const selectedOrigin = selection?.origin;
    if (
      !selectedOrigin ||
      !isCurrent(selectedOrigin) ||
      !sourceCurrent ||
      sources.error ||
      !selected.length ||
      busy
    )
      return;
    const origin = { ...selectedOrigin };
    const scope = selected.map((row) => ({ ...row }));
    operation.current?.abort();
    const controller = new AbortController();
    operation.current = controller;
    const owner = createKnowledgeTagOwner(origin, controller.signal, {
      refreshing: (refreshed) => {
        onCredentialRefresh(refreshed);
        setPending(refreshed);
      },
      verified: (verified) => {
        setSelection({ origin: verified, rows: scope });
        setPending(verified);
        // Source cache keeps its original provenance until a new verified read.
        void sources.refetch();
      },
    });
    setPending(origin);
    setResult(undefined);
    setFailure(undefined);
    try {
      const combined = await knowledgeTagsApi.aggregate(
        scope.map((row) => row.id),
        owner,
      );
      const libraries: Record<string, KnowledgeTagCount[]> = {};
      if (scope.length === 1) libraries[scope[0].id] = combined;
      else {
        // Serialize readonly phases so one expired credential cannot interrupt
        // another phase while userinfo is validating its replacement.
        for (const source of scope) {
          owner.assertCurrent();
          libraries[source.id] = await knowledgeTagsApi.aggregate(
            [source.id],
            owner,
          );
        }
      }
      owner.assertCurrent();
      setResult({ origin, sources: scope, combined, libraries });
    } catch (error) {
      if (owner.isCurrent())
        setFailure({ origin, message: parseApiError(error) });
      controller.abort(); // Stop the remaining distribution reads on any failure.
    } finally {
      if (operation.current === controller) setPending(undefined);
    }
  };
  const combinedCounts = new Map(
    visibleResult?.combined.map((tag) => [tag.value, tag.count]),
  );
  const libraryCounts = new Map(
    visibleResult?.sources.map((source) => [
      source.id,
      new Map(
        visibleResult.libraries[source.id].map((tag) => [tag.value, tag.count]),
      ),
    ]),
  );
  const observedNames = new Set([
    ...combinedCounts.keys(),
    ...[...libraryCounts.values()].flatMap((counts) => [...counts.keys()]),
  ]);
  const distribution = [...observedNames].map((value) => ({
    value,
    count: combinedCounts.get(value),
  }));
  return (
    <Space orientation="vertical" size="middle" style={{ width: "100%" }}>
      <Typography.Paragraph style={{ margin: 0 }}>
        {mt("tagAggregationScopeHint")}
      </Typography.Paragraph>
      {(Boolean(sources.error) ||
        (!sourceCurrent && !sources.isFetching && !busy)) && (
        <Alert
          type="warning"
          showIcon
          title={
            sources.error ? mt("sourceError") : mt("tagAggregationOwnerChanged")
          }
          description={sources.error ? parseApiError(sources.error) : undefined}
          action={
            <Button
              onClick={() => {
                void sources.refetch();
              }}
            >
              {mt("retry")}
            </Button>
          }
        />
      )}
      <Table<TagSource>
        size="small"
        rowKey="id"
        dataSource={rows}
        loading={sources.isFetching || (busy && !sourceCurrent)}
        scroll={{ x: 360 }}
        locale={{
          emptyText: mt(
            sourceCurrent
              ? "tagAggregationNoSources"
              : "tagAggregationReloadSources",
          ),
        }}
        rowSelection={{
          selectedRowKeys: selected.map((row) => row.id),
          preserveSelectedRowKeys: true,
          getCheckboxProps: () => ({
            disabled: busy || !sourceCurrent || Boolean(sources.error),
          }),
          onChange: (keys) => {
            if (
              !sourceCurrent ||
              !sources.origin ||
              !isCurrent(sources.origin) ||
              sources.error ||
              busy
            )
              return;
            const known = new Map(
              [...selected, ...rows].map((row) => [
                row.id,
                { id: row.id, name: row.name },
              ]),
            );
            const picked = keys.flatMap((key) => {
              const row = known.get(String(key));
              return row ? [row] : [];
            });
            setSelection({ origin: sources.origin, rows: picked });
            setResult(undefined);
            setFailure(undefined);
          },
        }}
        columns={[
          {
            title: mt("tagAggregationLibrary"),
            dataIndex: "name",
            render: (name: string) => (
              <span style={{ overflowWrap: "anywhere" }}>{name}</span>
            ),
          },
        ]}
        pagination={{
          current: page,
          pageSize: 10,
          total: sourceCurrent ? (sources.data?.total ?? 0) : 0,
          showSizeChanger: false,
          disabled:
            busy ||
            sources.isFetching ||
            !sourceCurrent ||
            Boolean(sources.error),
          onChange: (value) => {
            setPage(value);
          },
        }}
      />
      <Typography.Text type="secondary">
        {mt("tagAggregationPageHint", { page, count: rows.length })}
      </Typography.Text>
      <Space wrap>
        <Typography.Text>
          {mt("tagAggregationSelected", { count: selected.length })}
        </Typography.Text>
        {selected.map((source) => (
          <Tag key={source.id}>{source.name}</Tag>
        ))}
        <Button
          disabled={!selected.length || busy}
          onClick={() => {
            setSelection(undefined);
            setResult(undefined);
            setFailure(undefined);
          }}
        >
          {mt("tagAggregationClear")}
        </Button>
        <PrimaryButton
          disabled={
            !selected.length || !sourceCurrent || Boolean(sources.error) || busy
          }
          loading={busy}
          onClick={() => {
            void aggregate();
          }}
        >
          {mt("tagAggregationRun")}
        </PrimaryButton>
      </Space>
      {isCurrent(failure?.origin) && (
        <Alert
          type="error"
          showIcon
          title={mt("tagAggregationFailed")}
          description={failure?.message}
        />
      )}
      {visibleResult && (
        <>
          <Alert
            type="info"
            showIcon
            title={mt("tagAggregationIncomplete")}
            description={mt("tagAggregationDistributionHint")}
          />
          {(visibleResult.combined.length >= 1000 ||
            Object.values(visibleResult.libraries).some(
              (tags) => tags.length >= 1000,
            )) && (
            <Alert type="warning" showIcon title={mt("tagAggregationCapped")} />
          )}
          <Typography.Text>
            {mt("tagAggregationResultScope", {
              names: visibleResult.sources.map((row) => row.name).join(" / "),
            })}
          </Typography.Text>
          <Input.Search
            aria-label={mt("tagAggregationSearch")}
            placeholder={mt("tagAggregationSearch")}
            value={search}
            onChange={(event) => {
              setSearch(event.target.value);
            }}
          />
          <Table
            size="small"
            rowKey="value"
            scroll={{
              x: Math.max(560, 300 + visibleResult.sources.length * 180),
            }}
            locale={{
              emptyText: mt(
                distribution.length
                  ? "tagAggregationNoSearch"
                  : "tagAggregationEmpty",
              ),
            }}
            dataSource={distribution.filter((row) =>
              row.value.toLowerCase().includes(search.toLowerCase()),
            )}
            columns={[
              {
                title: mt("tagAggregationTag"),
                dataIndex: "value",
                fixed: "left",
                width: 180,
                render: (value: string) => (
                  <span style={{ overflowWrap: "anywhere" }}>{value}</span>
                ),
              },
              {
                title: mt("tagAggregationCount"),
                dataIndex: "count",
                width: 140,
                render: (count?: number) =>
                  count ?? mt("tagAggregationNotReturned"),
              },
              ...visibleResult.sources.map((source) => ({
                title: source.name,
                key: source.id,
                width: 180,
                render: (_: unknown, row: { value: string }) =>
                  libraryCounts.get(source.id)?.get(row.value) ??
                  mt("tagAggregationNotReturned"),
              })),
            ]}
          />
          <Space wrap>
            {visibleResult.sources.map((source) => (
              <Space key={source.id}>
                <Typography.Text>{source.name}</Typography.Text>
                <Link
                  to={`/knowledge/${encodeURIComponent(source.id)}/settings`}
                >
                  {mt("tagAggregationSettings")}
                </Link>
                <Link
                  to={`/knowledge/${encodeURIComponent(source.id)}/retrieval`}
                >
                  {mt("tagAggregationRetrieval")}
                </Link>
              </Space>
            ))}
          </Space>
        </>
      )}
    </Space>
  );
}

interface TagChange {
  from: string;
  to?: string;
  count: number;
}
function AutoTagSettings({
  id,
  config,
  options,
  sourcesLoading,
  sourceError,
  onRetry,
  onSaved,
}: {
  id: string;
  config: Record<string, unknown>;
  options: { value: string; label: string }[];
  sourcesLoading: boolean;
  sourceError?: unknown;
  onRetry: () => void;
  onSaved: () => void;
}) {
  const { t } = useTranslation();
  const { t: mt } = useMetadataTranslation();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [saved, setSaved] = useState(false);
  const canWrite = useCanWrite();
  const [form] = Form.useForm<{ tag_kb_ids: string[]; topn_tags: number }>();
  return (
    <Form
      form={form}
      layout="vertical"
      disabled={busy || !canWrite}
      initialValues={{
        tag_kb_ids: config.tag_kb_ids ?? [],
        topn_tags: config.topn_tags ?? 1,
      }}
      onValuesChange={() => {
        setSaved(false);
      }}
      onFinish={async (values) => {
        if (!canWrite) return;
        setBusy(true);
        setError("");
        setSaved(false);
        try {
          // Send only tag settings. Never replay unrelated parser configuration.
          const patch = {
            ...(form.isFieldTouched("tag_kb_ids")
              ? { tag_kb_ids: values.tag_kb_ids }
              : {}),
            ...(form.isFieldTouched("topn_tags")
              ? { topn_tags: values.topn_tags }
              : {}),
          };
          if (!Object.keys(patch).length) return;
          await knowledgeApi.datasets.update(id, { parser_config: patch });
          const persisted = await knowledgeApi.datasets.get(id);
          if (!containsKnowledgeFields(persisted.parser_config, patch))
            throw new Error(t("knowledge.manage.readbackFailed"));
          setSaved(true);
          onSaved();
        } catch (error) {
          setError(parseApiError(error));
        } finally {
          setBusy(false);
        }
      }}
    >
      {error && (
        <Alert
          type="error"
          showIcon
          title={error}
          style={{ marginBottom: 12 }}
        />
      )}
      {!!sourceError && (
        <Alert
          type="error"
          showIcon
          title={mt("sourceError")}
          description={parseApiError(sourceError)}
          action={<Button onClick={onRetry}>{mt("retry")}</Button>}
          style={{ marginBottom: 12 }}
        />
      )}
      <Form.Item
        name="tag_kb_ids"
        label={t("knowledge.manage.tagSources")}
        extra={t("knowledge.manage.tagSourcesHint")}
      >
        <Select
          mode="multiple"
          showSearch={{ optionFilterProp: "label" }}
          allowClear
          loading={sourcesLoading}
          options={options}
        />
      </Form.Item>
      <Form.Item
        name="topn_tags"
        label={t("knowledge.manage.topTags")}
        rules={[{ required: true }, { type: "number", min: 1, max: 10 }]}
      >
        <InputNumber min={1} max={10} />
      </Form.Item>
      <PrimaryButton htmlType="submit" loading={busy}>
        {t("knowledge.docs.saveBtn")}
      </PrimaryButton>
      {saved && (
        <Alert
          type="success"
          showIcon
          title={mt("saveVerified")}
          style={{ marginTop: 12 }}
        />
      )}
    </Form>
  );
}

function KnowledgeTagsPanelContent({ id }: { id: string }) {
  const { t } = useTranslation();
  const { t: mt } = useMetadataTranslation();
  const canWrite = useCanWrite();
  const style = usePrimaryButtonStyle();
  const tags = useKnowledgeResource<[string, number][]>(id, "tags");
  const dataset = useKnowledgeDetail(id);
  const [sourcePage, setSourcePage] = useState(1);
  const datasets = useKnowledgeList({ page: sourcePage, page_size: 100 });
  const [search, setSearch] = useState("");
  const [change, setChange] = useState<TagChange>();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [saved, setSaved] = useState(false);
  const [renameForm] = Form.useForm<{ from_tag: string; to_tag: string }>();
  const [readbackOnly, setReadbackOnly] = useState(false);
  const rows = (tags.data ?? []).map(([name, count]) => ({ name, count }));
  const verifyTag = async (operation: TagChange) => {
    const stored = await knowledgeRequest<[string, number][]>(
      "get",
      id,
      "tags",
    );
    // MultiRAG tags is a top-1000 aggregation, not a complete inventory.
    if (stored.length >= 1000) throw new Error(mt("tagReadbackIncomplete"));
    if (
      stored.some(([name]) => name === operation.from) ||
      (operation.to &&
        operation.count > 0 &&
        !stored.some(([name]) => name === operation.to))
    )
      throw new Error(t("knowledge.manage.readbackFailed"));
  };
  const apply = async () => {
    if (!canWrite || !change) return;
    setBusy(true);
    setError("");
    setSaved(false);
    try {
      if (!readbackOnly)
        await knowledgeRequest(
          change.to ? "put" : "delete",
          id,
          "tags",
          change.to
            ? { from_tag: change.from, to_tag: change.to }
            : { tags: [change.from] },
        );
      setReadbackOnly(true);
      await verifyTag(change);
      setSaved(true);
      setChange(undefined);
      setReadbackOnly(false);
      renameForm.resetFields();
      await tags.refetch();
    } catch (error) {
      setError(parseApiError(error));
      setReadbackOnly(true);
    } finally {
      setBusy(false);
    }
  };
  const sourceOptions = (datasets.data?.datasets ?? []).map((kb) => ({
    value: kb.id,
    label: kb.name,
  }));
  const selectedSourceIds = Array.isArray(
    dataset.data?.parser_config.tag_kb_ids,
  )
    ? (dataset.data.parser_config.tag_kb_ids as string[])
    : [];
  for (const source of selectedSourceIds)
    if (!sourceOptions.some((option) => option.value === source))
      sourceOptions.push({ value: source, label: source });
  return (
    <Space orientation="vertical" style={{ width: "100%" }} size="middle">
      <Collapse
        items={[
          {
            key: "aggregation",
            label: mt("tagAggregationTitle"),
            children: <TagAggregation />,
          },
        ]}
      />
      <Space wrap>
        <Button
          disabled={busy}
          onClick={() => {
            void tags.refetch();
            void dataset.refetch();
          }}
        >
          {t("knowledge.docs.refresh")}
        </Button>
        <Input.Search
          aria-label={mt("tagSearch")}
          placeholder={mt("tagSearch")}
          value={search}
          onChange={(event) => {
            setSearch(event.target.value);
          }}
        />
      </Space>
      {(tags.error ?? dataset.error) && (
        <Alert
          type="error"
          showIcon
          title={parseApiError(tags.error ?? dataset.error)}
        />
      )}
      {saved && <Alert type="success" showIcon title={mt("tagVerified")} />}
      <Table
        rowKey="name"
        size="small"
        dataSource={rows.filter((row) =>
          row.name.toLowerCase().includes(search.toLowerCase()),
        )}
        loading={tags.isFetching}
        scroll={{ x: 360 }}
        columns={[
          {
            title: t("knowledge.manage.tag"),
            dataIndex: "name",
            render: (name: string) => (
              <span style={{ overflowWrap: "anywhere" }}>{name}</span>
            ),
          },
          { title: t("knowledge.manage.count"), dataIndex: "count" },
          {
            title: t("knowledge.docs.actions"),
            render: (_, row) =>
              canWrite && (
                <Button
                  disabled={busy || Boolean(tags.error)}
                  danger
                  type="link"
                  onClick={() => {
                    setError("");
                    setReadbackOnly(false);
                    setChange({ from: row.name, count: row.count });
                  }}
                >
                  {t("common.delete")}
                </Button>
              ),
          },
        ]}
      />
      {canWrite && (
        <>
          <Form
            form={renameForm}
            layout="vertical"
            disabled={busy || Boolean(tags.error)}
            onFinish={(values) => {
              const from = values.from_tag;
              const to = values.to_tag.trim();
              if (from === to) return;
              setError("");
              setReadbackOnly(false);
              setChange({
                from,
                to,
                count: rows.find((row) => row.name === from)?.count ?? 0,
              });
            }}
          >
            <Form.Item
              name="from_tag"
              label={t("knowledge.manage.tag")}
              rules={[{ required: true }]}
            >
              <Select
                showSearch
                options={rows.map((row) => ({
                  value: row.name,
                  label: row.name,
                }))}
              />
            </Form.Item>
            <Form.Item
              name="to_tag"
              label={t("knowledge.manage.newTag")}
              rules={[
                { required: true, whitespace: true },
                {
                  validator: (_, value: string | undefined) =>
                    (value ?? "").trim() !==
                    renameForm.getFieldValue("from_tag")
                      ? Promise.resolve()
                      : Promise.reject(new Error(mt("invalid"))),
                },
              ]}
            >
              <Input />
            </Form.Item>
            <PrimaryButton htmlType="submit" loading={busy}>
              {t("knowledge.docs.rename")}
            </PrimaryButton>
          </Form>
          <Typography.Title level={5}>
            {t("knowledge.manage.autoTags")}
          </Typography.Title>
          {dataset.data && !dataset.error && (
            <AutoTagSettings
              id={id}
              config={dataset.data.parser_config}
              options={sourceOptions}
              sourcesLoading={datasets.isFetching}
              sourceError={datasets.error}
              onRetry={() => {
                void datasets.refetch();
              }}
              onSaved={() => {
                void dataset.refetch();
              }}
            />
          )}
          <Space wrap>
            <Button
              disabled={sourcePage === 1 || datasets.isFetching}
              onClick={() => {
                setSourcePage((page) => page - 1);
              }}
            >
              {mt("previous")}
            </Button>
            <Typography.Text>{sourcePage}</Typography.Text>
            <Button
              disabled={
                datasets.isFetching ||
                sourcePage * 100 >= (datasets.data?.total ?? 0)
              }
              onClick={() => {
                setSourcePage((page) => page + 1);
              }}
            >
              {mt("next")}
            </Button>
          </Space>
        </>
      )}
      <Modal
        open={Boolean(change)}
        title={
          change?.to
            ? mt("renameTitle")
            : t("knowledge.manage.deleteTagConfirm")
        }
        onCancel={() => {
          if (!busy) setChange(undefined);
        }}
        onOk={apply}
        confirmLoading={busy}
        closable={!busy}
        mask={{ closable: !busy }}
        cancelButtonProps={{ disabled: busy }}
        okText={
          readbackOnly
            ? mt("retryReadback")
            : change?.to
              ? t("knowledge.docs.rename")
              : t("common.delete")
        }
        okButtonProps={{ className: style.root, disabled: !canWrite }}
      >
        <Alert
          type={change?.to ? "info" : "warning"}
          showIcon
          title={
            change?.to
              ? mt("renameHint", {
                  from: change.from,
                  to: change.to,
                  count: change.count,
                })
              : mt("deleteTagHint", {
                  name: change?.from,
                  count: change?.count,
                })
          }
        />
        {error && (
          <Alert
            type="error"
            showIcon
            title={error}
            style={{ marginTop: 12 }}
          />
        )}
      </Modal>
    </Space>
  );
}

export default function KnowledgeTagsPanel({ id }: { id: string }) {
  return <KnowledgeTagsPanelContent key={id} id={id} />;
}
