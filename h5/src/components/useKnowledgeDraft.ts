import { useEffect, useState } from "react";
import { AUTH_CHANGED_EVENT, getStoredAuth } from "../api/auth";
import { createKnowledgeDraftStore } from "./knowledgeMobileState";

const drafts = createKnowledgeDraftStore();

export function useKnowledgeDraft<T>(
  key: string,
  initial: () => T,
): [T, (update: T | ((draft: T) => T)) => void, () => void] {
  const token = getStoredAuth()?.token;
  const [, refresh] = useState(0);
  useEffect(() => {
    const changed = (event: Event) => {
      if (event instanceof StorageEvent && event.key !== "zerone_auth") return;
      drafts.clear();
      refresh((value) => value + 1);
    };
    window.addEventListener(AUTH_CHANGED_EVENT, changed);
    window.addEventListener("storage", changed);
    return () => {
      window.removeEventListener(AUTH_CHANGED_EVENT, changed);
      window.removeEventListener("storage", changed);
    };
  }, []);
  const draft = drafts.read(token, key, initial);
  return [
    draft,
    (update) => {
      // An old asynchronous operation must not write a new user's drafts.
      if (getStoredAuth()?.token !== token) return;
      drafts.write(
        token,
        key,
        typeof update === "function"
          ? (update as (draft: T) => T)(drafts.read(token, key, initial))
          : update,
      );
      refresh((value) => value + 1);
    },
    () => {
      if (getStoredAuth()?.token !== token) return;
      drafts.remove(token, key);
      refresh((value) => value + 1);
    },
  ];
}
