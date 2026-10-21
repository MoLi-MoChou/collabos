import i18next, { type Resource } from "i18next";
import { initReactI18next } from "react-i18next";

/** Initializes i18next with resources supplied by packages/views. */
export function initI18n(resources: Resource, lng = "zh-Hans") {
  void i18next.use(initReactI18next).init({ resources, lng, fallbackLng: "en", interpolation: { escapeValue: false }, returnNull: false });
  return i18next;
}
