import React, { useEffect, useRef } from "react";
import { X } from "lucide-react";
import "./KnowledgeMobile.css";

/** Keep keyboard focus and scroll inside the mobile knowledge sheet. */
export function KnowledgeDialog({
  title,
  onClose,
  busy = false,
  children,
}: {
  title: string;
  onClose: () => void;
  busy?: boolean;
  children: React.ReactNode;
}) {
  const dialog = useRef<HTMLDivElement>(null);
  const close = useRef(onClose);
  close.current = onClose;
  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null;
    dialog.current?.focus();
    return () => {
      if (previous?.isConnected) previous.focus();
    };
  }, []);
  useEffect(() => {
    // Disabling the submitting button can move focus to the page body.
    if (dialog.current && !dialog.current.contains(document.activeElement))
      dialog.current.focus();
  }, [busy]);
  return (
    <div className="knowledge-mobile absolute inset-0 z-50 bg-black/40 flex items-end sm:items-center justify-center p-2 sm:p-4">
      <div
        ref={dialog}
        role="dialog"
        aria-modal="true"
        aria-label={title}
        tabIndex={-1}
        onKeyDown={(event) => {
          if (event.key === "Escape" && !busy) {
            event.stopPropagation();
            close.current();
          }
          if (event.key !== "Tab") return;
          const nodes = (
            Array.from(
              dialog.current?.querySelectorAll<HTMLElement>(
                'button:not(:disabled), input:not(:disabled), select:not(:disabled), textarea:not(:disabled), a[href], [tabindex="0"]',
              ) || [],
            ) as HTMLElement[]
          ).filter((node) => node.getClientRects().length);
          const first = nodes[0];
          const last = nodes.at(-1);
          if (!first) {
            event.preventDefault();
            return;
          }
          if (
            event.shiftKey &&
            (document.activeElement === first ||
              document.activeElement === dialog.current)
          ) {
            event.preventDefault();
            last?.focus();
          } else if (
            !event.shiftKey &&
            (document.activeElement === last ||
              document.activeElement === dialog.current)
          ) {
            event.preventDefault();
            first.focus();
          }
        }}
        className="w-full max-w-md bg-white rounded-2xl shadow-xl border border-gray-200 flex flex-col max-h-full min-h-0 overflow-hidden text-gray-900"
      >
        <div className="px-4 py-2 border-b border-gray-100 flex items-center justify-between shrink-0">
          <h2 className="text-sm font-bold">{title}</h2>
          <button
            type="button"
            aria-label="关闭"
            disabled={busy}
            onClick={onClose}
            className="rounded-xl text-gray-500"
          >
            <X className="w-5 h-5 mx-auto" />
          </button>
        </div>
        <div className="overflow-y-auto min-h-0">{children}</div>
      </div>
    </div>
  );
}
