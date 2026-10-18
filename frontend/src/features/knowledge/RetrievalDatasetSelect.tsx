import { useState } from "react";
import { useInfiniteQuery } from "@tanstack/react-query";
import { Alert, Button, Select, Typography } from "antd";
import { useTranslation } from "react-i18next";
import { knowledgeApi } from "@/api/knowledge";
import RetrievalDocumentSelect from "./RetrievalDocumentSelect";

export default function RetrievalDatasetSelect({
  currentId,
  value = [currentId],
  onChange,
  onScopeChange,
  id,
}: {
  currentId: string;
  value?: string[];
  onChange?: (value: string[]) => void;
  onScopeChange?: () => void;
  id?: string;
}) {
  const { t } = useTranslation("retrievalWorkbench");
  const directory = useInfiniteQuery({
    queryKey: ["knowledge", "retrieval-datasets"],
    initialPageParam: 1,
    queryFn: ({ pageParam }) =>
      knowledgeApi.datasets.list({ page: pageParam, page_size: 100 }),
    getNextPageParam: (last, pages) =>
      last.datasets.length &&
      pages.reduce((n, p) => n + p.datasets.length, 0) < last.total
        ? pages.length + 1
        : undefined,
    retry: false,
  });
  const options = new Map(
    directory.data?.pages.flatMap((page) =>
      page.datasets.map((dataset) => [dataset.id, dataset.name] as const),
    ),
  );
  for (const selected of value)
    if (!options.has(selected)) options.set(selected, selected);
  const incomplete =
    !directory.hasNextPage &&
    directory.data &&
    directory.data.pages.reduce((n, p) => n + p.datasets.length, 0) <
      (directory.data.pages.at(-1)?.total ?? 0);
  return (
    <div>
      <Select
        id={id}
        mode="multiple"
        allowClear
        showSearch={{ optionFilterProp: "label" }}
        style={{ width: "100%" }}
        value={value}
        loading={directory.isFetching}
        options={Array.from(options, ([value, label]) => ({ value, label }))}
        onChange={(next: string[]) => {
          onScopeChange?.();
          onChange?.(next);
        }}
      />
      {directory.hasNextPage && (
        <Button
          onClick={() => {
            void directory.fetchNextPage();
          }}
          loading={directory.isFetchingNextPage}
        >
          {t("datasetsMore")}
        </Button>
      )}
      {(directory.error ?? incomplete) && (
        <Alert
          type="warning"
          showIcon
          title={t("datasetsLoadError")}
          action={
            <Button
              onClick={() => {
                void directory.refetch();
              }}
            >
              {t("retry")}
            </Button>
          }
        />
      )}
    </div>
  );
}

export function RetrievalScopeDocuments({
  datasetIds,
  value = [],
  onChange,
  id,
}: {
  datasetIds: string[];
  value?: string[];
  onChange?: (value: string[]) => void;
  id?: string;
}) {
  const [byDataset, setByDataset] = useState<Record<string, string[]>>({});
  if (datasetIds.length === 1)
    return (
      <RetrievalDocumentSelect
        id={id}
        datasetId={datasetIds[0]}
        value={value}
        onChange={onChange}
      />
    );
  return (
    <div>
      {datasetIds.map((datasetId, index) => (
        <div key={datasetId} style={{ marginBottom: 12 }}>
          <Typography.Text>{datasetId}</Typography.Text>
          <RetrievalDocumentSelect
            id={index === 0 ? id : undefined}
            datasetId={datasetId}
            value={(byDataset[datasetId] ?? []).filter((doc) =>
              value.includes(doc),
            )}
            onChange={(documents) => {
              const next = { ...byDataset, [datasetId]: documents };
              setByDataset(next);
              onChange?.([
                ...new Set(datasetIds.flatMap((kb) => next[kb] ?? [])),
              ]);
            }}
          />
        </div>
      ))}
    </div>
  );
}
