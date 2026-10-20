import { defineConfig } from 'astro/config';

export default defineConfig({
  site: 'https://alexcatdad.github.io',
  base: '/pr-nanny',
  output: 'static',
  build: { format: 'directory' },
});
