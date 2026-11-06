import assert from "node:assert/strict";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import { brotliDecompressSync, gunzipSync } from "node:zlib";
import { precompress } from "../scripts/precompress-static.mjs";

test("static variants preserve exact UI bytes without compressing fonts or tiny files", async () => {
  const directory = await mkdtemp(path.join(os.tmpdir(), "aptex static "));
  try {
    const text = Buffer.from("const label = 'APTEX by Afterglow Labs';\n".repeat(100));
    await writeFile(path.join(directory,"app.js"),text);
    await writeFile(path.join(directory,"font.woff2"),text);
    await writeFile(path.join(directory,"tiny.svg"),"<svg/>");
    assert.equal(await precompress(directory),2);
    assert.deepEqual(gunzipSync(await readFile(path.join(directory,"app.js.gz"))),text);
    assert.deepEqual(brotliDecompressSync(await readFile(path.join(directory,"app.js.br"))),text);
    await assert.rejects(readFile(path.join(directory,"font.woff2.gz")),{code:"ENOENT"});
    await assert.rejects(readFile(path.join(directory,"tiny.svg.gz")),{code:"ENOENT"});
  } finally { await rm(directory,{recursive:true,force:true}); }
});
