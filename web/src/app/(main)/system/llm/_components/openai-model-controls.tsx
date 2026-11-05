"use client";

import { useI18n } from "@/i18n";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { openAIAccessProgram, openAIModelChoices, supportsDaybreakBlue } from "@/lib/openai-models";

export function OpenAIModelControls({ model, accessProgram, onModelChange, onAccessChange, catalog = [], id = "openai" }: {
  model: string;
  accessProgram: string;
  onModelChange: (model: string) => void;
  onAccessChange: (program: string) => void;
  catalog?: readonly { id: string; name?: string }[];
  id?: string;
}) {
  const { t } = useI18n();
  const choices = openAIModelChoices(catalog);
  const blue = supportsDaybreakBlue(model);
  return <div className="grid min-w-0 gap-3 rounded-md border p-3">
    <div className="grid min-w-0 gap-2">
      <Label htmlFor={`${id}-model-choice`}>{t("providers.openaiModel")}</Label>
      <select id={`${id}-model-choice`} className="h-9 w-full min-w-0 rounded-md border bg-background px-3 text-sm" value={model} onChange={(event) => onModelChange(event.target.value)}>
        {!choices.some((choice) => choice.id === model) && <option value={model}>{model || t("providers.chooseModel")}</option>}
        {choices.map((choice) => <option key={choice.id} value={choice.id}>{choice.name}</option>)}
      </select>
      <p className="text-xs text-muted-foreground">{t("providers.openaiModelHelp")}</p>
    </div>
    {blue ? <div className="flex items-center justify-between gap-3">
      <div className="grid min-w-0 gap-1">
        <Label htmlFor={`${id}-daybreak-blue`}>{t("providers.daybreakBlue")}</Label>
        <p className="text-xs text-muted-foreground">{t("providers.daybreakBlueHelp")}</p>
      </div>
      <Switch className="shrink-0" id={`${id}-daybreak-blue`} checked={openAIAccessProgram(model, accessProgram) === "daybreak_blue"} onCheckedChange={(checked) => onAccessChange(checked ? "daybreak_blue" : "standard")} />
    </div> : model ? <p className="text-xs text-muted-foreground">{t("providers.standardAccessOnly")}</p> : null}
  </div>;
}
