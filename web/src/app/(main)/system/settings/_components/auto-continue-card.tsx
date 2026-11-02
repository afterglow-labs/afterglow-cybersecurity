"use client";

import * as React from "react";
import { toast } from "sonner";
import { useI18n } from "@/i18n";
import { api } from "@/lib/api";
import type { AutoContinuePolicy } from "@/lib/types";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";

export function AutoContinueCard() {
  const { t } = useI18n();
  const [policy, setPolicy] = React.useState<AutoContinuePolicy>({ enabled: false, delay_minutes: 10, wave_timeout_minutes: 60 });
  const [saving, setSaving] = React.useState(false);
  React.useEffect(() => {
    let alive = true;
    void api.autoContinuePolicy().then((value) => { if (alive) setPolicy(value); }).catch((error) => toast.error((error as Error).message));
    return () => { alive = false; };
  }, []);
  async function save() {
    setSaving(true);
    try {
      setPolicy(await api.saveAutoContinuePolicy(policy));
      toast.success(t("autowaves.saved"));
    } catch (error) {
      toast.error((error as Error).message);
    } finally {
      setSaving(false);
    }
  }
  return <Card className="mb-4 break-inside-avoid md:mb-6">
    <CardHeader><CardTitle className="text-base">{t("autowaves.title")}</CardTitle><CardDescription>{t("autowaves.description")}</CardDescription></CardHeader>
    <CardContent className="grid gap-4">
      <div className="flex items-center justify-between gap-3"><Label htmlFor="auto-waves-enabled">{t("autowaves.enabled")}</Label><Switch id="auto-waves-enabled" checked={policy.enabled} onCheckedChange={(enabled) => setPolicy((p) => ({ ...p, enabled }))} /></div>
      <div className="grid grid-cols-2 gap-3">
        <div className="grid gap-1"><Label htmlFor="auto-waves-delay">{t("autowaves.delay")}</Label><Input id="auto-waves-delay" type="number" min={1} max={1440} value={policy.delay_minutes} onChange={(event) => setPolicy((p) => ({ ...p, delay_minutes: Number(event.target.value) }))} /></div>
        <div className="grid gap-1"><Label htmlFor="auto-waves-budget">{t("autowaves.budget")}</Label><Input id="auto-waves-budget" type="number" min={5} max={480} value={policy.wave_timeout_minutes} onChange={(event) => setPolicy((p) => ({ ...p, wave_timeout_minutes: Number(event.target.value) }))} /></div>
      </div>
      <Button className="w-fit" disabled={saving} onClick={() => void save()}>{t("autowaves.save")}</Button>
    </CardContent>
  </Card>;
}
