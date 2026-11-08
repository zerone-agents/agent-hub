import i18next from "@/i18n";
import { useTranslation } from "react-i18next";
import en from "./connectorMessages.en.json";
import zh from "./connectorMessages.zh.json";

i18next.addResourceBundle("en", "knowledgeConnectors", en);
i18next.addResourceBundle("zh", "knowledgeConnectors", zh);

export function useConnectorTranslation() {
  return useTranslation("knowledgeConnectors");
}
