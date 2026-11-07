"use client";

import * as React from "react";
import { Background, Controls, MiniMap, ReactFlow, useEdgesState, useNodesState, type Edge, type Node } from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { toast } from "sonner";
import { useI18n } from "@/i18n";
import { api } from "@/lib/api";
import type { Agent, Workflow } from "@/lib/types";
import { blankWorkflow, workflowFromTemplate, workflowTemplates, WORKFLOW_TEMPLATE_INPUT_LIMIT, type WorkflowDraft, type WorkflowTemplate } from "@/lib/workflow-templates";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";

const scheduleKey = { manual: "workflows.manual", interval: "workflows.interval", daily: "workflows.daily" } as const;
const defaultNodes = (): Node[] => [
  { id: "trigger", type: "default", position: { x: 40, y: 90 }, data: { label: "Trigger" } },
  { id: "agent", type: "default", position: { x: 270, y: 90 }, data: { label: "Agent" } },
  { id: "action", type: "default", position: { x: 500, y: 90 }, data: { label: "Instructions" } },
];
const defaultEdges = (): Edge[] => [
  { id: "trigger-agent", source: "trigger", target: "agent" },
  { id: "agent-action", source: "agent", target: "action" },
];
function graphOf(w: Workflow | null): { nodes: Node[]; edges: Edge[] } {
  if (!w) return { nodes: defaultNodes(), edges: defaultEdges() };
  try {
    const graph = JSON.parse(w.graph_json) as { nodes?: Node[]; edges?: Edge[] };
    if (Array.isArray(graph.nodes) && graph.nodes.length > 0 && Array.isArray(graph.edges)) {
      // Older server-generated graphs used a custom type without a registered renderer.
      return { nodes: graph.nodes.map((node) => node.type === "workflow" ? { ...node, type: "default" } : node), edges: graph.edges };
    }
  } catch { /* use the default visual graph */ }
  return { nodes: defaultNodes(), edges: defaultEdges() };
}

