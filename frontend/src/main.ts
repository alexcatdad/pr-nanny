import { mount } from 'svelte';
import App from './App.svelte';
import './styles.css';

if (import.meta.env.VITE_TEST_FIXTURES === 'true') {
  const { installMockBridge } = await import('./lib/mockBridge');
  installMockBridge();
}
const target = document.getElementById('app');
if (!target) throw new Error('App mount target is missing');
mount(App, { target });
