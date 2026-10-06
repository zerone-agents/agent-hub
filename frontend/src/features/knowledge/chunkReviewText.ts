import i18next from "@/i18n";
import { useTranslation } from "react-i18next";

export function chunkReviewText(key: string): string {
  return i18next.t(`knowledge.chunks.review.${key}`);
}
export function useChunkReviewText() {
  useTranslation();
  return chunkReviewText;
}
