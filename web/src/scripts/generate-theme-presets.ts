/** Generate localized preset metadata from the checkout's CSS token files. */
import { execFileSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";

const presetDir = path.resolve(__dirname, "../styles/presets");
const outputPath = path.resolve(__dirname, "../lib/preferences/theme.ts");
const globalPath = path.resolve(__dirname, "../app/globals.css");
for (const required of [presetDir, outputPath, globalPath]) {
  if (!fs.existsSync(required)) throw new Error(`Required theme source is missing: ${required}`);
}

const classicLabels: Record<string, string> = {
  default: "english.e188", brutalist: "english.e189", "soft-pop": "english.e190", tangerine: "english.e191",
};

function property(block: string, name: string): string {
  const value = block.match(new RegExp(`--${name}:\\s*([^;]+);`))?.[1]?.trim();
  if (!value) throw new Error(`Theme is missing --${name}`);
  return value;
}

function palette(block: string) {
  return { primary: property(block, "primary"), background: property(block, "background"), surface: property(block, "card"), border: property(block, "border") };
}

const global = fs.readFileSync(globalPath, "utf8");
const defaultLight = global.match(/:root\s*\{([^}]+)\}/)?.[1] ?? "";
const defaultDark = global.match(/\.dark\s*\{([^}]+)\}/)?.[1] ?? "";
const presets = fs.readdirSync(presetDir).filter((file) => file.endsWith(".css")).sort().map((file) => {
  const css = fs.readFileSync(path.join(presetDir, file), "utf8");
  const metadata = (name: string) => css.match(new RegExp(`^${name}:\\s*(.+)$`, "m"))?.[1]?.trim();
  const value = metadata("value");
  const label = metadata("label");
  if (!value || !label) throw new Error(`Theme ${file} requires label and value metadata`);
  const blocks = [...css.matchAll(/([^{}]+)\{([^{}]+)\}/g)];
  const light = blocks.find(([_, selector]) => selector.includes(`:root[data-theme-preset="${value}"]`) && !selector.includes(".dark"))?.[2] ?? "";
  const dark = blocks.find(([_, selector]) => selector.includes(`.dark:root[data-theme-preset="${value}"]`))?.[2] ?? "";
  return { value, label, labelKey: metadata("labelKey") ?? classicLabels[value], descriptionKey: metadata("descriptionKey"), family: metadata("family") ?? "classic", light: palette(light), dark: palette(dark) };
});
presets.push({ value: "default", label: "Default", labelKey: classicLabels.default, descriptionKey: undefined, family: "classic", light: palette(defaultLight), dark: palette(defaultDark) });
const order = ["aptex-obsidian", "aptex-phosphor", "aptex-afterglow", "default", "brutalist", "soft-pop", "tangerine"];
presets.sort((a, b) => {
  const ai = order.indexOf(a.value); const bi = order.indexOf(b.value);
  return (ai < 0 ? order.length : ai) - (bi < 0 ? order.length : bi) || a.value.localeCompare(b.value);
});
const q = JSON.stringify;
const generated = presets.map((p) => `{
  get label() { return ${p.labelKey ? `swt(${q(p.labelKey)})` : q(p.label)}; },
  get description() { return ${p.descriptionKey ? `swt(${q(p.descriptionKey)})` : '""'}; },
  value: ${q(p.value)}, family: ${q(p.family)},
  primary: ${q({ light: p.light.primary, dark: p.dark.primary })},
  preview: ${q({ light: p.light, dark: p.dark })},
}`).join(",\n");
const block = `// --- generated:themePresets:start ---
export const THEME_PRESET_OPTIONS = [${generated}] as const;
export const THEME_PRESET_VALUES = THEME_PRESET_OPTIONS.map((p) => p.value);
export type ThemePreset = (typeof THEME_PRESET_OPTIONS)[number]["value"];
// --- generated:themePresets:end ---`;
const original = fs.readFileSync(outputPath, "utf8");
const markers = /\/\/ --- generated:themePresets:start ---[\s\S]*?\/\/ --- generated:themePresets:end ---/;
if (!markers.test(original)) throw new Error(`Theme generation markers are missing: ${outputPath}`);
const result = execFileSync(process.execPath, [require.resolve("@biomejs/biome/bin/biome"), "format", "--stdin-file-path", outputPath], { input: original.replace(markers, block), encoding: "utf8" });
if (result !== original) fs.writeFileSync(outputPath, result);
console.log(`Theme presets verified: ${presets.length}`);
