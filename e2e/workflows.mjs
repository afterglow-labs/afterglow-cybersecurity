import { LOCALES, namespaceMessages } from "./locale-resources.mjs";

const messages = namespaceMessages("workflows");

// Exercise real workflow CRUD against the disposable smoke database. No agent
// is run: selecting, saving, editing, or deleting must not contact a model.
export async function runWorkflowChecks({ base, token, step, check, openPage, out }) {
  for (const locale of LOCALES) {
    await step(`[${locale}] starter workflows create scoped, unscheduled drafts and persist edits`, async () => {
      const page = await openPage(locale, token);
      const t = messages[locale];
      const headers = { Authorization: `Bearer ${token}` };
      const createdIDs = [];
      const writes = [];
      page.on("request", (request) => {
        if (request.method() !== "GET" && new URL(request.url()).pathname.startsWith("/api/workflows")) writes.push(request);
      });
      try {
        await page.goto(`${base}/function/workflows`);
        await page.getByRole("heading", { name: t.title, exact: true }).waitFor();
        await page.getByText(t.loading, { exact: true }).waitFor({ state: "hidden" });
        check(await page.locator("[data-workflow-template]").count() === 12, `${locale}: missing starter workflows`);
        check(!await page.getByText(t.loadFailed, { exact: true }).isVisible(), `${locale}: workflow load failed`);
        check(await page.getByRole("button", { name: t.useTemplate, exact: true }).isDisabled(), "template accepted an empty scope");
        const ids = await page.locator("[data-workflow-template]").evaluateAll((nodes) => nodes.map((node) => node.dataset.workflowTemplate));
        // All twelve templates exercise their real agent references in English;
        // every other locale also creates, reloads, edits, and deletes a draft.
        for (const id of locale === "en" ? ids : ids.slice(0, 1)) {
          if (!await page.locator("#workflow-library").isVisible()) await page.getByRole("button", { name: t.library, exact: true }).click();
          await page.locator(`[data-workflow-template="${id}"]`).click();
          await page.locator("#wf-scope").fill(`Task 42; fixture-${id}.localhost; saved evidence only`);
          await page.locator("#wf-context").fill("No new network requests. Use existing synthetic evidence.");
          const before = writes.length;
          await page.getByRole("button", { name: t.useTemplate, exact: true }).click();
          check(writes.length === before, "selecting a template wrote to the server");
          check(await page.locator("#wf-schedule").inputValue() === "manual", "template scheduled itself");
          check(!await page.getByRole("checkbox").isChecked(), "template enabled scheduling");
          check((await page.locator("#wf-prompt").inputValue()).includes(`fixture-${id}.localhost`), "scope was lost");
          check(await page.getByRole("button", { name: t.run, exact: true }).count() === 0, "unsaved draft can run");
          await page.locator("#wf-name").fill(`smoke-workflow-${locale}-${id}`);
          const create = page.waitForResponse((response) => new URL(response.url()).pathname === "/api/workflows" && response.request().method() === "POST");
          await page.getByRole("button", { name: t.save, exact: true }).click();
          const response = await create;
          check(response.status() === 201, `${id}: create returned ${response.status()}`);
          const workflow = await response.json();
          createdIDs.push(workflow.id);
          check(workflow.enabled === false && workflow.schedule_kind === "manual" && !workflow.last_fire, "saving a template activated it");
          await page.reload();
          await page.getByRole("button").filter({ has: page.getByText(workflow.name, { exact: true }) }).click();
          check(await page.locator("#wf-prompt").inputValue() === workflow.prompt, "reload changed saved instructions");
          const edited = `${workflow.prompt}\n\nOperator note: preserve this edit.`;
          await page.locator("#wf-prompt").fill(edited);
          check(await page.getByRole("button", { name: t.run, exact: true }).isDisabled(), "unsaved instructions could run the old saved workflow");
          const update = page.waitForResponse((r) => new URL(r.url()).pathname === `/api/workflows/${workflow.id}` && r.request().method() === "PATCH");
          await page.getByRole("button", { name: t.save, exact: true }).click();
          check((await update).ok(), "editing the workflow failed");
          const stored = await page.request.get(`${base}/api/workflows/${workflow.id}`, { headers });
          check((await stored.json()).prompt === edited, "saved edit was lost");
          check(writes.every((r) => !new URL(r.url()).pathname.endsWith("/run")), "template CRUD ran an agent");
          if (id === ids[0]) {
            await page.getByRole("button", { name: t.library, exact: true }).click();
            await page.screenshot({ path: `${out}/workflows-${locale}.png`, fullPage: false });
          }
          const deleted = await page.request.delete(`${base}/api/workflows/${workflow.id}`, { headers });
          check(deleted.ok(), "fixture cleanup failed");
          createdIDs.pop();
          await page.reload();
          await page.locator("[data-workflow-template]").first().waitFor();
        }
      } finally {
        for (const id of createdIDs) await page.request.delete(`${base}/api/workflows/${id}`, { headers });
        await page.context().close();
      }
    });
  }
}
