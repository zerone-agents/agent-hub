/** Session-only drafts: File objects and durable recovery IDs survive sheet closure. */
export function createKnowledgeDraftStore() {
  let identity: string | undefined;
  const drafts = new Map<string, unknown>();
  const scope = (token?: string) => {
    if (token !== identity) {
      drafts.clear();
      identity = token;
    }
  };
  return {
    read<T>(token: string | undefined, key: string, initial: () => T): T {
      scope(token);
      if (!drafts.has(key)) drafts.set(key, initial());
      return drafts.get(key) as T;
    },
    write<T>(token: string | undefined, key: string, value: T) {
      scope(token);
      drafts.set(key, value);
    },
    remove(token: string | undefined, key: string) {
      scope(token);
      drafts.delete(key);
    },
    clear() {
      drafts.clear();
      identity = undefined;
    },
  };
}

export function desktopDocumentUrl(
  base: string | undefined,
  datasetId: string,
  documentId: string,
): string | undefined {
  if (!base?.trim()) return;
  try {
    const url = new URL(base);
    if (
      !["https:", "http:"].includes(url.protocol) ||
      url.username ||
      url.password
    )
      return;
    url.pathname = `${url.pathname.replace(/\/$/, "")}/knowledge/${encodeURIComponent(datasetId)}/documents/${encodeURIComponent(documentId)}/chunks`;
    url.search = "";
    url.hash = "";
    return url.href;
  } catch {
    return;
  }
}
