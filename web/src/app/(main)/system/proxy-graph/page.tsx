"use client";

import * as React from "react";
import { toast } from "sonner";
import { useI18n } from "@/i18n";
import { http } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

type Policy = "round_robin" | "asn_diverse" | "least_used";
type ProxyNode = { id: string; url: string; type: string; asn: string; active: boolean; score: number };
type Config = { policy: Policy; enabled: boolean };
const emptyForm = { id: "", url: "", type: "datacenter", asn: "" };
const policyKeys = { round_robin: "proxygraph.roundRobin", asn_diverse: "proxygraph.asnDiverse", least_used: "proxygraph.leastUsed" } as const;

export default function ProxyGraphPage() {
  const { t } = useI18n();
  const [nodes, setNodes] = React.useState<ProxyNode[]>([]);
  const [config, setConfig] = React.useState<Config>({ policy: "round_robin", enabled: false });
  const [form, setForm] = React.useState(emptyForm);
  const [busy, setBusy] = React.useState(false);

  const load = React.useCallback(async () => {
    try {
      const [list, cfg] = await Promise.all([
        http<{ nodes: ProxyNode[] }>("/proxygraph/nodes"),
        http<Config>("/proxygraph/config"),
      ]);
      setNodes(list.nodes);
      setConfig(cfg);
    } catch (error) { toast.error((error as Error).message); }
  }, []);
  React.useEffect(() => { void load(); }, [load]);

  async function saveConfig(next: Config) {
    setBusy(true);
    try {
      const saved = await http<Config>("/proxygraph/config", { method: "PUT", body: JSON.stringify(next) });
      setConfig(saved);
      toast.success(t("proxygraph.saved"));
    } catch (error) { toast.error((error as Error).message); }
    finally { setBusy(false); }
  }
  async function add(event: React.FormEvent) {
    event.preventDefault();
    setBusy(true);
    try {
      await http("/proxygraph/nodes", { method: "POST", body: JSON.stringify(form) });
      setForm(emptyForm);
      await load();
      toast.success(t("proxygraph.added"));
    } catch (error) { toast.error((error as Error).message); }
    finally { setBusy(false); }
  }
  async function remove(id: string) {
    setBusy(true);
    try {
      await http(`/proxygraph/nodes/${encodeURIComponent(id)}`, { method: "DELETE" });
      await load();
      toast.success(t("proxygraph.removed"));
    } catch (error) { toast.error((error as Error).message); }
    finally { setBusy(false); }
  }
  async function check() {
    setBusy(true);
    try {
      const result = await http<{ nodes: ProxyNode[] }>("/proxygraph/health", { method: "POST" });
      setNodes(result.nodes);
      toast.success(t("proxygraph.checked"));
    } catch (error) { toast.error((error as Error).message); }
    finally { setBusy(false); }
  }

  return <div className="mx-auto grid max-w-4xl gap-5">
    <div><h1 className="text-xl font-semibold">{t("proxygraph.title")}</h1>
      <p className="text-sm text-muted-foreground">{t("proxygraph.description")}</p></div>
    <Card><CardHeader><CardTitle>{t("proxygraph.routing")}</CardTitle></CardHeader><CardContent className="grid gap-3">
      <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={config.enabled} disabled={busy}
        onChange={(e) => void saveConfig({ ...config, enabled: e.target.checked })} />{t("proxygraph.enabled")}</label>
      <div className="grid gap-1"><Label htmlFor="proxy-policy">{t("proxygraph.policy")}</Label>
        <select id="proxy-policy" className="h-9 max-w-xs rounded-md border bg-background px-2 text-sm" value={config.policy} disabled={busy}
          onChange={(e) => void saveConfig({ ...config, policy: e.target.value as Policy })}>
          {(Object.keys(policyKeys) as Policy[]).map((key) => <option key={key} value={key}>{t(policyKeys[key])}</option>)}
        </select></div>
      <p className="text-xs text-muted-foreground">{t("proxygraph.captureNote")}</p>
    </CardContent></Card>
    <Card><CardHeader><CardTitle>{t("proxygraph.add")}</CardTitle></CardHeader><CardContent>
      <form onSubmit={(e) => void add(e)} className="grid gap-3 sm:grid-cols-2">
        <div className="grid gap-1"><Label htmlFor="proxy-id">{t("proxygraph.id")}</Label><Input id="proxy-id" required maxLength={64} value={form.id} onChange={(e) => setForm({ ...form, id: e.target.value })} /></div>
        <div className="grid gap-1"><Label htmlFor="proxy-url">{t("proxygraph.url")}</Label><Input id="proxy-url" required value={form.url} placeholder="http://proxy.example:8080" onChange={(e) => setForm({ ...form, url: e.target.value })} /></div>
        <div className="grid gap-1"><Label htmlFor="proxy-type">{t("proxygraph.type")}</Label><select id="proxy-type" className="h-9 rounded-md border bg-background px-2 text-sm" value={form.type} onChange={(e) => setForm({ ...form, type: e.target.value })}>
          <option value="datacenter">{t("proxygraph.datacenter")}</option><option value="residential">{t("proxygraph.residential")}</option><option value="tor">Tor</option>
        </select></div>
        <div className="grid gap-1"><Label htmlFor="proxy-asn">ASN</Label><Input id="proxy-asn" maxLength={64} value={form.asn} onChange={(e) => setForm({ ...form, asn: e.target.value })} /></div>
        <div className="sm:col-span-2"><Button type="submit" disabled={busy}>{t("proxygraph.add")}</Button></div>
      </form>
    </CardContent></Card>
    <Card><CardHeader className="flex flex-row items-center justify-between"><CardTitle>{t("proxygraph.nodes")} ({nodes.length})</CardTitle>
      <Button variant="outline" disabled={busy || nodes.length === 0} onClick={() => void check()}>{t("proxygraph.check")}</Button></CardHeader>
      <CardContent className="grid gap-2">{nodes.length === 0 && <p className="text-sm text-muted-foreground">{t("proxygraph.empty")}</p>}
        {nodes.map((node) => <div key={node.id} className="flex items-center justify-between gap-3 rounded-md border p-3">
          <div className="min-w-0"><div className="flex items-center gap-2"><strong className="text-sm">{node.id}</strong><Badge variant={node.active ? "default" : "secondary"}>{t(node.active ? "proxygraph.active" : "proxygraph.inactive")}</Badge></div>
            <p className="truncate text-xs text-muted-foreground">{node.url} · {node.type}{node.asn ? ` · ${node.asn}` : ""}</p></div>
          <Button variant="ghost" size="sm" disabled={busy} onClick={() => void remove(node.id)}>{t("proxygraph.remove")}</Button>
        </div>)}
      </CardContent></Card>
  </div>;
}
