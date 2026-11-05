import type { Locale } from "./config";

const usesCornerQuotes = (locale: Locale): boolean => locale === "ja" || locale.startsWith("zh-");

export function openingQuote(locale: Locale): string {
  if (usesCornerQuotes(locale)) return "「";
  return locale === "es" ? "«" : "“";
}

export function closingQuote(locale: Locale): string {
  if (usesCornerQuotes(locale)) return "」";
  return locale === "es" ? "»" : "”";
}

export function quoted(value: string, locale: Locale): string {
  return openingQuote(locale) + value + closingQuote(locale);
}

export function listSeparator(locale: Locale): string {
  return usesCornerQuotes(locale) ? "、" : ", ";
}

export function questionMark(locale: Locale): string {
  return usesCornerQuotes(locale) ? "？" : "?";
}

export function sentenceStop(locale: Locale): string {
  return usesCornerQuotes(locale) ? "。" : ".";
}

export function bracketLabel(label: string, locale: Locale): string {
  return usesCornerQuotes(locale) ? `【${label}】` : `[${label}]`;
}
