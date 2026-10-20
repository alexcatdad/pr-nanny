import { mount } from 'svelte';
import App from './App.svelte';
import './styles.css';

if (import.meta.env.VITE_TEST_FIXTURES === 'true') {
  const { installMockBridge } = await import('./lib/mockBridge');
  installMockBridge();
}
// macOS WebKit draws input[switch] as the system switch; other webviews get a CSS look-alike.
if (/Macintosh/.test(navigator.userAgent) && !/Chrome|Chromium|Edg/.test(navigator.userAgent))
  document.documentElement.dataset.nativeSwitch = '';
const target = document.getElementById('app');
if (!target) throw new Error('App mount target is missing');
mount(App, { target });
