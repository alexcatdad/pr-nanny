import { readFile, readdir, stat } from 'node:fs/promises';
import { join, relative } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = fileURLToPath(new URL('../dist/', import.meta.url));
const pages = [];
async function collect(directory) {
  for (const entry of await readdir(directory, { withFileTypes: true })) {
    const path = join(directory, entry.name);
    if (entry.isDirectory()) await collect(path);
    else if (entry.name.endsWith('.html')) pages.push(path);
  }
}

await collect(root);
if (pages.length === 0) throw new Error('No built HTML pages found. Run `pnpm build` first.');
const issues = [];
for (const page of pages) {
  const html = await readFile(page, 'utf8');
  const ids = new Set([...html.matchAll(/\bid="([^"]+)"/g)].map((match) => match[1]));
  const anchors = [...html.matchAll(/\b(?:href|src)="([^"]+)"/g)].map((match) => match[1]);
  for (const href of anchors) {
    if (/^(?:https?:|mailto:|tel:|data:)/i.test(href)) continue;
    const url = new URL(href, `https://local.test/pr-nanny/${relative(root, page)}`);
    if (url.origin !== 'https://local.test') continue;
    if (url.hash && (url.pathname === '/pr-nanny/' || url.pathname === '/pr-nanny/index.html') && !ids.has(decodeURIComponent(url.hash.slice(1)))) {
      issues.push(`${relative(root, page)} -> missing anchor ${url.hash}`);
      continue;
    }
    if (url.pathname !== '/pr-nanny' && !url.pathname.startsWith('/pr-nanny/')) {
      issues.push(`${relative(root, page)} -> ${href} (outside the Pages base path)`);
      continue;
    }
    const localPath = decodeURIComponent(url.pathname.replace(/^\/pr-nanny\/?/, ''));
    const candidate = join(root, localPath);
    try {
      const details = await stat(candidate);
      const htmlPath = details.isDirectory() ? join(candidate, 'index.html') : candidate;
      if (details.isDirectory() && !(await stat(htmlPath)).isFile()) throw new Error('missing index.html');
      if (url.hash && htmlPath.endsWith('.html')) {
        const linkedHtml = await readFile(htmlPath, 'utf8');
        const linkedIds = new Set([...linkedHtml.matchAll(/\bid="([^"]+)"/g)].map((match) => match[1]));
        if (!linkedIds.has(decodeURIComponent(url.hash.slice(1)))) issues.push(`${relative(root, page)} -> ${href} (missing anchor)`);
      }
    } catch {
      issues.push(`${relative(root, page)} -> ${href}`);
    }
  }
}
if (issues.length) throw new Error(`Broken local links or assets:\n${issues.join('\n')}`);
console.log(`Checked ${pages.length} built page(s); all local links and assets resolve.`);
