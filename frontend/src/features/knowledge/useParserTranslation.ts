import i18next from "@/i18n";
import { useTranslation } from "react-i18next";
import en from "./parserMessages.en.json";
import zh from "./parserMessages.zh.json";
i18next.addResourceBundle("en", "knowledgeParser", en);
i18next.addResourceBundle("zh", "knowledgeParser", zh);
export function useParserTranslation() {
  return useTranslation("knowledgeParser");
}
