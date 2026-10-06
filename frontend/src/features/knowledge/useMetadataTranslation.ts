import i18next from "@/i18n";
import { useTranslation } from "react-i18next";
import en from "./metadataMessages.en.json";
import zh from "./metadataMessages.zh.json";
i18next.addResourceBundle("en", "knowledgeMetadata", en);
i18next.addResourceBundle("zh", "knowledgeMetadata", zh);
export function useMetadataTranslation() {
  return useTranslation("knowledgeMetadata");
}
