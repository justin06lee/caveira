// The cell editor's dev server: serves the app and reads/writes designs in
// tui/designs. Each save writes three files per design:
//
//   <name>.cells.json  the editable source
//   <name>.ans         ANSI, so `cat` shows it in a real terminal
//   <name>.txt         plain text, for reading and diffing
import { mkdir, readdir, readFile, rm, stat, writeFile } from "node:fs/promises";
import { join, resolve } from "node:path";
import index from "./src/index.html";
import { toAnsi, toPlainText } from "./src/lib/ansi";
import { compose } from "./src/lib/compose";
import { sanitizeDoc } from "./src/lib/model";

const DIR = resolve(import.meta.dir, process.env.CELLS_DIR ?? "../tui/designs");
const PORT = Number(process.env.PORT ?? 4747);
const SOURCE = ".cells.json";

await mkdir(DIR, { recursive: true });

const validName = (n: string) => /^[a-z0-9][a-z0-9_-]{0,63}$/.test(n);
const file = (name: string, ext: string) => join(DIR, name + ext);

async function list() {
  const names = (await readdir(DIR)).filter((f) => f.endsWith(SOURCE)).map((f) => f.slice(0, -SOURCE.length));
  const out = await Promise.all(
    names.map(async (name) => ({ name, updatedAt: (await stat(file(name, SOURCE))).mtimeMs })),
  );
  return out.sort((a, b) => b.updatedAt - a.updatedAt);
}

async function save(name: string, body: unknown) {
  const doc = sanitizeDoc(body, name);
  doc.name = name;
  const grid = compose(doc);
  await Promise.all([
    writeFile(file(name, SOURCE), JSON.stringify(doc, null, 2) + "\n"),
    writeFile(file(name, ".ans"), toAnsi(grid)),
    writeFile(file(name, ".txt"), toPlainText(grid)),
  ]);
}

const server = Bun.serve({
  port: PORT,
  development: process.env.NODE_ENV !== "production" && { hmr: true, console: true },
  routes: {
    "/": index,
    "/api/designs": {
      GET: async () => Response.json({ dir: DIR, designs: await list() }),
    },
    "/api/designs/:name": {
      GET: async (req) => {
        const { name } = req.params;
        if (!validName(name)) return new Response("bad name", { status: 400 });
        try {
          return Response.json(sanitizeDoc(JSON.parse(await readFile(file(name, SOURCE), "utf8")), name));
        } catch {
          return new Response("not found", { status: 404 });
        }
      },
      PUT: async (req) => {
        const { name } = req.params;
        if (!validName(name)) return new Response("bad name", { status: 400 });
        await save(name, await req.json());
        return Response.json({ ok: true });
      },
      DELETE: async (req) => {
        const { name } = req.params;
        if (!validName(name)) return new Response("bad name", { status: 400 });
        await Promise.all([SOURCE, ".ans", ".txt"].map((ext) => rm(file(name, ext), { force: true })));
        return Response.json({ ok: true });
      },
    },
  },
});

console.log(`cell editor  ${server.url}`);
console.log(`designs      ${DIR}`);
