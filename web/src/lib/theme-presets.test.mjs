import assert from "node:assert/strict";
import fs from "node:fs";
import test from "node:test";
import { THEME_PRESET_OPTIONS } from "./preferences/theme.ts";
import { __resetLocaleForTests, setLocale, translateIn } from "../i18n/runtime.ts";

function luminance(hex) {
  const channels = hex.slice(1).match(/../g).map((value) => parseInt(value, 16) / 255)
    .map((value) => value <= 0.04045 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4);
  return channels[0] * 0.2126 + channels[1] * 0.7152 + channels[2] * 0.0722;
}
function contrast(a, b) {
  const values = [luminance(a), luminance(b)].sort((x, y) => y - x);
  return (values[0] + 0.05) / (values[1] + 0.05);
}

test("cybersecurity themes keep normal text readable in both appearances", () => {
  const cyber = THEME_PRESET_OPTIONS.filter((preset) => preset.family === "cyber");
  assert.deepEqual(cyber.map((preset) => preset.value), ["aptex-obsidian", "aptex-phosphor", "aptex-afterglow"]);
  for (const preset of cyber) {
    const css = fs.readFileSync(new URL(`../styles/presets/${preset.value}.css`, import.meta.url), "utf8");
    const blocks = [...css.matchAll(/\{([^}]+)\}/g)];
    assert.equal(blocks.length, 2);
    for (const [index, [, block]] of blocks.entries()) {
      const values = Object.fromEntries([...block.matchAll(/--([\w-]+):\s*(#[\da-f]{6});/g)].map(([, key, value]) => [key, value]));
      for (const [text, surface] of [["foreground", "background"], ["card-foreground", "card"], ["muted-foreground", "card"], ["primary-foreground", "primary"], ["sidebar-foreground", "sidebar"]]) {
        assert.ok(contrast(values[text], values[surface]) >= 4.5, `${preset.value} ${index ? "dark" : "light"}: ${text} on ${surface}`);
      }
      const mode = index ? "dark" : "light";
      assert.equal(preset.primary[mode], values.primary);
      assert.equal(preset.preview[mode].background, values.background);
    }
  }
});

test("generated presets retain named themes and localize descriptions and classic labels", () => {
  __resetLocaleForTests();
  try {
    setLocale("ko");
    assert.equal(THEME_PRESET_OPTIONS.find((p) => p.value === "default").label, translateIn("ko", "english.e188"));
    for (const [value, name] of [["aptex-obsidian", "Obsidian"], ["aptex-phosphor", "Phosphor"], ["aptex-afterglow", "Afterglow"]]) {
      const preset = THEME_PRESET_OPTIONS.find((p) => p.value === value);
      assert.equal(preset.label, name);
      assert.equal(preset.description, translateIn("ko", `app.theme${name}Description`));
      assert.notEqual(preset.description, translateIn("en", `app.theme${name}Description`));
    }
  } finally { __resetLocaleForTests(); }
});
