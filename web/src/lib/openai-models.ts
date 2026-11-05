// Model suggestions do not assert that an account can use a model. The provider
// checks access when a request is sent. This installation exposes Blue controls.
export const OPENAI_MODELS = [
  { id: "gpt-6-astra", name: "GPT-6 Astra" },
  { id: "gpt-6.1-sol", name: "GPT-6.1 Sol" },
  { id: "gpt-6-sol", name: "GPT-6 Sol" },
  { id: "gpt-5.6-sol", name: "GPT-5.6 Sol" },
  { id: "gpt-daybreak-blue-latest", name: "Daybreak Blue Latest" },
  { id: "gpt-6-luna", name: "GPT-6 Luna" },
] as const;

export function supportsDaybreakBlue(model: string): boolean {
  return model === "gpt-daybreak-blue-latest" || /^gpt-(?:6|5\.6)-sol(?:-\d{4}-\d{2}-\d{2})?$/.test(model);
}

export function openAIAccessProgram(model: string, selected = "standard"): "standard" | "daybreak_blue" {
  return supportsDaybreakBlue(model) && selected === "daybreak_blue" ? "daybreak_blue" : "standard";
}

export function defaultOpenAIAccess(model: string): string {
  return model === "gpt-daybreak-blue-latest" ? "daybreak_blue" : "standard";
}

export function openAIModelChoices(catalog: readonly { id: string; name?: string }[] = []) {
  const choices = new Map<string, { id: string; name: string }>(OPENAI_MODELS.map((model) => [model.id, { ...model }]));
  for (const model of catalog) choices.set(model.id, { id: model.id, name: model.name || choices.get(model.id)?.name || model.id });
  return [...choices.values()];
}

export function isOpenAIEndpoint(baseURL: string): boolean {
  if (!baseURL.trim()) return true;
  try {
    const url = new URL(baseURL);
    return url.protocol === "https:" && url.hostname === "api.openai.com" && !url.username && !url.password;
  } catch { return false; }
}
