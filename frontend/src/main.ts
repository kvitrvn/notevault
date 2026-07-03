import { mount } from 'svelte';
import App from './App.svelte';
import './styles.css';

const storedTheme = window.localStorage.getItem('notevault-theme');
document.documentElement.dataset.theme = storedTheme === 'light' ? 'light' : 'dark';

const target = document.getElementById('app');

if (!target) {
  throw new Error('Élément #app introuvable');
}

mount(App, { target });
