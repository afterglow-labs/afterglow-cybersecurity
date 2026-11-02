"use client";

import * as React from "react";
import Link from "next/link";
import { toast } from "sonner";
import { useI18n } from "@/i18n";
import { api } from "@/lib/api";
import type { AttackChain, AutoContinueWave, BusinessMapEntry, CampaignBranch, C2Handoff, ChainComposeState, HuntClass, TaskArtifact, TaskLesson, TaskOutcome } from "@/lib/types";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";

export function LearningTab({ taskId }: { taskId: string }) {
  const { t } = useI18n();
  const [outcome, setOutcome] = React.useState<TaskOutcome | null>(null);
  const [lessons, setLessons] = React.useState<TaskLesson[]>([]);
  const [waves, setWaves] = React.useState<AutoContinueWave[]>([]);
  const [artifacts, setArtifacts] = React.useState<TaskArtifact[]>([]);
  const [handoffs, setHandoffs] = React.useState<C2Handoff[]>([]);
  const [businessMap, setBusinessMap] = React.useState<BusinessMapEntry[]>([]);
  const [campaign, setCampaign] = React.useState<CampaignBranch[]>([]);
  const [huntClasses, setHuntClasses] = React.useState<HuntClass[]>([]);
  const [huntReasons, setHuntReasons] = React.useState<Record<string, string>>({});
  const [huntTags, setHuntTags] = React.useState<Record<string, HuntClass["miss_tag"]>>({});
  const [changingHunt, setChangingHunt] = React.useState("");
  const [chains, setChains] = React.useState<AttackChain[]>([]);
  const [composeState, setComposeState] = React.useState<ChainComposeState>({ running: false, added: 0 });
  const [changingHandoff, setChangingHandoff] = React.useState("");
  const [verifyingChain, setVerifyingChain] = React.useState("");
  const [mode, setMode] = React.useState("off");
  const [busy, setBusy] = React.useState(false);
  const load = React.useCallback(async () => {
    try {
      const [result, library, followups, indexed, pendingHandoffs, model, branches, coverage, attackChains, composing] = await Promise.all([api.taskOutcome(taskId), api.taskLessons(taskId), api.taskWaves(taskId), api.taskArtifacts(taskId), api.taskC2Handoffs(taskId), api.taskBusinessMap(taskId), api.taskCampaign(taskId), api.taskHuntClasses(taskId), api.taskAttackChains(taskId), api.chainComposeStatus(taskId)]);
      setOutcome(result); setLessons(library.lessons); setMode(library.mode); setWaves(followups); setArtifacts(indexed); setHandoffs(pendingHandoffs); setBusinessMap(model); setCampaign(branches); setHuntClasses(coverage); setChains(attackChains); setComposeState(composing);
    } catch (error) { toast.error((error as Error).message); }
  }, [taskId]);
  React.useEffect(() => { void load(); }, [load]);
  React.useEffect(() => {
    if (!composeState.running) return;
    const timer = window.setInterval(() => { void api.chainComposeStatus(taskId).then((state) => { setComposeState(state); if (!state.running) void load(); }).catch(() => {}); }, 4000);
    return () => window.clearInterval(timer);
  }, [composeState.running, load, taskId]);
  async function composeChains() {
    try { setComposeState(await api.composeTaskChains(taskId)); toast.success(t("taskintel.composeStarted")); }
    catch (error) { toast.error((error as Error).message); }
  }
  async function verifyChain(id: string) {
    setVerifyingChain(id);
    try { await api.verifyAttackChain(taskId, id); await load(); toast.success(t("taskintel.verificationStarted")); }
    catch (error) { toast.error((error as Error).message); }
    finally { setVerifyingChain(""); }
  }
  async function enableHunt() {
    setChangingHunt("enable");
    try { setHuntClasses(await api.enableTaskHuntClasses(taskId)); }
    catch (error) { toast.error((error as Error).message); }
    finally { setChangingHunt(""); }
  }
  async function updateHunt(huntClass: string, status: "open" | "exhausted" | "miss_tag") {
    const reason = huntReasons[huntClass]?.trim() ?? "";
    if (status !== "open" && !reason) { toast.error(t("taskintel.huntReasonRequired")); return; }
    setChangingHunt(huntClass);
    try { setHuntClasses(await api.updateTaskHuntClass(taskId, huntClass, { status, reason, miss_tag: status === "miss_tag" ? (huntTags[huntClass] ?? "coverage-gap") : undefined })); }
    catch (error) { toast.error((error as Error).message); }
    finally { setChangingHunt(""); }
  }
  async function extract() {
    setBusy(true);
    try { await api.extractTaskLessons(taskId); await load(); toast.success(t("taskintel.extracted")); }
    catch (error) { toast.error((error as Error).message); }
    finally { setBusy(false); }
  }
  async function setHandoff(id: string, status: "acknowledged" | "cancelled") {
    setChangingHandoff(id);
    try { await api.setC2HandoffStatus(taskId, id, status); await load(); }
    catch (error) { toast.error((error as Error).message); }
    finally { setChangingHandoff(""); }
  }
  const metrics = outcome ? [
    ["taskintel.confirmed", outcome.findings.confirmed], ["taskintel.open", outcome.findings.open],
    ["taskintel.toolCalls", outcome.tool_calls], ["taskintel.skillMisses", outcome.skill_misses],
    ["taskintel.wallTime", outcome.wall_seconds], ["taskintel.tokens", outcome.tokens.output_tokens],
  ] as const : [];
  return <div className="grid max-w-4xl gap-4">
    <Card><CardHeader><CardTitle>{t("taskintel.outcome")}</CardTitle></CardHeader><CardContent className="grid grid-cols-2 gap-3 sm:grid-cols-3">
      {metrics.map(([key, value]) => <div key={key} className="rounded-md border p-3"><strong className="text-xl tabular-nums">{value}</strong><p className="text-xs text-muted-foreground">{t(key)}</p></div>)}
    </CardContent></Card>
    <Card><CardHeader className="flex flex-row items-center justify-between"><div><CardTitle>{t("taskintel.lessons")} ({lessons.length})</CardTitle><p className="text-xs text-muted-foreground">{mode}</p></div>
      <Button variant="outline" disabled={busy} onClick={() => void extract()}>{t("taskintel.extract")}</Button></CardHeader>
      <CardContent className="grid gap-2">{lessons.length === 0 && <p className="text-sm text-muted-foreground">{t("taskintel.emptyLessons")}</p>}
        {lessons.map((lesson) => <div key={lesson.id} className="rounded-md border p-3 text-sm"><div className="flex items-center gap-2"><Badge variant="outline">{lesson.kind}</Badge><strong>{lesson.cue}</strong></div>
          <p className="mt-1">{lesson.action}</p><p className="mt-1 text-xs text-muted-foreground">{t("taskintel.evidence")}: {lesson.support}</p>
        </div>)}
      </CardContent></Card>
    <Card><CardHeader><CardTitle>{t("autowaves.waves")}</CardTitle></CardHeader><CardContent className="grid gap-2">
      {waves.length === 0 && <p className="text-sm text-muted-foreground">{t("autowaves.empty")}</p>}
      {waves.map((wave) => <div key={wave.id} className="rounded-md border p-3 text-sm">
        <div className="flex items-center gap-2"><strong>{t("autowaves.wave", { number: wave.wave_number })}</strong><Badge variant="outline">{t(({ pending: "autowaves.pending", spawning: "autowaves.spawning", spawned: "autowaves.spawned", cancelled: "autowaves.cancelled" } as const)[wave.status])}</Badge></div>
        <p className="mt-1 text-muted-foreground">{t(({ timeout: "autowaves.reasonTimeout", "open-goals": "autowaves.reasonGoals", "open-intents": "autowaves.reasonIntents" } as const)[wave.reason])}</p>
        {wave.spawned_task_id && <Link className="mt-2 inline-block underline" href={`/function/tasks/detail?id=${encodeURIComponent(wave.spawned_task_id)}`}>{t("autowaves.child", { id: wave.spawned_task_id })}</Link>}
      </div>)}
    </CardContent></Card>
    <Card><CardHeader><CardTitle>{t("taskintel.artifacts")}</CardTitle></CardHeader><CardContent className="grid gap-2">
      {artifacts.length === 0 && <p className="text-sm text-muted-foreground">{t("taskintel.emptyArtifacts")}</p>}
      {artifacts.map((artifact) => <div key={artifact.id} className="rounded-md border p-3 text-sm">
        <div className="flex items-center gap-2"><Badge variant="outline">{artifact.type}</Badge><span className="min-w-0 flex-1 truncate" title={artifact.path}>{artifact.path}</span><span className="tabular-nums">{Math.ceil(artifact.size_bytes / 1024)} KiB</span></div>
        <code className="mt-1 block break-all text-xs text-muted-foreground">SHA-256: {artifact.sha256}</code>
      </div>)}
    </CardContent></Card>
    <Card><CardHeader><CardTitle>{t("taskintel.businessModel")}</CardTitle></CardHeader><CardContent className="grid gap-2">
      {businessMap.length === 0 && <p className="text-sm text-muted-foreground">{t("taskintel.emptyBusiness")}</p>}
      {businessMap.map((entry) => <div key={entry.id} className="rounded-md border p-3 text-sm">
        <div className="flex items-center gap-2"><Badge variant="outline">{entry.kind}</Badge><strong>{entry.name}</strong></div>
        <pre className="mt-2 overflow-x-auto whitespace-pre-wrap break-words text-xs text-muted-foreground">{JSON.stringify(entry.detail, null, 2)}</pre>
      </div>)}
    </CardContent></Card>
    <Card><CardHeader><CardTitle>{t("taskintel.campaign")}</CardTitle></CardHeader><CardContent className="grid gap-2">
      {campaign.length === 0 && <p className="text-sm text-muted-foreground">{t("taskintel.emptyCampaign")}</p>}
      {campaign.map((branch) => <div key={branch.id} className="rounded-md border p-3 text-sm">
        <div className="flex items-center gap-2"><Badge variant="outline">{branch.branch_id}</Badge><strong>{branch.hypothesis}</strong><Badge>{t(({ untested: "taskintel.branchUntested", confirmed: "taskintel.branchConfirmed", clean: "taskintel.branchClean" } as const)[branch.status])}</Badge></div>
        {branch.probe_spec && <p className="mt-2 text-xs text-muted-foreground">{t("taskintel.branchProbe")}: {branch.probe_spec}</p>}
        {branch.expected_signal && <p className="mt-1 text-xs text-muted-foreground">{t("taskintel.branchSignal")}: {branch.expected_signal}</p>}
        {branch.reason && <p className="mt-1 text-xs text-muted-foreground">{branch.reason}</p>}
      </div>)}
    </CardContent></Card>
    <Card><CardHeader className="flex flex-row items-center justify-between gap-3"><div><CardTitle>{t("taskintel.huntBoard")}</CardTitle><p className="text-sm text-muted-foreground">{t("taskintel.huntHelp")}</p></div>
      {huntClasses.length === 0 && <Button variant="outline" disabled={changingHunt === "enable"} onClick={() => void enableHunt()}>{t("taskintel.huntEnable")}</Button>}
    </CardHeader><CardContent className="grid gap-2">
      {huntClasses.length === 0 && <p className="text-sm text-muted-foreground">{t("taskintel.huntEmpty")}</p>}
      {huntClasses.map((entry) => <div key={entry.id} className="rounded-md border p-3 text-sm">
        <div className="flex items-center gap-2"><strong className="flex-1">{entry.class}</strong><Badge variant="outline">{t(({ open: "taskintel.huntOpen", found: "taskintel.huntFound", exhausted: "taskintel.huntExhausted", miss_tag: "taskintel.huntGap" } as const)[entry.status])}</Badge></div>
        {entry.evidence && <p className="mt-1 text-xs text-muted-foreground">{entry.evidence}</p>}
        {entry.reason && <p className="mt-1 text-xs text-muted-foreground">{entry.reason}</p>}
        {entry.status !== "found" && <div className="mt-3 grid gap-2">
          <input className="h-9 rounded-md border bg-background px-3" aria-label={`${entry.class}: ${t("taskintel.huntReason")}`} placeholder={t("taskintel.huntReason")} maxLength={4000} value={huntReasons[entry.class] ?? ""} onChange={(event) => setHuntReasons((prev) => ({ ...prev, [entry.class]: event.target.value }))} />
          <div className="flex flex-wrap items-center gap-2"><select className="h-9 rounded-md border bg-background px-2" aria-label={`${entry.class}: ${t("taskintel.huntGapType")}`} value={huntTags[entry.class] ?? "coverage-gap"} onChange={(event) => setHuntTags((prev) => ({ ...prev, [entry.class]: event.target.value as HuntClass["miss_tag"] }))}>
            {(["coverage-gap", "probe-defect", "target-down", "scope-out"] as const).map((tag) => <option key={tag} value={tag}>{tag}</option>)}
          </select><Button size="sm" variant="outline" disabled={changingHunt === entry.class} onClick={() => void updateHunt(entry.class, "miss_tag")}>{t("taskintel.huntMarkGap")}</Button>
          <Button size="sm" variant="outline" disabled={changingHunt === entry.class} onClick={() => void updateHunt(entry.class, "exhausted")}>{t("taskintel.huntMarkExhausted")}</Button>
          {entry.status !== "open" && <Button size="sm" variant="ghost" disabled={changingHunt === entry.class} onClick={() => void updateHunt(entry.class, "open")}>{t("taskintel.huntReopen")}</Button>}</div>
        </div>}
      </div>)}
    </CardContent></Card>
    <Card><CardHeader className="flex flex-row items-center justify-between gap-3"><CardTitle>{t("taskintel.attackChains")}</CardTitle>
      <Button variant="outline" disabled={composeState.running || (outcome?.findings.confirmed ?? 0) < 2} onClick={() => void composeChains()}>{composeState.running ? t("taskintel.composeRunning") : t("taskintel.compose")}</Button>
    </CardHeader><CardContent className="grid gap-2">
      {composeState.error && <p role="alert" className="text-sm text-destructive">{t("taskintel.composeFailed")}</p>}
      {chains.length === 0 && <p className="text-sm text-muted-foreground">{t("taskintel.emptyChains")}</p>}
      {chains.map((chain) => <div key={chain.id} className="rounded-md border p-3 text-sm">
        <div className="flex flex-wrap items-center gap-2"><Badge>{chain.impact}</Badge><strong>{chain.name}</strong><Badge variant="outline">{t(({ proposed: "taskintel.chainProposed", verified: "taskintel.chainVerified", rejected: "taskintel.chainRejected" } as const)[chain.status])}</Badge></div>
        <p className="mt-1 text-xs text-muted-foreground">{t("taskintel.chainSource")}: {chain.source_finding_ids.join(", ")}</p>
        <ol className="mt-2 list-inside list-decimal space-y-1">{chain.steps.map((step, index) => <li key={`${chain.id}-${index}`}>{step.step}: {step.description}</li>)}</ol>
        {chain.reason && <p className="mt-2 text-xs text-muted-foreground">{chain.reason}</p>}
        {chain.spawned_task_id ? <Link className="mt-2 inline-block underline" href={`/function/tasks/detail?id=${encodeURIComponent(chain.spawned_task_id)}`}>{t("taskintel.verificationTask", { id: chain.spawned_task_id })}</Link>
          : chain.status === "proposed" && <Button className="mt-2" size="sm" variant="outline" disabled={verifyingChain === chain.id} onClick={() => void verifyChain(chain.id)}>{t("taskintel.verifyChain")}</Button>}
      </div>)}
    </CardContent></Card>
    <Card><CardHeader><CardTitle>{t("taskintel.c2Title")}</CardTitle><p className="text-sm text-muted-foreground">{t("taskintel.c2Help")}</p></CardHeader><CardContent className="grid gap-2">
      {handoffs.length === 0 && <p className="text-sm text-muted-foreground">{t("taskintel.c2Empty")}</p>}
      {handoffs.map((handoff) => <div key={handoff.id} className="rounded-md border p-3 text-sm">
        <div className="flex flex-wrap items-center gap-2"><Badge variant="outline">{handoff.vector}</Badge><strong>{handoff.target_host}</strong><span>{handoff.target_path}</span><Badge>{t(({ requested: "taskintel.c2Requested", acknowledged: "taskintel.c2Acknowledged", cancelled: "taskintel.c2Cancelled" } as const)[handoff.status])}</Badge></div>
        <code className="mt-1 block break-all text-xs text-muted-foreground">{handoff.binary_arch} · SHA-256: {handoff.binary_sha256}</code>
        {handoff.shell_url && <code className="mt-1 block break-all text-xs text-muted-foreground">{handoff.shell_url}</code>}
        {handoff.status === "requested" && <div className="mt-3 flex gap-2"><Button size="sm" disabled={changingHandoff === handoff.id} onClick={() => void setHandoff(handoff.id, "acknowledged")}>{t("taskintel.c2Acknowledge")}</Button><Button size="sm" variant="outline" disabled={changingHandoff === handoff.id} onClick={() => void setHandoff(handoff.id, "cancelled")}>{t("taskintel.c2Cancel")}</Button></div>}
      </div>)}
    </CardContent></Card>
  </div>;
}
