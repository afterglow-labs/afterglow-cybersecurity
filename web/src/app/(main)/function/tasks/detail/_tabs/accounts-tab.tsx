"use client";

import * as React from "react";
import { toast } from "sonner";
import { useI18n } from "@/i18n";
import { api } from "@/lib/api";
import type { TaskCredential } from "@/lib/types";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

export function AccountsTab({ taskId }: { taskId: string }) {
  const { t } = useI18n();
  const [accounts, setAccounts] = React.useState<TaskCredential[]>([]);
  const [form, setForm] = React.useState({ domain: "", username: "", password: "", note: "" });
  const [revealed, setRevealed] = React.useState<{ id: number; password: string } | null>(null);
  const [busy, setBusy] = React.useState(false);
  const load = React.useCallback(async () => {
    try { setAccounts(await api.taskCredentials(taskId)); }
    catch (error) { toast.error((error as Error).message); }
  }, [taskId]);
  React.useEffect(() => { void load(); }, [load]);
  React.useEffect(() => {
    if (!revealed) return;
    const timer = window.setTimeout(() => setRevealed(null), 20000);
    return () => window.clearTimeout(timer);
  }, [revealed]);

  async function add(event: React.FormEvent) {
    event.preventDefault();
    setBusy(true);
    try {
      await api.addTaskCredential(taskId, form);
      setForm({ domain: "", username: "", password: "", note: "" });
      setRevealed(null);
      await load();
      toast.success(t("taskintel.saved"));
    } catch (error) { toast.error((error as Error).message); }
    finally { setBusy(false); }
  }
  async function remove(id: number) {
    setBusy(true);
    try {
      await api.deleteTaskCredential(taskId, id);
      setRevealed(null);
      await load();
      toast.success(t("taskintel.removed"));
    } catch (error) { toast.error((error as Error).message); }
    finally { setBusy(false); }
  }
  async function reveal(id: number) {
    if (revealed?.id === id) { setRevealed(null); return; }
    try { setRevealed({ id, password: (await api.revealTaskCredential(taskId, id)).password }); }
    catch (error) { toast.error((error as Error).message); }
  }

  return <div className="grid max-w-4xl gap-4">
    <Card><CardHeader><CardTitle>{t("taskintel.accounts")}</CardTitle><p className="text-sm text-muted-foreground">{t("taskintel.accountsHelp")}</p></CardHeader>
      <CardContent><form onSubmit={(e) => void add(e)} className="grid gap-3 sm:grid-cols-2">
        <div className="grid gap-1"><Label htmlFor="account-domain">{t("taskintel.domain")}</Label><Input id="account-domain" required maxLength={255} value={form.domain} onChange={(e) => setForm({ ...form, domain: e.target.value })} /></div>
        <div className="grid gap-1"><Label htmlFor="account-user">{t("taskintel.username")}</Label><Input id="account-user" required maxLength={320} value={form.username} onChange={(e) => setForm({ ...form, username: e.target.value })} /></div>
        <div className="grid gap-1"><Label htmlFor="account-password">{t("taskintel.password")}</Label><Input id="account-password" type="password" autoComplete="off" maxLength={4096} value={form.password} onChange={(e) => setForm({ ...form, password: e.target.value })} /></div>
        <div className="grid gap-1"><Label htmlFor="account-note">{t("taskintel.note")}</Label><Input id="account-note" maxLength={1000} value={form.note} onChange={(e) => setForm({ ...form, note: e.target.value })} /></div>
        <div className="sm:col-span-2"><Button type="submit" disabled={busy}>{t("taskintel.add")}</Button></div>
      </form></CardContent></Card>
    <Card><CardHeader><CardTitle>{t("taskintel.accounts")} ({accounts.length})</CardTitle></CardHeader>
      <CardContent className="grid gap-2">{accounts.length === 0 && <p className="text-sm text-muted-foreground">{t("taskintel.emptyAccounts")}</p>}
        {accounts.map((account) => <div key={account.id} className="flex flex-wrap items-center justify-between gap-3 rounded-md border p-3">
          <div className="min-w-0"><div className="flex items-center gap-2"><strong className="text-sm">{account.username}</strong><Badge variant="outline">{t(account.source === "agent" ? "taskintel.agent" : "taskintel.user")}</Badge></div>
            <p className="break-all text-xs text-muted-foreground">{account.domain}{account.note ? ` · ${account.note}` : ""}</p>
            {revealed?.id === account.id && <code className="break-all text-sm">{revealed.password}</code>}
            {!account.has_password && <p className="text-xs text-muted-foreground">{t("taskintel.pendingPassword")}</p>}
          </div>
          <div className="flex gap-2"><Button variant="outline" size="sm" disabled={!account.has_password || busy} onClick={() => void reveal(account.id)}>{t(revealed?.id === account.id ? "taskintel.hide" : "taskintel.reveal")}</Button>
            <Button variant="ghost" size="sm" disabled={busy} onClick={() => void remove(account.id)}>{t("taskintel.remove")}</Button></div>
        </div>)}
      </CardContent></Card>
  </div>;
}
