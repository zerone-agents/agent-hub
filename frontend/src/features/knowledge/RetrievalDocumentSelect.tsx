import { useEffect, useMemo, useState } from "react";
import { useInfiniteQuery } from "@tanstack/react-query";
import { Alert, Button, Select, Typography } from "antd";
import { useTranslation } from "react-i18next";
import { knowledgeApi } from "@/api/knowledge";
import { parseApiError } from "@/api/client";
import "@/i18n/locales/retrievalWorkbench";

export default function RetrievalDocumentSelect({
  datasetId,
  value = [],
  onChange,
  id,
}: {
  datasetId: string;
  value?: string[];
  onChange?: (value: string[]) => void;
  id?: string;
}) {
  const { t } = useTranslation("retrievalWorkbench");
  const [search, setSearch] = useState("");
  const [keywords, setKeywords] = useState("");
  const [selectedNames, setSelectedNames] = useState<Record<string, string>>(
    {},
  );
  useEffect(() => {
    const timer = setTimeout(() => {
      setKeywords(search.trim());
    }, 250);
    return () => {
      clearTimeout(timer);
    };
  }, [search]);
  const directory = useInfiniteQuery({
    queryKey: ["knowledge", "retrieval-directory", datasetId, keywords],
    initialPageParam: 1,
    queryFn: ({ pageParam }) =>
      knowledgeApi.documents.list(datasetId, {
        page: pageParam,
        page_size: 100,
        keywords,
      }),
    getNextPageParam: (last, pages) =>
      last.documents.length &&
      pages.reduce((n, p) => n + p.documents.length, 0) < last.total
        ? pages.length + 1
        : undefined,
  });
  const documents = useMemo(
    () => directory.data?.pages.flatMap((p) => p.documents) ?? [],
    [directory.data],
  );
  const options = new Map(documents.map((doc) => [doc.id, doc.name]));
  // Keep only selected cached labels; never broaden the visible search results.
  for (const key of value)
    if (!options.has(key)) options.set(key, selectedNames[key] ?? key);
  const total = directory.data?.pages.at(-1)?.total ?? 0;
  const loaded = documents.length;
  const incomplete = !directory.hasNextPage && loaded < total;
  const retry = () =>
    directory.isFetchNextPageError
      ? directory.fetchNextPage()
      : directory.refetch();
  return (
    <div>
      <Select
        id={id}
        style={{ width: "100%" }}
        mode="multiple"
        allowClear
        value={value}
        loading={directory.isFetching}
        popupRender={(menu) => (
          <>
            {menu}
            {directory.hasNextPage && (
              <div
                style={{
                  padding: "8px 12px",
                  borderTop: "1px solid var(--border)",
                }}
              >
                <Button
                  block
                  loading={directory.isFetchingNextPage}
                  onMouseDown={(event) => {
                    event.preventDefault();
                  }}
                  onClick={() => {
                    void directory.fetchNextPage();
                  }}
                >
                  {t("more")}
                </Button>
              </div>
            )}
          </>
        )}
        showSearch={{
          searchValue: search,
          filterOption: false,
          onSearch: setSearch,
        }}
        placeholder={t("sourcePlaceholder")}
        options={Array.from(options, ([optionValue, label]) => ({
          value: optionValue,
          label,
        }))}
        onChange={(next: string[]) => {
          setSelectedNames(
            Object.fromEntries(
              next.map((key) => [key, options.get(key) ?? key]),
            ),
          );
          onChange?.(next);
        }}
      />
      <div
        style={{
          display: "flex",
          alignItems: "center",
          flexWrap: "wrap",
          gap: 8,
          marginTop: 8,
        }}
      >
        <Typography.Text type="secondary" aria-live="polite">
          {t("loaded", { loaded, total })}
        </Typography.Text>
        {directory.hasNextPage && (
          <Button
            aria-label={t("morePage")}
            size="small"
            loading={directory.isFetchingNextPage}
            onClick={() => {
              void directory.fetchNextPage();
            }}
          >
            {t("more")}
          </Button>
        )}
      </div>
      {(directory.error ?? incomplete) && (
        <Alert
          style={{ marginTop: 8 }}
          type="warning"
          showIcon
          title={
            directory.error
              ? parseApiError(directory.error)
              : t("sourceIncomplete")
          }
          action={
            <Button
              size="small"
              loading={directory.isFetching}
              onClick={() => {
                void retry();
              }}
            >
              {t("retry")}
            </Button>
          }
        />
      )}
      <Typography.Text type="secondary">{t("sourceHint")}</Typography.Text>
    </div>
  );
}
