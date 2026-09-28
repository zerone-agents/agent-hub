import Table from "cli-table3";
import { writeStdout } from "./stdout";

export function outputTable(rows: Record<string, unknown>[], columns: string[]): void {
  if (rows.length === 0) {
    writeStdout("(无数据)\n");
    return;
  }
  const table = new Table({ head: columns });
  for (const row of rows) {
    table.push(columns.map((c) => formatCell(row[c])));
  }
  writeStdout(table.toString() + "\n");
}

function formatCell(v: unknown): string {
  if (v === null || v === undefined) return "-";
  if (typeof v === "boolean") return v ? "✓" : "✗";
  if (typeof v === "object") return JSON.stringify(v);
  return String(v);
}
