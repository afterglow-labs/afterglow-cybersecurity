import { translate as swt } from "../../i18n/runtime.ts";
export const THEME_MODE_OPTIONS = [
  {
    get label() {
      return swt("english.e012");
    },
    value: "light",
  },
  {
    get label() {
      return swt("english.e014");
    },
    value: "dark",
  },
  {
    get label() {
      return swt("english.e016");
    },
    value: "system",
  },
] as const;

export const THEME_MODE_VALUES = THEME_MODE_OPTIONS.map((o) => o.value);
export type ThemeMode = (typeof THEME_MODE_VALUES)[number];
export type ResolvedThemeMode = "light" | "dark";

// --- generated:themePresets:start ---
export const THEME_PRESET_OPTIONS = [
  {
    get label() {
      return swt("app.themeObsidian");
    },
    get description() {
      return swt("app.themeObsidianDescription");
    },
    value: "aptex-obsidian",
    family: "cyber",
    primary: { light: "#0e7490", dark: "#67e8f9" },
    preview: {
      light: { primary: "#0e7490", background: "#f5f8fa", surface: "#ffffff", border: "#c5d1d9" },
      dark: { primary: "#67e8f9", background: "#090e13", surface: "#111920", border: "#2d3c48" },
    },
  },
  {
    get label() {
      return swt("app.themePhosphor");
    },
    get description() {
      return swt("app.themePhosphorDescription");
    },
    value: "aptex-phosphor",
    family: "cyber",
    primary: { light: "#167844", dark: "#5ceba2" },
    preview: {
      light: { primary: "#167844", background: "#f4f8f5", surface: "#ffffff", border: "#c4d5c8" },
      dark: { primary: "#5ceba2", background: "#080e0c", surface: "#101b16", border: "#2c4035" },
    },
  },
  {
    get label() {
      return swt("app.themeAfterglow");
    },
    get description() {
      return swt("app.themeAfterglowDescription");
    },
    value: "aptex-afterglow",
    family: "cyber",
    primary: { light: "#a45216", dark: "#f5b75c" },
    preview: {
      light: { primary: "#a45216", background: "#fbf8f2", surface: "#fffdf9", border: "#dbcfbd" },
      dark: { primary: "#f5b75c", background: "#120f0b", surface: "#1c1812", border: "#4a3c2b" },
    },
  },
  {
    get label() {
      return swt("english.e188");
    },
    get description() {
      return "";
    },
    value: "default",
    family: "classic",
    primary: { light: "oklch(0.205 0 0)", dark: "oklch(0.922 0 0)" },
    preview: {
      light: {
        primary: "oklch(0.205 0 0)",
        background: "oklch(1 0 0)",
        surface: "oklch(1 0 0)",
        border: "oklch(0.922 0 0)",
      },
      dark: {
        primary: "oklch(0.922 0 0)",
        background: "oklch(0.145 0 0)",
        surface: "oklch(0.205 0 0)",
        border: "oklch(1 0 0 / 10%)",
      },
    },
  },
  {
    get label() {
      return swt("english.e189");
    },
    get description() {
      return "";
    },
    value: "brutalist",
    family: "classic",
    primary: { light: "oklch(0.6489 0.237 26.9728)", dark: "oklch(0.7044 0.1872 23.1858)" },
    preview: {
      light: {
        primary: "oklch(0.6489 0.237 26.9728)",
        background: "oklch(1 0 0)",
        surface: "oklch(1 0 0)",
        border: "oklch(0 0 0)",
      },
      dark: {
        primary: "oklch(0.7044 0.1872 23.1858)",
        background: "oklch(0 0 0)",
        surface: "oklch(0.3211 0 0)",
        border: "oklch(1 0 0)",
      },
    },
  },
  {
    get label() {
      return swt("english.e190");
    },
    get description() {
      return "";
    },
    value: "soft-pop",
    family: "classic",
    primary: { light: "oklch(0.5106 0.2301 276.9656)", dark: "oklch(0.6801 0.1583 276.9349)" },
    preview: {
      light: {
        primary: "oklch(0.5106 0.2301 276.9656)",
        background: "oklch(0.9789 0.0082 121.6272)",
        surface: "oklch(1 0 0)",
        border: "oklch(0 0 0)",
      },
      dark: {
        primary: "oklch(0.6801 0.1583 276.9349)",
        background: "oklch(0 0 0)",
        surface: "oklch(0.2455 0.0217 257.2823)",
        border: "oklch(0.4459 0 0)",
      },
    },
  },
  {
    get label() {
      return swt("english.e191");
    },
    get description() {
      return "";
    },
    value: "tangerine",
    family: "classic",
    primary: { light: "oklch(0.64 0.17 36.44)", dark: "oklch(0.64 0.17 36.44)" },
    preview: {
      light: {
        primary: "oklch(0.64 0.17 36.44)",
        background: "oklch(0.94 0 236.5)",
        surface: "oklch(1 0 0)",
        border: "oklch(0.9 0.01 247.88)",
      },
      dark: {
        primary: "oklch(0.64 0.17 36.44)",
        background: "oklch(0.26 0.03 262.67)",
        surface: "oklch(0.31 0.03 268.64)",
        border: "oklch(0.38 0.03 269.73)",
      },
    },
  },
] as const;
export const THEME_PRESET_VALUES = THEME_PRESET_OPTIONS.map((p) => p.value);
export type ThemePreset = (typeof THEME_PRESET_OPTIONS)[number]["value"];
// --- generated:themePresets:end ---
