"use client";

import * as React from "react";
import { toast } from "sonner";
import { useI18n } from "@/i18n";
import { api } from "@/lib/api";
import type { ChatGPTModel, ChatGPTStatus, LLMProfile } from "@/lib/types";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { OpenAIModelControls } from "./openai-model-controls";
import { defaultOpenAIAccess, openAIAccessProgram } from "@/lib/openai-models";

export function ChatGPTPanel({ profiles, editing, onSaved }: {
  profiles: LLMProfile[];
  editing: LLMProfile | null;
  onSaved: () => void;
}) {
  const { t } = useI18n();
  const [status, setStatus] = React.useState<ChatGPTStatus | null>(null);
  const [accountID, setAccountID] = React.useState("");
  const [models, setModels] = React.useState<ChatGPTModel[]>([]);
  const [model, setModel] = React.useState("gpt-6-sol");
  const [accessProgram, setAccessProgram] = React.useState("standard");
  const [effort, setEffort] = React.useState("");
  const [name, setName] = React.useState("");
  const [profileID, setProfileID] = React.useState("");
  const [busy, setBusy] = React.useState(false);
  const [modelsBusy, setModelsBusy] = React.useState(false);

  const loadStatus = React.useCallback(async () => {
    try {
      const next = await api.chatgptStatus();
      setStatus(next);
      setAccountID((current) => next.accounts.some((a) => a.id === current && a.connected && a.plan_enabled)
        ? current : next.accounts.find((a) => a.connected && a.plan_enabled)?.id ?? "");
    } catch (error) {
      toast.error((error as Error).message);
    }
  }, []);

  React.useEffect(() => { void loadStatus(); }, [loadStatus]);
  React.useEffect(() => {
    if (!status?.pending) return;
    const timer = window.setInterval(() => { void loadStatus(); }, 3000);
    return () => window.clearInterval(timer);
  }, [status?.pending, loadStatus]);
  React.useEffect(() => {
    if (!editing || editing.auth_mode !== "chatgpt_plan") return;
    setProfileID(editing.id);
    setName(editing.name);
    setAccountID(editing.chatgpt_account_id ?? "");
    setModel(editing.model);
    setAccessProgram(editing.access_program_cyber || "standard");
    setEffort(editing.reasoning_effort || "");
  }, [editing]);
  React.useEffect(() => {
    if (!accountID) { setModels([]); return; }
    let live = true;
    setModelsBusy(true);
    void api.chatgptModels(accountID).then((items) => {
      if (!live) return;
      setModels(items);
      setModel((current) => current || "gpt-6-sol");
    }).catch((error) => {
      if (live) { setModels([]); toast.error((error as Error).message); }
    }).finally(() => { if (live) setModelsBusy(false); });
    return () => { live = false; };
  }, [accountID]);

  async function connect(id?: string) {
    const popup = window.open("about:blank", "_blank");
    if (popup) popup.opener = null;
    setBusy(true);
    try {
      const login = await api.chatgptLogin(id);
      if (popup) popup.location.replace(login.url);
      else window.location.assign(login.url);
      toast.message(t("chatgpt.openLogin"));
      await loadStatus();
    } catch (error) {
      popup?.close();
      toast.error((error as Error).message);
    } finally { setBusy(false); }
  }

  async function disconnect(id: string) {
    setBusy(true);
    try {
      const result = await api.chatgptLogout(id);
      if (result.warning) toast.warning(result.warning);
      await loadStatus();
    } catch (error) { toast.error((error as Error).message); }
    finally { setBusy(false); }
  }

  async function save() {
    if (!accountID || !model || !name.trim()) { toast.error(t("chatgpt.selectAccountModel")); return; }
    setBusy(true);
    try {
      const { id } = await api.saveLLMProfile({
        id: profileID ? Number(profileID) : undefined,
        name: name.trim(), format: "openai-responses", auth_mode: "chatgpt_plan",
        chatgpt_account_id: accountID, model, pool_exclude: true, streaming: true,
        access_program_cyber: openAIAccessProgram(model, accessProgram), reasoning_effort: effort,
      });
      setProfileID(String(id));
      toast.success(t("chatgpt.saved"));
      onSaved();
    } catch (error) { toast.error((error as Error).message); }
    finally { setBusy(false); }
  }

  async function recheck() {
    if (!profileID) return;
    setBusy(true);
    try {
      const result = await api.chatgptRecheck(profileID);
      if (result.ok) toast.success(result.reply ?? t("chatgpt.statusConnected"));
      else toast.error(result.error ?? t("chatgpt.statusBlocked"));
      await loadStatus();
      onSaved();
    } catch (error) { toast.error((error as Error).message); }
    finally { setBusy(false); }
  }

  const planProfiles = profiles.filter((p) => p.auth_mode === "chatgpt_plan");
  return <Card id="chatgpt-panel">
    <CardHeader><CardTitle>{t("chatgpt.title")}</CardTitle>
      <p className="text-sm text-muted-foreground">{t("chatgpt.help")}</p>
    </CardHeader>
    <CardContent className="grid gap-4">
      <div className="flex flex-wrap items-center gap-2">
        <Button type="button" variant="outline" disabled={busy} onClick={() => void connect()}>{t("chatgpt.connect")}</Button>
        {status?.accounts.map((a) => <div key={a.id} className="flex min-w-0 flex-wrap items-center gap-2 rounded-md border px-2 py-1 text-sm">
          <span className="min-w-0 break-all">{a.email || a.id}</span>
          <Badge variant="outline">{a.plan_blocked ? t("chatgpt.statusBlocked") : a.connected && a.plan_enabled ? t("chatgpt.statusConnected") : t("chatgpt.statusSignedOut")}</Badge>
          <Button size="sm" variant="ghost" disabled={busy} onClick={() => void connect(a.id)}>{t("chatgpt.reconnect")}</Button>
          <Button size="sm" variant="ghost" disabled={busy} onClick={() => void disconnect(a.id)}>{t("chatgpt.disconnect")}</Button>
        </div>)}
      </div>
      {status?.error && <p role="alert" className="text-sm text-destructive">{status.error}</p>}
      {!accountID && <p className="text-sm text-muted-foreground">{t("chatgpt.noAccount")}</p>}
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <div className="grid min-w-0 gap-1"><Label htmlFor="chatgpt-profile">{t("chatgpt.profile")}</Label>
          <select id="chatgpt-profile" className="h-9 w-full min-w-0 rounded-md border bg-background px-2 text-sm" value={profileID} onChange={(e) => {
            const p = planProfiles.find((item) => item.id === e.target.value);
            setProfileID(p?.id ?? ""); setName(p?.name ?? ""); setAccountID(p?.chatgpt_account_id ?? accountID); setModel(p?.model ?? "gpt-6-sol"); setAccessProgram(p?.access_program_cyber || "standard"); setEffort(p?.reasoning_effort || "");
          }}><option value="">{t("chatgpt.newProfile")}</option>{planProfiles.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}</select>
          <Input aria-label={t("chatgpt.profile")} value={name} onChange={(e) => setName(e.target.value)} placeholder={t("chatgpt.profile")} />
        </div>
        <div className="grid min-w-0 gap-1"><Label htmlFor="chatgpt-account">{t("chatgpt.account")}</Label>
          <select id="chatgpt-account" className="h-9 w-full min-w-0 rounded-md border bg-background px-2 text-sm" value={accountID} onChange={(e) => setAccountID(e.target.value)}>
            <option value="">{t("chatgpt.account")}</option>{status?.accounts.filter((a) => a.connected && a.plan_enabled).map((a) => <option key={a.id} value={a.id}>{a.email || a.id}</option>)}
          </select>
        </div>
        <div className="grid min-w-0 gap-1"><Label htmlFor="chatgpt-model">{t("chatgpt.model")}</Label>
          <OpenAIModelControls id="chatgpt" model={model} accessProgram={accessProgram} onAccessChange={setAccessProgram}
            onModelChange={(next) => { setModel(next); setAccessProgram(defaultOpenAIAccess(next)); }} catalog={models} />
          <Input id="chatgpt-model" aria-label={t("chatgpt.model")} value={model} onChange={(event) => { setModel(event.target.value); setAccessProgram(defaultOpenAIAccess(event.target.value)); }} />
          {accountID && !modelsBusy && models.length === 0 && <span className="text-xs text-muted-foreground">{t("chatgpt.noModels")}</span>}
        </div>
        <div className="flex flex-wrap items-end gap-2">
          <Button disabled={busy || !accountID || !model || !name.trim()} onClick={() => void save()}>{profileID ? t("chatgpt.update") : t("chatgpt.create")}</Button>
          {profileID && <Button variant="outline" disabled={busy} onClick={() => void recheck()}>{t("chatgpt.recheck")}</Button>}
        </div>
      </div>
    </CardContent>
  </Card>;
}
