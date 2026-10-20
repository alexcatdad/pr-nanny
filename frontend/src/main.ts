import { mount } from 'svelte';
import App from './App.svelte';
import './styles.css';

if (import.meta.env.VITE_TEST_FIXTURES === 'true') {
  const { installMockBridge } = await import('./lib/mockBridge');
  installMockBridge();
}
mount(App, { target: document.getElementById('app')! });
