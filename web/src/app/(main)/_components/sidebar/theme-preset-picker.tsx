"use client";

import { useI18n } from "@/i18n";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectGroup, SelectItem, SelectLabel, SelectTrigger, SelectValue } from "@/components/ui/select";
import { THEME_PRESET_OPTIONS, type ResolvedThemeMode, type ThemePreset } from "@/lib/preferences/theme";

export function ThemePresetPicker({ value, mode, onChange }: {
  value: ThemePreset;
  mode: ResolvedThemeMode;
  onChange: (preset: ThemePreset) => void;
}) {
  const { t } = useI18n();
  const selected = THEME_PRESET_OPTIONS.find((preset) => preset.value === value);
  return <div className="space-y-1.5">
    <Label id="theme-preset-label" htmlFor="theme-preset" className="font-medium text-xs">{t("english.e006")}</Label>
    <Select value={value} onValueChange={onChange}>
      <SelectTrigger id="theme-preset" aria-labelledby="theme-preset-label" size="sm" className="w-full text-xs">
        <SelectValue>{selected?.label}</SelectValue>
      </SelectTrigger>
      <SelectContent position="popper" className="max-h-[min(28rem,70dvh)] w-[var(--radix-select-trigger-width)] max-w-[calc(100vw-2rem)]">
        {(["cyber", "classic"] as const).map((family) => <SelectGroup key={family}>
          <SelectLabel className="font-mono text-[10px] uppercase tracking-wider">{t(family === "cyber" ? "app.cyberThemes" : "app.classicThemes")}</SelectLabel>
          {THEME_PRESET_OPTIONS.filter((preset) => preset.family === family).map((preset) => {
            const preview = preset.preview[mode];
            return <SelectItem key={preset.value} value={preset.value} textValue={preset.label} className={family === "cyber" ? "py-2 text-xs" : "py-1.5 text-xs"}>
              <span className="flex min-w-0 items-center gap-3">
                {family === "cyber" ? <span aria-hidden="true" className="flex h-10 w-14 shrink-0 gap-1 overflow-hidden rounded border p-1" style={{ backgroundColor: preview.background, borderColor: preview.border }}>
                  <span className="w-2 rounded-sm" style={{ backgroundColor: preview.surface }} />
                  <span className="flex flex-1 flex-col gap-1 pt-1">
                    <span className="h-1 w-4/5 rounded-sm" style={{ backgroundColor: preview.primary }} />
                    <span className="h-1 rounded-sm" style={{ backgroundColor: preview.border }} />
                    <span className="h-1 w-2/3 rounded-sm" style={{ backgroundColor: preview.border }} />
                  </span>
                </span> : <span aria-hidden="true" className="size-2.5 shrink-0 rounded-full" style={{ backgroundColor: preview.primary }} />}
                <span className="grid min-w-0 gap-1">
                  <span className="font-medium">{preset.label}</span>
                  {preset.description && <span className="whitespace-normal text-[11px] leading-snug text-muted-foreground">{preset.description}</span>}
                </span>
              </span>
            </SelectItem>;
          })}
        </SelectGroup>)}
      </SelectContent>
    </Select>
  </div>;
}
