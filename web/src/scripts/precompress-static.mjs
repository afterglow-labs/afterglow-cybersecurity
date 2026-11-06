import { readdir, readFile, stat, writeFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { brotliCompressSync, constants, gzipSync } from "node:zlib";

export async function precompress(directory) {
  if (!(await stat(directory)).isDirectory()) throw new Error(`Static export directory is missing: ${directory}`);
  let count = 0;
  for (const entry of await readdir(directory, { withFileTypes: true })) {
    const filename = path.join(directory, entry.name);
    if (entry.isDirectory()) { count += await precompress(filename); continue; }
    if (!entry.isFile() || !/\.(?:js|css|html|json|txt|svg)$/.test(entry.name)) continue;
    const source = await readFile(filename);
    if (source.length < 1024) continue;
    for (const [extension, data] of [["gz", gzipSync(source, { level: 9 })], ["br", brotliCompressSync(source, { params: { [constants.BROTLI_PARAM_QUALITY]: 9 } })]]) {
      if (data.length < source.length) { await writeFile(`${filename}.${extension}`, data); count++; }
    }
  }
  return count;
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const directory = fileURLToPath(new URL("../../out/", import.meta.url));
  console.log(`Precompressed ${await precompress(directory)} static asset variants.`);
}