export default function WorkflowsPage() {
  const { t } = useI18n();
  const [items, setItems] = React.useState<Workflow[]>([]);
  const [agents, setAgents] = React.useState<Agent[]>([]);
  const [selected, setSelected] = React.useState<Workflow | null>(null);
  const [draft, setDraft] = React.useState<WorkflowDraft>(blankWorkflow);
  const [nodes, setNodes, onNodesChange] = useNodesState<Node>(defaultNodes());
  const [edges, setEdges] = useEdgesState<Edge>(defaultEdges());
  const [busy, setBusy] = React.useState(false);
  const [loading, setLoading] = React.useState(true);
  const [loadError, setLoadError] = React.useState(false);
  const [showLibrary, setShowLibrary] = React.useState(true);
  const [template, setTemplate] = React.useState<WorkflowTemplate>(workflowTemplates[0]);
  const [scope, setScope] = React.useState("");
  const [context, setContext] = React.useState("");
  const editor = React.useRef<HTMLDivElement>(null);

  const load = React.useCallback(async () => {
    setLoading(true);
    setLoadError(false);
    try {
      const [workflows, availableAgents] = await Promise.all([api.workflows(), api.agents()]);
      setItems(workflows);
      setAgents(availableAgents);
    } catch (error) { setLoadError(true); toast.error((error as Error).message); }
    finally { setLoading(false); }
  }, []);
  React.useEffect(() => { void load(); }, [load]);

  function choose(w: Workflow | null) {
    setSelected(w);
    setDraft(w ? { name: w.name, description: w.description, agent_key: w.agent_key, enabled: w.enabled,
      schedule_kind: w.schedule_kind, interval_sec: w.interval_sec, daily_hour: w.daily_hour,
      daily_minute: w.daily_minute, timezone: w.timezone, prompt: w.prompt } : blankWorkflow());
    const graph = graphOf(w);
    setNodes(graph.nodes);
    setEdges(graph.edges);
    if (w) setShowLibrary(false);
  }
  function useTemplate() {
    if (!scope.trim() || !agents.some((a) => a.key === template.agentKey && a.enabled)) return;
    setSelected(null);
    setDraft(workflowFromTemplate(template, scope, context, t));
    setNodes(defaultNodes());
    setEdges(defaultEdges());
    setShowLibrary(false);
    // Move keyboard focus as well as the viewport to the newly created draft.
    requestAnimationFrame(() => {
      editor.current?.scrollIntoView({ block: "start" });
      editor.current?.querySelector<HTMLInputElement>("#wf-name")?.focus({ preventScroll: true });
    });
  }
  async function save() {
    if (!draft.name.trim() || !draft.prompt.trim()) return;
    setBusy(true);
    try {
      const body = { ...draft, name: draft.name.trim(), prompt: draft.prompt.trim(),
        graph_json: JSON.stringify({ nodes, edges, name: draft.name.trim() }) };
      const saved = selected ? await api.updateWorkflow(selected.id, body) : await api.createWorkflow(body);
      choose(saved);
      toast.success(t("workflows.saved"));
      await load();
    } catch (error) { toast.error((error as Error).message); }
    finally { setBusy(false); }
  }
  async function run() {
    if (!selected || unsavedChanges) return;
    setBusy(true);
    try { await api.runWorkflow(selected.id); toast.success(t("workflows.started")); await load(); }
    catch (error) { toast.error((error as Error).message); }
    finally { setBusy(false); }
  }
  async function remove() {
    if (!selected || !window.confirm(`${t("workflows.delete")}: ${selected.name}?`)) return;
    setBusy(true);
    try { await api.deleteWorkflow(selected.id); choose(null); toast.success(t("workflows.removed")); await load(); }
    catch (error) { toast.error((error as Error).message); }
    finally { setBusy(false); }
  }
  const set = <K extends keyof WorkflowDraft>(key: K, value: WorkflowDraft[K]) => setDraft((current) => ({ ...current, [key]: value }));
  const templateAvailable = agents.some((a) => a.key === template.agentKey && a.enabled);
  const editable = !loading && !loadError;
  const unsavedChanges = selected !== null && Object.entries(draft).some(([key, value]) => value !== selected[key as keyof WorkflowDraft]);

  return <div className="flex flex-col gap-4">
    <div className="flex items-start justify-between gap-3"><div>
      <h1 className="text-xl font-semibold">{t("workflows.title")}</h1>
      <p className="text-sm text-muted-foreground">{t("workflows.description")}</p>
    </div><div className="flex flex-wrap justify-end gap-2">
      <Button variant="outline" aria-expanded={showLibrary} aria-controls="workflow-library" disabled={busy} onClick={() => setShowLibrary((open) => !open)}>{t("workflows.library")}</Button>
      <Button disabled={busy} onClick={() => choose(null)}>{t("workflows.create")}</Button>
    </div></div>
    {loadError && <div role="alert" className="flex flex-wrap items-center gap-3 rounded-md border border-destructive p-3 text-sm">
      <span>{t("workflows.loadFailed")}</span><Button variant="outline" onClick={() => void load()}>{t("workflows.retry")}</Button>
    </div>}
    {showLibrary && <Card id="workflow-library"><CardHeader>
      <CardTitle>{t("workflows.library")} <Badge variant="outline">{workflowTemplates.length}</Badge></CardTitle>
      <p className="text-sm text-muted-foreground">{t("workflows.libraryHelp")}</p>
    </CardHeader><CardContent className="grid gap-4">
      <div className="grid gap-2 sm:grid-cols-2 xl:grid-cols-3">
        {workflowTemplates.map((item) => <button key={item.id} type="button" aria-pressed={template.id === item.id}
          disabled={busy} onClick={() => setTemplate(item)} data-workflow-template={item.id}
          className={`flex flex-col gap-1 rounded-md border p-3 text-left text-sm hover:bg-muted ${template.id === item.id ? "border-primary bg-muted" : ""}`}>
          <span className="font-medium">{t(item.nameKey)}</span><span className="text-xs text-muted-foreground">{t(item.descriptionKey)}</span>
        </button>)}
      </div>
      <div className="grid gap-3 md:grid-cols-2">
        <div className="grid gap-1"><Label htmlFor="wf-scope">{t("workflows.scope")}</Label><Textarea id="wf-scope" maxLength={WORKFLOW_TEMPLATE_INPUT_LIMIT} rows={3} placeholder={t("workflows.scopeHint")} value={scope} onChange={(e) => setScope(e.target.value)} /></div>
        <div className="grid gap-1"><Label htmlFor="wf-context">{t("workflows.context")}</Label><Textarea id="wf-context" maxLength={WORKFLOW_TEMPLATE_INPUT_LIMIT} rows={3} placeholder={t("workflows.contextHint")} value={context} onChange={(e) => setContext(e.target.value)} /></div>
      </div>
      <p className="text-xs text-muted-foreground">{t("workflows.manualNote")} {t("workflows.defaultModel")}</p>
      {editable && !templateAvailable && <p role="status" className="text-sm text-destructive">{t("workflows.templateUnavailable")}</p>}
      <Button className="w-fit" disabled={busy || !editable || !scope.trim() || !templateAvailable} onClick={useTemplate}>{t("workflows.useTemplate")}</Button>
    </CardContent></Card>}
    <div className="grid gap-4 xl:grid-cols-[260px_minmax(0,1fr)]">
      <Card><CardHeader><CardTitle>{t("workflows.savedList")}</CardTitle></CardHeader><CardContent className="grid gap-2">
        {loading && <p role="status" className="text-sm text-muted-foreground">{t("workflows.loading")}</p>}
        {!loading && !loadError && items.length === 0 && <p className="text-sm text-muted-foreground">{t("workflows.empty")}</p>}
        {items.map((w) => <button type="button" key={w.id} onClick={() => choose(w)}
          disabled={busy || !editable}
          className={`flex w-full flex-col gap-1 rounded-md border p-2 text-left text-sm hover:bg-muted ${selected?.id === w.id ? "border-primary bg-muted" : ""}`}>
          <span className="font-medium">{w.name}</span>
          <span className="flex items-center gap-2 text-xs text-muted-foreground"><Badge variant="outline">{t(scheduleKey[w.schedule_kind])}</Badge>{w.agent_key}</span>
        </button>)}
      </CardContent></Card>
      <div className="grid gap-4">
        <Card ref={editor}><CardHeader><CardTitle>{selected?.name || t("workflows.create")}</CardTitle></CardHeader>
          <CardContent className="grid gap-4">
            <div className="grid gap-3 md:grid-cols-2">
              <div className="grid gap-1"><Label htmlFor="wf-name">{t("workflows.name")}</Label><Input id="wf-name" maxLength={160} value={draft.name} onChange={(e) => set("name", e.target.value)} /></div>
              <div className="grid gap-1"><Label htmlFor="wf-agent">{t("workflows.agent")}</Label><select id="wf-agent" className="h-9 rounded-md border bg-background px-2 text-sm" value={draft.agent_key} onChange={(e) => set("agent_key", e.target.value)}>
                {agents.map((a) => <option key={a.key} value={a.key} disabled={!a.enabled}>{a.name || a.key}</option>)}
              </select></div>
            </div>
            <div className="grid gap-1"><Label htmlFor="wf-description">{t("workflows.details")}</Label><Input id="wf-description" maxLength={4000} value={draft.description} onChange={(e) => set("description", e.target.value)} /></div>
            <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
              <div className="grid gap-1"><Label htmlFor="wf-schedule">{t("workflows.schedule")}</Label><select id="wf-schedule" className="h-9 rounded-md border bg-background px-2 text-sm" value={draft.schedule_kind} onChange={(e) => set("schedule_kind", e.target.value as WorkflowDraft["schedule_kind"])}>
                {(["manual", "interval", "daily"] as const).map((kind) => <option key={kind} value={kind}>{t(scheduleKey[kind])}</option>)}
              </select></div>
              {draft.schedule_kind === "interval" && <div className="grid gap-1"><Label htmlFor="wf-interval">{t("workflows.minutes")}</Label><Input id="wf-interval" type="number" min={1} max={10080} value={Math.max(1, draft.interval_sec / 60)} onChange={(e) => set("interval_sec", Math.max(60, Number(e.target.value) * 60))} /></div>}
              {draft.schedule_kind === "daily" && <><div className="grid gap-1"><Label htmlFor="wf-hour">{t("workflows.hour")}</Label><Input id="wf-hour" type="number" min={0} max={23} value={draft.daily_hour} onChange={(e) => set("daily_hour", Number(e.target.value))} /></div>
                <div className="grid gap-1"><Label htmlFor="wf-minute">{t("workflows.minute")}</Label><Input id="wf-minute" type="number" min={0} max={59} value={draft.daily_minute} onChange={(e) => set("daily_minute", Number(e.target.value))} /></div></>}
              {draft.schedule_kind === "daily" && <div className="grid gap-1"><Label htmlFor="wf-timezone">{t("workflows.timezone")}</Label><Input id="wf-timezone" value={draft.timezone} onChange={(e) => set("timezone", e.target.value)} /></div>}
            </div>
            <div className="grid gap-1"><Label htmlFor="wf-prompt">{t("workflows.prompt")}</Label><Textarea id="wf-prompt" maxLength={16000} rows={5} value={draft.prompt} onChange={(e) => set("prompt", e.target.value)} /></div>
            <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={draft.enabled} onChange={(e) => set("enabled", e.target.checked)} />{t("workflows.enabled")}</label>
            {selected?.last_fire && <p className="text-xs text-muted-foreground">{t("workflows.lastRun")}: {new Date(selected.last_fire).toLocaleString()}</p>}
            <div className="flex flex-wrap gap-2"><Button disabled={busy || !editable || !draft.name.trim() || !draft.prompt.trim() || !agents.some((a) => a.key === draft.agent_key && a.enabled)} onClick={() => void save()}>{t("workflows.save")}</Button>
              {selected && <><Button variant="outline" disabled={busy || !editable || unsavedChanges} onClick={() => void run()}>{t("workflows.run")}</Button>
                <Button variant="destructive" disabled={busy} onClick={() => void remove()}>{t("workflows.delete")}</Button></>}
            </div>
            {unsavedChanges && <p role="status" className="text-xs text-muted-foreground">{t("workflows.saveBeforeRun")}</p>}
          </CardContent></Card>
        <Card><CardHeader><CardTitle>{t("workflows.canvas")}</CardTitle></CardHeader><CardContent>
          <div className="h-72 overflow-hidden rounded-md border"><ReactFlow nodes={nodes} edges={edges} onNodesChange={onNodesChange} fitView nodesConnectable={false}>
            <Background /><Controls /><MiniMap />
          </ReactFlow></div>
        </CardContent></Card>
      </div>
    </div>
  </div>;
}
