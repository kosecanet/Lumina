(() => {
'use strict';

/* ================= utilidades ================= */
const $ = (s, r = document) => r.querySelector(s);
const view = $('#view'), nav = $('#nav');

function el(tag, a, ...kids) {
  const n = document.createElement(tag);
  if (a) for (const k in a) {
    const v = a[k];
    if (v == null || v === false) continue;
    if (k === 'class') n.className = v;
    else if (k === 'text') n.textContent = v;
    else if (k.startsWith('on')) n.addEventListener(k.slice(2), v);
    else n.setAttribute(k, v === true ? '' : v);
  }
  for (const c of kids.flat(Infinity)) if (c != null && c !== false) n.append(c.nodeType ? c : document.createTextNode(c));
  return n;
}

const ICONS = {
  play: 'M8 5v14l11-7z', pause: 'M6 5h4v14H6zm8 0h4v14h-4z',
  'chev-l': 'M15.41 7.41 14 6l-6 6 6 6 1.41-1.41L10.83 12z', 'chev-r': 'M8.59 16.59 13.17 12 8.59 7.41 10 6l6 6-6 6z',
  search: 'M15.5 14h-.79l-.28-.27A6.47 6.47 0 0 0 16 9.5 6.5 6.5 0 1 0 9.5 16c1.61 0 3.09-.59 4.23-1.57l.27.28v.79l5 4.99L20.49 19zm-6 0C7.01 14 5 11.99 5 9.5S7.01 5 9.5 5 14 7.01 14 9.5 11.99 14 9.5 14',
  cog: 'M19.14 12.94c.04-.3.06-.61.06-.94s-.02-.64-.07-.94l2.03-1.58a.49.49 0 0 0 .12-.61l-1.92-3.32a.49.49 0 0 0-.59-.22l-2.39.96c-.5-.38-1.03-.7-1.62-.94l-.36-2.54a.48.48 0 0 0-.48-.41h-3.84c-.24 0-.43.17-.47.41l-.36 2.54c-.59.24-1.13.57-1.62.94l-2.39-.96a.48.48 0 0 0-.59.22L2.74 8.87c-.12.21-.08.47.12.61l2.03 1.58c-.05.3-.09.63-.09.94s.02.64.07.94l-2.03 1.58a.49.49 0 0 0-.12.61l1.92 3.32c.12.22.37.29.59.22l2.39-.96c.5.38 1.03.7 1.62.94l.36 2.54c.05.24.24.41.48.41h3.84c.24 0 .44-.17.47-.41l.36-2.54c.59-.24 1.13-.56 1.62-.94l2.39.96c.22.08.47 0 .59-.22l1.92-3.32c.12-.22.07-.47-.12-.61zM12 15.6A3.6 3.6 0 1 1 12 8.4a3.6 3.6 0 0 1 0 7.2',
  close: 'M19 6.41 17.59 5 12 10.59 6.41 5 5 6.41 10.59 12 5 17.59 6.41 19 12 13.41 17.59 19 19 17.59 13.41 12z',
  back: 'M20 11H7.83l5.59-5.59L12 4l-8 8 8 8 1.41-1.41L7.83 13H20z',
  fs: 'M7 14H5v5h5v-2H7zm-2-4h2V7h3V5H5zm12 7h-3v2h5v-5h-2zM14 5v2h3v3h2V5z',
  vol: 'M3 9v6h4l5 5V4L7 9zm13.5 3A4.5 4.5 0 0 0 14 7.97v8.05c1.48-.73 2.5-2.25 2.5-4.02',
  mute: 'M16.5 12A4.5 4.5 0 0 0 14 7.97v2.21l2.45 2.45c.03-.2.05-.41.05-.63m2.5 0c0 .94-.2 1.82-.54 2.64l1.51 1.51A8.8 8.8 0 0 0 21 12c0-4.28-2.99-7.86-7-8.77v2.06c2.89.86 5 3.54 5 6.71M4.27 3 3 4.27 7.73 9H3v6h4l5 5v-6.73l4.25 4.25c-.67.52-1.42.93-2.25 1.18v2.06a8.99 8.99 0 0 0 3.69-1.81L19.73 21 21 19.73l-9-9zM12 4 9.91 6.09 12 8.18z',
  cc: 'M19 4H5a2 2 0 0 0-2 2v12a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2V6a2 2 0 0 0-2-2m-8 7H9.5v-.5h-2v3h2V13H11v1c0 .55-.45 1-1 1H7c-.55 0-1-.45-1-1v-4c0-.55.45-1 1-1h3c.55 0 1 .45 1 1zm7 0h-1.5v-.5h-2v3h2V13H18v1c0 .55-.45 1-1 1h-3c-.55 0-1-.45-1-1v-4c0-.55.45-1 1-1h3c.55 0 1 .45 1 1z',
  star: 'M22 9.24l-7.19-.62L12 2 9.19 8.63 2 9.24l5.46 4.73L5.82 21 12 17.27 18.18 21l-1.63-7.03zM12 15.4l-3.76 2.27 1-4.28-3.32-2.88 4.38-.38L12 6.1l1.71 4.04 4.38.38-3.32 2.88 1 4.28z',
  starfill: 'M12 17.27 18.18 21l-1.64-7.03L22 9.24l-7.19-.61L12 2 9.19 8.63 2 9.24l5.46 4.73L5.82 21z',
  up: 'M7.41 15.41 12 10.83l4.59 4.58L18 14l-6-6-6 6z', down: 'M7.41 8.59 12 13.17l4.59-4.58L18 10l-6 6-6-6z',
  plus: 'M19 13h-6v6h-2v-6H5v-2h6V5h2v6h6z',
  trash: 'M6 19c0 1.1.9 2 2 2h8c1.1 0 2-.9 2-2V7H6zM19 4h-3.5l-1-1h-5l-1 1H5v2h14z',
  refresh: 'M17.65 6.35A7.96 7.96 0 0 0 12 4a8 8 0 1 0 7.73 10h-2.08A5.99 5.99 0 0 1 12 18a6 6 0 1 1 0-12c1.66 0 3.14.69 4.22 1.78L13 11h7V4z',
  folder: 'M10 4H4c-1.1 0-1.99.9-1.99 2L2 18c0 1.1.9 2 2 2h16c1.1 0 2-.9 2-2V8c0-1.1-.9-2-2-2h-8z',
  info: 'M11 7h2v2h-2zm0 4h2v6h-2zm1-9C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2m0 18c-4.41 0-8-3.59-8-8s3.59-8 8-8 8 3.59 8 8-3.59 8-8 8',
  home: 'M10 20v-6h4v6h5v-8h3L12 3 2 12h3v8z', film: 'M18 4l2 4h-3l-2-4h-2l2 4h-3l-2-4H8l2 4H7L5 4H4c-1.1 0-1.99.9-1.99 2L2 18c0 1.1.9 2 2 2h16c1.1 0 2-.9 2-2V4z',
  series: 'M21 3H3c-1.1 0-2 .9-2 2v12c0 1.1.9 2 2 2h5v2h8v-2h5c1.1 0 1.99-.9 1.99-2L23 5c0-1.1-.9-2-2-2m0 14H3V5h18z',
  live: 'M21 6h-7.59l3.29-3.29L16 2l-4 4-4-4-.71.71L10.59 6H3c-1.1 0-2 .9-2 2v12c0 1.1.9 2 2 2h18c1.1 0 2-.9 2-2V8c0-1.1-.9-2-2-2m0 14H3V8h18zM9 10v8l7-4z',
  heart: 'M12 21.35l-1.45-1.32C5.4 15.36 2 12.28 2 8.5 2 5.42 4.42 3 7.5 3c1.74 0 3.41.81 4.5 2.09C13.09 3.81 14.76 3 16.5 3 19.58 3 22 5.42 22 8.5c0 3.78-3.4 6.86-8.55 11.54z',
  kids: 'M9 11.75a1.25 1.25 0 1 0 0 2.5 1.25 1.25 0 0 0 0-2.5zm6 0a1.25 1.25 0 1 0 0 2.5 1.25 1.25 0 0 0 0-2.5zM12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm0 18c-4.41 0-8-3.59-8-8 0-.29.02-.58.05-.86 2.36-1.05 4.23-2.98 5.21-5.37a9.974 9.974 0 0 0 10.41 3.97c.21.71.33 1.47.33 2.26 0 4.41-3.59 8-8 8z',
  music: 'M12 3v10.55c-.59-.34-1.27-.55-2-.55-2.21 0-4 1.79-4 4s1.79 4 4 4 4-1.79 4-4V7h4V3z',
  bolt: 'M11 21h-1l1-7H7.5c-.58 0-.57-.32-.38-.66.19-.34.05-.08.07-.12C8.48 10.94 10.42 7.54 13 3h1l-1 7h3.5c.49 0 .56.33.47.51l-.07.15C12.96 17.55 11 21 11 21z',
  globe: 'M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-1 17.93c-3.95-.49-7-3.85-7-7.93 0-.62.08-1.21.21-1.79L9 15v1c0 1.1.9 2 2 2zm6.9-2.54c-.26-.81-1-1.39-1.9-1.39h-1v-3c0-.55-.45-1-1-1H8v-2h2c.55 0 1-.45 1-1V7h2c1.1 0 2-.9 2-2v-.41c2.93 1.19 5 4.06 5 7.41 0 2.08-.8 3.97-2.1 5.39z',
  camera: 'M17 10.5V7c0-.55-.45-1-1-1H4c-.55 0-1 .45-1 1v10c0 .55.45 1 1 1h12c.55 0 1-.45 1-1v-3.5l4 4v-11z',
  cloud: 'M19.35 10.04C18.67 6.59 15.64 4 12 4 9.11 4 6.6 5.64 5.35 8.04 2.34 8.36 0 10.91 0 14c0 3.31 2.69 6 6 6h13c2.76 0 5-2.24 5-5 0-2.64-2.05-4.78-4.65-4.96z',
  next: 'M6 18l8.5-6L6 6zM16 6v12h2V6z', top: 'M5 3h14v2H5zm7 4-7 7h4v7h6v-7h4z',
};
const LIB_ICONS = ['film', 'series', 'kids', 'heart', 'star', 'music', 'bolt', 'globe', 'camera', 'cloud', 'folder', 'play', 'live', 'home'];
const libIcon = l => (l.icon && ICONS[l.icon]) ? l.icon : (l.type === 'cloud' ? 'cloud' : l.type === 'series' ? 'series' : 'film');
function icon(n, cls) {
  const NS = 'http://www.w3.org/2000/svg';
  const s = document.createElementNS(NS, 'svg'), p = document.createElementNS(NS, 'path');
  s.setAttribute('viewBox', '0 0 24 24'); s.setAttribute('aria-hidden', 'true');
  if (cls) s.setAttribute('class', cls);
  p.setAttribute('d', ICONS[n] || ''); s.append(p);
  return s;
}

const pad2 = n => String(n).padStart(2, '0');
function fmtT(s) {
  s = Math.max(0, Math.floor(s || 0));
  const h = Math.floor(s / 3600), m = Math.floor(s % 3600 / 60), x = s % 60;
  return h ? `${h}:${pad2(m)}:${pad2(x)}` : `${m}:${pad2(x)}`;
}
function fmtDur(s) {
  if (!s) return '';
  const m = Math.max(1, Math.round(s / 60));
  if (m < 60) return m + ' min';
  const h = Math.floor(m / 60), r = m % 60;
  return r ? `${h} h ${r} min` : `${h} h`;
}
const clock = ts => new Date(ts * 1000).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
function hue(s) { let h = 0; for (let i = 0; i < s.length; i++) h = (h * 31 + s.charCodeAt(i)) % 360; return h; }
function grad(s) { const h = hue(s || ''); return `--g:linear-gradient(135deg,hsl(${h} 42% 30%),hsl(${(h + 50) % 360} 48% 17%))`; }
function initials(s) {
  const w = (s || '?').replace(/[^\p{L}\p{N} ]/gu, ' ').trim().split(/\s+/);
  return ((w[0] || '?')[0] + (w.length > 1 ? w[1][0] : '')).toUpperCase();
}
const nn = (...a) => a.flat(Infinity).filter(x => x != null && x !== false);
const sleep = ms => new Promise(r => setTimeout(r, ms));
function debounce(f, ms) { let t; return (...a) => { clearTimeout(t); t = setTimeout(() => f(...a), ms); }; }

let toastT;
function toast(msg, bad) {
  const t = $('#toast');
  t.textContent = msg; t.className = 'show' + (bad ? ' bad' : '');
  clearTimeout(toastT); toastT = setTimeout(() => t.className = '', 3800);
}

function modal(title, body, actions) {
  const m = $('#modal');
  const close = () => { m.hidden = true; m.replaceChildren(); document.removeEventListener('keydown', esc, true); };
  const esc = e => { if (e.key === 'Escape') { e.stopPropagation(); close(); } };
  document.addEventListener('keydown', esc, true);
  const btns = (actions || [{ label: 'Cerrar' }]).map(a => el('button', {
    class: 'btn' + (a.pri ? ' pri' : '') + (a.bad ? ' bad' : ''), text: a.label,
    onclick: async () => { if (a.fn && (await a.fn()) === false) return; close(); },
  }));
  m.replaceChildren(el('div', { class: 'mb', onmousedown: e => { if (e.target === e.currentTarget) close(); } },
    el('div', { class: 'md', role: 'dialog', 'aria-label': title }, el('h3', { text: title }), el('div', { class: 'mc' }, body), el('div', { class: 'ma' }, btns))));
  m.hidden = false;
  (m.querySelector('.btn.pri') || m.querySelector('.btn'))?.focus();
  return close;
}

/* ================= API ================= */
async function api(path, o = {}) {
  o = { ...o, headers: { ...(o.headers || {}) } };
  if (o.body && typeof o.body !== 'string') { o.body = JSON.stringify(o.body); o.headers['Content-Type'] = 'application/json'; }
  let r;
  try { r = await fetch(path, o); } catch (e) { throw new Error('Sin conexión con el servidor'); }
  if (r.status === 401 && path !== '/api/login') { showLogin(); throw new Error('auth'); }
  const txt = await r.text();
  let d = null; try { d = txt ? JSON.parse(txt) : null; } catch { /* no es JSON */ }
  if (!r.ok) { const e = new Error((d && d.error) || `Error ${r.status}`); e.status = r.status; e.data = d; throw e; }
  return d;
}
const post = (p, body) => api(p, { method: 'POST', body: body ?? {} });

const S = { libs: [], scan: null, xt: null, live: false };
async function refreshMeta() {
  const m = await api('/api/me');
  S.accounts = m.accounts; S.me = m.me; S.admin = m.me.admin;
  if (S.admin) {
    const [l, x] = await Promise.all([api('/api/libraries'), api('/api/xtream/config')]);
    S.libs = l.libs; S.scan = l.scan; S.xt = x; S.live = !!(x.url && x.selected > 0);
  } else { S.libs = m.me.libs; S.scan = null; S.xt = null; S.live = m.me.live; }
  buildNav();
}

window.L = { el, icon, api, post, toast, modal, fmtT, fmtDur, clock, initials, grad, sleep, S, $, refresh: () => route(true) };

/* ================= tarjetas y filas ================= */
const cardHref = c => (c.t === 's' ? '#/series/' : '#/title/') + c.id;
const thumbUrl = (c, w) => `/api/thumb/${c.th}?` + (c.sa ? 's=1&' : '') + (w ? 'w=' + w : '');

function lazyImg(src, im, cls) {
  const img = el('img', { alt: '', loading: 'lazy', decoding: 'async', src });
  img.onload = () => img.classList.add('ld');
  img.onerror = () => { img.remove(); im.classList.add('noimg'); };
  return img;
}

function card(c) {
  const im = el('div', { class: 'im', style: grad(c.ti), 'data-i': initials(c.ti) });
  im.append(lazyImg(thumbUrl(c), im), el('div', { class: 'pl' }, icon('play')));
  if (c.p > 0) im.append(el('div', { class: 'pb' }, el('i', { style: `width:${Math.round(c.p * 100)}%` })));
  return el('a', { class: 'card', href: cardHref(c), title: c.ti }, im,
    el('div', { class: 'ct', text: c.ti }), c.su ? el('div', { class: 'cs', text: c.su }) : null);
}

function nnPct(now) { return now ? Math.min(100, Math.max(0, (Date.now() / 1000 - now.s) / Math.max(1, now.e - now.s) * 100)) : 0; }

function liveCard(c, list) {
  const im = el('div', { class: 'im', 'data-i': initials(c.n) });
  if (c.ic) im.append(lazyImg('/api/logo/' + c.id, im)); else im.classList.add('noimg');
  im.append(el('span', { class: 'badge', text: 'EN DIRECTO' }), el('div', { class: 'pl' }, icon('play')));
  if (c.now) im.append(el('div', { class: 'pb' }, el('i', { style: `width:${nnPct(c.now)}%` })));
  return el('a', { class: 'card live', href: '#/live', title: c.n, onclick: e => { e.preventDefault(); Player.live(c, list); } }, im,
    el('div', { class: 'ct', text: c.n }), el('div', { class: 'cs', text: c.now ? c.now.t : 'Sin guía' }));
}

function row(title, cards, link) {
  const track = el('div', { class: 'track' }, cards);
  const go = d => track.scrollBy({ left: d * track.clientWidth * .85, behavior: 'smooth' });
  return el('section', { class: 'row' },
    el('h2', null, link ? el('a', { href: link }, title, icon('chev-r')) : title),
    el('div', { class: 'rw' },
      el('button', { class: 'nv l', 'aria-label': 'Anterior', onclick: () => go(-1) }, icon('chev-l')), track,
      el('button', { class: 'nv r', 'aria-label': 'Siguiente', onclick: () => go(1) }, icon('chev-r'))));
}

/* ================= navegación ================= */
let navToken = 0, heroTimer = 0;
const searchIn = el('input', {
  type: 'search', placeholder: 'Buscar', 'aria-label': 'Buscar', autocomplete: 'off',
  oninput: debounce(e => {
    const q = e.target.value.trim();
    if (q.length < 2) return;
    const h = '#/search/' + encodeURIComponent(q);
    if (location.hash.startsWith('#/search')) location.replace(h); else location.hash = h;
  }, 280),
  onkeydown: e => { if (e.key === 'Escape') { e.target.value = ''; e.target.blur(); } },
});

/* ---------- avatares de perfil ---------- */
const AVC = [['#58b9e3', '#2b7196'], ['#f2a547', '#d17a1d'], ['#6fc77d', '#2f8b4b'], ['#b98ae3', '#7b4eb2'], ['#ef8383', '#c24949'], ['#52c9be', '#27887f']];
let avN = 0;
function avatar(p, size) {
  const c = AVC[(p && p.color) % 6 || 0], id = 'ag' + (++avN);
  const kid = p && p.kind === 'k';
  const face = kid
    ? `<defs><linearGradient id="${id}f" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#fff1c4"/><stop offset="1" stop-color="#f9c566"/></linearGradient></defs>
       <path d="M50 20c3 0 5 2 6.5 5l5 9.6 10.6 1.9c4 .7 5.5 5.5 2.7 8.4l-7.5 7.7 1.6 10.8c.6 4-3.5 7-7.1 5.2L50 63.7l-9.8 5.3c-3.600 1.800-7.700-1.200-7.100-5.200l1.600-10.800-7.500-7.700c-2.800-2.900-1.300-7.700 2.700-8.400l10.600-1.900 5-9.600C45 22 47 20 50 20z" fill="url(#${id}f)"/>
       <circle cx="43" cy="46" r="3" fill="#a85b14"/><circle cx="57" cy="46" r="3" fill="#a85b14"/><path d="M44 54c3 3.500 9 3.500 12 0" stroke="#a85b14" stroke-width="3" stroke-linecap="round" fill="none"/>`
    : `<ellipse cx="50" cy="38" rx="15" ry="18" fill="#c9efff" opacity=".92"/><path d="M14 100c0-22 16-33 36-33s36 11 36 33z" fill="#c9efff" opacity=".78"/>`;
  const n = el('span', { class: 'av' });
  if (size) n.style.cssText = `width:${size}px;height:${size}px`;
  n.innerHTML = `<svg viewBox="0 0 100 100" aria-hidden="true"><defs><linearGradient id="${id}" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="${c[0]}"/><stop offset="1" stop-color="${c[1]}"/></linearGradient></defs><rect width="100" height="100" fill="url(#${id})"/>${face}</svg>`;
  return n;
}

/* ---------- barra lateral ---------- */
function buildNav() {
  const p = S.profile || {};
  const item = (h, ic, t) => el('a', { class: 'ni', href: h, 'data-h': h, 'aria-label': t }, icon(ic), el('span', { class: 'nl', text: t }));
  nav.replaceChildren(...nn(
    el('button', { class: 'ni pf', title: S.accounts ? 'Mi cuenta' : 'Cambiar de perfil', 'aria-label': (S.accounts ? 'Mi cuenta (' : 'Cambiar de perfil (') + (p.name || '') + ')', onclick: () => (S.accounts ? accountMenu() : showProfiles()) }, avatar(p), el('span', { class: 'nl', text: p.name || 'Perfil' })),
    item('#/search/', 'search', 'Buscar'),
    item('#/', 'home', 'Inicio'),
    S.libs.map(l => item('#/lib/' + l.id, libIcon(l), l.name)),
    S.live ? item('#/live', 'live', 'TV en directo') : null,
    el('div', { class: 'sp' }),
    S.admin ? item('#/settings', 'cog', 'Ajustes') : null));
  markNav();
}
function markNav() {
  const h = (location.hash || '#/').split('?')[0];
  nav.querySelectorAll('.ni[data-h]').forEach(a => {
    const d = a.dataset.h;
    a.classList.toggle('on', d === '#/' ? (h === '#/' || h === '#') : h.startsWith(d.replace(/\/$/, '')));
  });
}

const routes = [
  [/^\/?$/, vHome], [/^\/lib\/(\w+)$/, vLib], [/^\/series\/(\w+)$/, vSeries], [/^\/title\/(\w+)$/, vTitle],
  [/^\/live$/, vLive], [/^\/search(?:\/(.*))?$/, vSearch], [/^\/settings(?:\/(\w+))?$/, vSettings],
];

let skTimer = 0;
function mount(tok, node) {
  if (tok !== navToken) return false;
  clearTimeout(skTimer); // la pantalla de carga no debe tapar la vista ya dibujada
  view.replaceChildren(node);
  return true;
}

function skeleton() {
  return el('div', { class: 'skel' }, el('div', { class: 'a sk' }), el('div', { class: 'b' }, [1, 2, 3, 4, 5].map(() => el('div', { class: 'sk' }))));
}

async function route(keep) {
  clearInterval(heroTimer); clearInterval(liveTimer);
  const hash = location.hash.slice(1) || '/';
  const [path, qs] = hash.split('?');
  const q = new URLSearchParams(qs || '');
  markNav();
  if (!path.startsWith('/search') && searchIn.value) searchIn.value = '';
  for (const [re, fn] of routes) {
    const m = path.match(re);
    if (!m) continue;
    const tok = ++navToken;
    clearTimeout(skTimer);
    const sk = skTimer = setTimeout(() => { if (tok === navToken) view.replaceChildren(skeleton()); }, 140);
    try { await fn(m.slice(1).map(decodeURIComponent), q, tok); }
    catch (e) { if (e.message !== 'auth' && tok === navToken) mount(tok, el('div', { class: 'empty' }, el('h1', { text: 'Algo ha fallado' }), el('p', { text: e.message }), el('a', { class: 'btn pri', href: '#/', text: 'Ir al inicio' }))); }
    clearTimeout(sk);
    if (tok === navToken && !keep) scrollTo(0, 0);
    return;
  }
  location.hash = '#/';
}
addEventListener('hashchange', () => { Player.closeSilent(); route(); });

/* ================= vistas ================= */
function onboarding() {
  return el('div', { class: 'empty' },
    el('h1', { text: 'Bienvenido a Lumina' }),
    el('p', { text: 'Añade una carpeta con tus vídeos y, si quieres, conecta tu servicio de TV en directo (Xtream).' }),
    el('div', { class: 'acts' },
      el('a', { class: 'btn pri', href: '#/settings/bib', text: 'Añadir biblioteca' }),
      el('a', { class: 'btn', href: '#/settings/tv', text: 'Configurar TV en directo' })));
}

function hero(items) {
  let cur = 0;
  const slides = items.map((c, i) => {
    const img = el('img', { class: 'bg', alt: '', decoding: 'async' });
    const meta = [c.y || '', c.t === 's' ? c.su : fmtDur(c.d)].filter(Boolean).map(t => el('span', { text: t }));
    const s = el('div', { class: 'slide', 'data-src': thumbUrl(c, 1280), 'aria-hidden': i ? 'true' : 'false' }, img, el('div', { class: 'shade' }),
      el('div', { class: 'hc' },
        el('span', { class: 'tag', text: c.t === 's' ? 'Serie' : 'Película' }),
        el('h1', { text: c.ti }),
        el('div', { class: 'meta' }, meta.flatMap((m, k) => k ? [' · ', m] : [m])),
        el('div', { class: 'acts' },
          el('a', { class: 'btn pri', href: cardHref(c) + '?play=1' }, icon('play'), 'Reproducir'),
          el('a', { class: 'btn', href: cardHref(c) }, icon('info'), 'Más información'))));
    img.onerror = () => { img.remove(); s.style.cssText = grad(c.ti) + ';background:var(--g)'; };
    return s;
  });
  const dots = items.map((_, i) => el('button', { 'aria-label': 'Ir a ' + (i + 1), onclick: () => { show(i); restart(); } }));
  const box = el('div', { class: 'hero' }, slides, items.length > 1 ? el('div', { class: 'dots' }, dots) : null);
  function show(i) {
    cur = i;
    slides.forEach((s, k) => {
      const on = k === i;
      if (on && !s.firstChild.src && s.dataset.src) s.firstChild.src = s.dataset.src;
      s.classList.toggle('on', on); s.setAttribute('aria-hidden', on ? 'false' : 'true');
    });
    dots.forEach((d, k) => d.classList.toggle('on', k === i));
  }
  function restart() { clearInterval(heroTimer); if (items.length > 1) heroTimer = setInterval(() => { if (!document.hidden && !box.matches(':hover')) show((cur + 1) % items.length); }, 9000); }
  show(0); restart();
  return box;
}

async function vHome(_, q, tok) {
  const d = await api('/api/home');
  const page = el('div');
  if (d.empty) return mount(tok, onboarding());
  const rows = el('div', { class: 'rows' });
  page.append(d.hero.length ? hero(d.hero) : el('div', { class: 'spacer' }), rows);
  let liveDone = false;
  const addLive = () => { if (!liveDone && d.live && d.live.length) { liveDone = true; rows.append(row('TV en directo', d.live.map(c => liveCard(c, d.live)), '#/live')); } };
  for (const r of d.rows) {
    if (r.key !== 'continue') addLive();
    rows.append(row(r.title, r.cards.map(card), r.link));
  }
  addLive();
  if (!rows.children.length) rows.append(el('div', { class: 'empty' }, el('h1', { text: 'Todavía no hay contenido' }), el('p', { text: 'Si acabas de añadir una biblioteca, el escaneo puede tardar unos segundos.' }), el('a', { class: 'btn', href: '#/settings/bib', text: 'Ver bibliotecas' })));
  mount(tok, page);
}

async function vLib([id], q, tok) {
  const d = await api('/api/library/' + id);
  let col = q.get('col') || '', sort = 'az', filter = '';
  let all = d.cards;
  const grid = el('div', { class: 'grid' });
  const sentinel = el('div', { class: 'more' });
  const cnt = el('span', { class: 'cnt' });
  let list = [], shown = 0;
  const BATCH = 60;
  const io = new IntersectionObserver(es => { if (es[0].isIntersecting) more(); }, { rootMargin: '900px' });
  function more() { const end = Math.min(list.length, shown + BATCH); for (; shown < end; shown++) grid.append(card(list[shown])); }
  const by = { az: (a, b) => a.ti.localeCompare(b.ti, 'es', { sensitivity: 'base' }), recent: (a, b) => b.a - a.a, year: (a, b) => (b.y || 0) - (a.y || 0) };
  function refresh() {
    const f = filter.toLowerCase();
    list = all.filter(c => (!col || c.co === col) && (!f || c.ti.toLowerCase().includes(f))).sort(by[sort]);
    grid.replaceChildren(); shown = 0; more();
    cnt.textContent = list.length + (list.length === 1 ? ' título' : ' títulos');
  }
  const chips = d.cols && d.cols.length ? el('div', { class: 'chips' }, [['', 'Todo'], ...d.cols.map(c => [c, c])].map(([v, t]) => el('button', {
    class: 'chip' + (v === col ? ' on' : ''), text: t,
    onclick: e => { col = v; chips.querySelectorAll('.chip').forEach(b => b.classList.toggle('on', b === e.currentTarget)); refresh(); },
  }))) : null;
  const head = el('div', { class: 'ph' }, el('h1', { text: d.lib.name }), cnt,
    el('input', { class: 'inp', type: 'search', placeholder: 'Filtrar…', 'aria-label': 'Filtrar', oninput: debounce(e => { filter = e.target.value.trim(); refresh(); }, 150) }),
    el('select', { class: 'inp', 'aria-label': 'Ordenar', onchange: e => { sort = e.target.value; refresh(); } },
      el('option', { value: 'az', text: 'A – Z' }), el('option', { value: 'recent', text: 'Añadidos recientemente' }), el('option', { value: 'year', text: 'Año' })));
  if (!all.length && !d.pending) return mount(tok, el('div', null, head, el('div', { class: 'empty' }, el('p', { text: 'Esta biblioteca está vacía (o aún se está escaneando).' }))));
  mount(tok, el('div', null, head, chips, grid, sentinel));
  refresh(); io.observe(sentinel);
  // biblioteca en la nube: las categorías que aún se están leyendo del proveedor van llegando poco a poco
  if (d.pending) {
    cnt.textContent += ' · cargando…';
    const again = async n => {
      await sleep(2000);
      if (tok !== navToken) return;
      let r; try { r = await api('/api/library/' + id); } catch { return; }
      if (tok !== navToken) return;
      all = r.cards; refresh();
      if (r.pending && n < 60) { cnt.textContent += ' · cargando…'; again(n + 1); }
    };
    again(0);
  }
}

function detailHeader(bg, h1, meta, sub, acts) {
  const img = el('img', { class: 'bg', alt: '', src: bg });
  const box = el('section', { class: 'det', style: '--g:linear-gradient(135deg,#223040,#0f171e);background:var(--g)' }, img, el('div', { class: 'shade' }),
    el('div', { class: 'in' }, el('h1', { text: h1 }), el('div', { class: 'meta' }, meta.filter(Boolean).map(m => el('span', { text: m }))), sub ? el('div', { class: 'sub', text: sub }) : null, el('div', { class: 'acts' }, acts)));
  img.onerror = () => img.remove();
  return box;
}

// Ficha en castellano (sinopsis, géneros, reparto) bajo la cabecera; se rellena cuando llega, sin retrasar la página.
function metaBlock(m, kind) {
  const chips = [m.rating > 0 ? '★ ' + m.rating.toFixed(1) : '', m.runtime ? m.runtime + ' min' : '', ...(m.genres || [])].filter(Boolean);
  return el('section', { class: 'minfo' },
    m.tagline ? el('p', { class: 'tg', text: m.tagline }) : null,
    m.overview ? el('p', { class: 'ov', text: m.overview }) : null,
    chips.length ? el('div', { class: 'mi-ch' }, chips.map(c => el('span', { text: c }))) : null,
    m.director ? el('p', { class: 'mi-cr' }, el('b', { text: kind === 'tv' ? 'Creación: ' : 'Dirección: ' }), m.director) : null,
    m.cast && m.cast.length ? el('p', { class: 'mi-cr' }, el('b', { text: 'Reparto: ' }), m.cast.join(', ')) : null,
    el('p', { class: 'src', text: 'Información: ' + (m.source || '') }));
}
function addMeta(page, id, tok) {
  api('/api/meta/' + id).then(r => {
    if (!r || !r.meta || tok !== navToken || !page.isConnected) return;
    const b = metaBlock(r.meta, r.kind), eps = page.querySelector('.eps');
    if (page.classList.contains('det')) page.after(b); else if (eps) page.insertBefore(b, eps); else page.append(b);
  }).catch(() => { /* es solo un extra */ });
}

async function vTitle([id], q, tok) {
  const d = await api('/api/item/' + id);
  const isEp = d.kind === 'episode';
  const finished = d.dur > 0 && d.pos >= d.dur * .95;
  const resume = d.pos > 10 && !finished;
  const acts = [
    el('button', { class: 'btn pri', onclick: () => Player.vod(id, { start: resume ? d.pos : 0 }) }, icon('play'), resume ? `Reanudar · ${fmtT(d.pos)}` : 'Reproducir'),
    resume ? el('button', { class: 'btn', onclick: () => Player.vod(id, { start: 0 }) }, 'Desde el principio') : null,
    isEp ? el('a', { class: 'btn', href: '#/series/' + d.sid }, 'Ver serie') : null,
  ];
  const meta = [d.year || '', fmtDur(d.dur), d.vc ? d.vc.toUpperCase() : ''];
  const page = detailHeader(`/api/thumb/${id}?w=1280`, isEp ? d.series : d.title, meta, isEp ? `${d.season ? 'Temporada ' + d.season + ' · ' : ''}Episodio ${d.ep || ''}: ${d.title}` : '', acts);
  if (!mount(tok, page)) return;
  addMeta(page, id, tok);
  if (q.get('play') === '1') { history.replaceState(null, '', '#/title/' + id); Player.vod(id, { start: resume ? d.pos : 0 }); }
}

async function vSeries([sid], q, tok) {
  const d = await api('/api/series/' + sid);
  const seasons = d.seasons;
  let active = seasons.findIndex(s => s.eps.some(e => e.id === d.resume));
  if (active < 0) active = 0;
  const flat = seasons.flatMap(s => s.eps);
  const list = el('div');
  const tabs = el('div', { class: 'seas' });
  const play = id => Player.vod(id, { start: -1 });
  function renderEps() {
    const s = seasons[active];
    list.replaceChildren(...s.eps.map((e, i) => {
      const im = el('div', { class: 'im', style: grad(e.ti), 'data-i': e.e || i + 1 });
      im.append(lazyImg(thumbUrl(e), im), el('div', { class: 'pl' }, icon('play')));
      if (e.p > 0) im.append(el('div', { class: 'pb' }, el('i', { style: `width:${Math.round(e.p * 100)}%` })));
      return el('button', { class: 'ep', onclick: () => play(e.id) }, el('span', { class: 'n', text: e.e || i + 1 }), im,
        el('div', null, el('div', { class: 't', text: e.ti }), el('div', { class: 'd', text: fmtDur(e.d) + (e.p >= .95 ? ' · Visto' : '') })));
    }));
    tabs.querySelectorAll('button').forEach((b, k) => b.classList.toggle('on', k === active));
  }
  if (seasons.length > 1) seasons.forEach((s, i) => tabs.append(el('button', { class: 'chip', text: s.n ? 'Temporada ' + s.n : 'Episodios', onclick: () => { active = i; renderEps(); } })));
  const resumeEp = flat.find(e => e.id === d.resume);
  const started = resumeEp && (resumeEp.p > 0 || d.resume !== flat[0].id);
  const page = el('div', null,
    detailHeader(`/api/thumb/${d.th}?s=1&w=1280`, d.name, [d.year || '', seasons.length > 1 ? seasons.length + ' temporadas' : '', d.count + (d.count === 1 ? ' episodio' : ' episodios')], '',
      [el('button', { class: 'btn pri', onclick: () => Player.vod(d.resume, { start: -1 }) }, icon('play'), started && resumeEp ? `Continuar · ${(resumeEp.se ? 'T' + resumeEp.se + ' ' : '')}E${resumeEp.e || ''}` : 'Reproducir')]),
    el('section', { class: 'eps' }, el('h2', { text: 'Episodios' }), tabs, list));
  renderEps();
  if (!mount(tok, page)) return;
  if (d.resume) addMeta(page, d.resume, tok);
  if (q.get('play') === '1') { history.replaceState(null, '', '#/series/' + sid); Player.vod(d.resume, { start: -1 }); }
}

/* ---------- TV en directo ---------- */
let liveTimer = 0;
async function toggleFav(c, star) {
  c.f = !c.f;
  star.classList.toggle('on', c.f); star.replaceChildren(icon(c.f ? 'starfill' : 'star'));
  try { await post('/api/xtream/fav', { id: c.id, on: c.f }); } catch (e) { toast(e.message, true); }
}

// Precalentar un canal: si no se está viendo nada, el servidor lo abre ya y al pulsarlo arranca casi al instante.
let warmT = 0;
function warmCancel() { clearTimeout(warmT); }
function warmLater(id, ms) {
  clearTimeout(warmT);
  warmT = setTimeout(() => { if (!document.hidden && !window.Player.isOpen()) api('/api/live/warm', { method: 'POST', body: { id } }).catch(() => { /* es solo una ayuda */ }); }, ms);
}

function chanRow(c, list, num) {
  const lg = el('div', { class: 'lg' });
  if (c.ic) { const im = el('img', { src: '/api/logo/' + c.id, alt: '', loading: 'lazy', decoding: 'async' }); im.onerror = () => { im.remove(); lg.textContent = initials(c.n); }; lg.append(im); } else lg.textContent = initials(c.n);
  const info = el('div', { class: 'inf' },
    c.now ? el('div', { class: 'now' }, el('b', { text: c.now.t || '—' }), el('span', { class: 'tm', text: clock(c.now.s) + ' – ' + clock(c.now.e) })) : el('div', { class: 'now mut', text: 'Sin información de programación' }),
    c.now ? el('div', { class: 'bar' }, el('i', { style: `width:${nnPct(c.now)}%` })) : null,
    c.next ? el('div', { class: 'nx', text: 'A continuación · ' + (c.next.t || '—') }) : null);
  const star = el('span', { class: 'star' + (c.f ? ' on' : ''), role: 'button', tabindex: '0', 'aria-label': 'Favorito', onclick: e => { e.stopPropagation(); toggleFav(c, star); } }, icon(c.f ? 'starfill' : 'star'));
  const go = () => Player.live(c, list);
  return el('div', { class: 'ch', role: 'button', tabindex: '0', onclick: go, onmouseenter: () => warmLater(c.id, 600), onmouseleave: () => warmCancel(), onfocus: () => warmLater(c.id, 900), onblur: () => warmCancel(), onkeydown: e => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); go(); } } },
    el('span', { class: 'num', text: num }), lg,
    el('div', { class: 'cn' }, el('b', null, c.n, c.v ? el('span', { class: 'use', text: 'EN USO' }) : null), el('span', { class: 'cc', text: c.cn || '' })), info, star);
}

async function vLive(_, q, tok) {
  let d = await api('/api/live/channels');
  if (!d.chans.length) return mount(tok, el('div', { class: 'empty' }, el('h1', { text: 'No hay canales seleccionados' }), el('p', { text: 'Elige en Ajustes qué canales de tu servicio quieres ver.' }), el('a', { class: 'btn pri', href: '#/settings/tv', text: 'Elegir canales' })));
  let cat = sessionStorage.getItem('lcat') || '';
  const list = el('div', { class: 'chl' });
  const chips = el('div', { class: 'chips' });
  const head = el('div', { class: 'ph' }, el('h1', { text: 'TV en directo' }), el('span', { class: 'cnt', text: `Hasta ${d.max} ${d.max === 1 ? 'canal distinto' : 'canales distintos'} a la vez` }));
  function vis() { return d.chans.filter(c => cat === '*' ? c.f : (!cat || c.c === cat)); }
  // con cientos o miles de canales no se pinta todo de golpe: se van añadiendo filas al hacer scroll
  const sentinel = el('div', { class: 'more' });
  let vcur = [], shown = 0;
  const io = new IntersectionObserver(es => { if (es[0].isIntersecting) more(); }, { rootMargin: '900px' });
  function more() { const end = Math.min(vcur.length, shown + 40); const fr = document.createDocumentFragment(); for (; shown < end; shown++) fr.append(chanRow(vcur[shown], vcur, shown + 1)); list.append(fr); }
  function render(keep) {
    const v = vis(); vcur = v;
    const want = keep ? Math.max(40, shown) : 40;
    list.replaceChildren(); shown = 0;
    while (shown < Math.min(v.length, want)) more();
    if (!v.length) list.append(el('div', { class: 'empty', style: 'padding:30px' }, el('p', { text: cat === '*' ? 'Aún no tienes favoritos: pulsa la estrella de un canal.' : 'No hay canales aquí.' })));
    chips.replaceChildren(...[['', 'Todos'], ['*', '★ Favoritos'], ...d.cats.map(c => [c.id, c.n])].map(([v, t]) =>
      el('button', { class: 'chip' + (v === cat ? ' on' : ''), text: t, onclick: () => { cat = v; sessionStorage.setItem('lcat', v); render(); } })));
  }
  render();
  mount(tok, el('div', null, head, chips, list, sentinel)); io.observe(sentinel);
  try { const last = Number(localStorage.getItem('lumina.lastlive')); if (last && d.chans.some(c => c.id === last)) warmLater(last, 400); } catch { /* nada */ }
  liveTimer = setInterval(async () => {
    if (document.hidden || tok !== navToken) return;
    const stale = d.chans.some(c => c.now && c.now.e < Date.now() / 1000);
    if (stale || (Date.now() % 120000) < 30000) { try { d = await api('/api/live/channels'); const y = scrollY; render(true); scrollTo(0, y); } catch { /* reintento luego */ } }
    else list.querySelectorAll('.bar i').forEach((b, i) => { const c = vis()[i]; if (c && c.now) b.style.width = nnPct(c.now) + '%'; });
  }, 30000);
}

async function vSearch([qs], q, tok) {
  const term = (qs || '').trim();
  if (searchIn.value !== term) searchIn.value = term;
  const had = document.activeElement === searchIn;
  const d = term.length < 2 ? { cards: [], chans: [] } : await api('/api/search?q=' + encodeURIComponent(term));
  const page = el('div', null, el('div', { class: 'ph' }, el('h1', { text: term.length < 2 ? 'Buscar' : `Resultados para «${term}»` })), el('label', { class: 'sbar' }, icon('search'), searchIn));
  if (d.cards.length) page.append(el('div', { class: 'grid' }, d.cards.map(card)));
  if (d.chans.length) page.append(el('div', { class: 'ph', style: 'padding-top:10px' }, el('h1', { style: 'font-size:1.4rem', text: 'Canales de TV' })), el('div', { class: 'chl' }, d.chans.map((c, i) => chanRow(c, d.chans, i + 1))));
  if (!d.cards.length && !d.chans.length) page.append(el('div', { class: 'empty', style: 'padding-top:50px' }, el('p', { text: term.length < 2 ? 'Escribe el título de una película, serie o canal.' : 'No se ha encontrado nada.' })));
  if (!mount(tok, page)) return;
  if (had || term.length < 2) { searchIn.focus(); try { searchIn.setSelectionRange(term.length, term.length); } catch { /* tipo search */ } }
}

/* ---------- ajustes ---------- */
async function vSettings([tab], q, tok) {
  tab = tab || 'bib';
  await refreshMeta();
  if (!S.admin) { location.hash = '#/'; return; }
  const body = el('div');
  const tabs = el('div', { class: 'tabs' }, [['bib', 'Bibliotecas'], ['tv', 'TV en directo'], ['usr', 'Usuarios'], ['sys', 'Sistema']].map(([k, t]) => el('a', { href: '#/settings/' + k, class: k === tab ? 'on' : '', text: t })));
  if (!mount(tok, el('div', { class: 'set' }, el('h1', { text: 'Ajustes' }), tabs, body))) return;
  if (tab === 'tv') await setTV(body, tok); else if (tab === 'usr') await setUsers(body, tok); else if (tab === 'sys') await setSys(body, tok); else await setLibs(body, tok);
}

function field(label, input) { return el('div', { class: 'fr' }, el('label', { text: label }), input); }

async function pickFolder(start) {
  return new Promise(resolve => {
    let cur = start || '';
    const pathEl = el('div', { class: 'path' }), ul = el('ul', { class: 'fd' });
    async function load(p) {
      try {
        const d = await api('/api/fs?path=' + encodeURIComponent(p));
        cur = d.path; pathEl.textContent = d.path || 'Unidades';
        ul.replaceChildren(...nn(
          d.path ? el('li', null, el('button', { onclick: () => load(d.parent) }, icon('up'), '.. (subir)')) : null,
          d.dirs.map(x => el('li', null, el('button', { onclick: () => load(x.p) }, icon('folder'), x.n))),
          d.dirs.length ? null : el('li', { class: 'mut', style: 'padding:10px 12px', text: 'No hay subcarpetas' })));
      } catch (e) { toast(e.message, true); }
    }
    let done = false;
    const close = modal('Elegir carpeta del servidor', el('div', null, pathEl, ul), [
      { label: 'Cancelar', fn: () => { if (!done) { done = true; resolve(null); } } },
      { label: 'Elegir esta carpeta', pri: true, fn: () => { if (!cur) { toast('Entra primero en una carpeta', true); return false; } done = true; resolve(cur); } }]);
    load(cur);
  });
}

function pickIcon(l, after) {
  let sel = libIcon(l);
  const grid = el('div', { class: 'icg' }, LIB_ICONS.map(n => el('button', { class: n === sel ? 'on' : '', title: n, onclick: e => { sel = n; grid.querySelectorAll('button').forEach(b => b.classList.toggle('on', b === e.currentTarget)); } }, icon(n))));
  modal('Icono de «' + l.name + '»', grid, [{ label: 'Cancelar' }, { label: 'Guardar', pri: true, fn: async () => { await api('/api/libraries/' + l.id, { method: 'PUT', body: { icon: sel } }); await refreshMeta(); after(); } }]);
}

/* Selector de categorías / series del catálogo Xtream para una biblioteca en la nube */
function cloudPicker(l, onSave) {
  const cats = new Set((l && l.cats) || []), sers = new Set((l && l.sers) || []);
  const filter = el('input', { class: 'inp', placeholder: 'Filtrar categorías…' });
  const list = el('div', { class: 'cpl' }, el('p', { class: 'mut', text: 'Cargando catálogo…' }));
  let all = [];
  function row(c) {
    const cb = el('input', { type: 'checkbox' });
    cb.checked = cats.has(c.id);
    cb.onchange = () => { cb.checked ? cats.add(c.id) : cats.delete(c.id); };
    const sub = el('div', { class: 'cps', hidden: true });
    const more = el('button', { class: 'btn sm', type: 'button', text: 'Series…' });
    more.onclick = async () => {
      if (!sub.hidden) { sub.hidden = true; return; }
      sub.hidden = false; sub.replaceChildren(el('span', { class: 'mut', text: 'Cargando…' }));
      try {
        const d = await api('/api/cloud/cat/' + encodeURIComponent(c.id));
        sub.replaceChildren(...(d.series || []).map(s => {
          const k = el('input', { type: 'checkbox' }); k.checked = sers.has(s.id);
          k.onchange = () => { k.checked ? sers.add(s.id) : sers.delete(s.id); };
          return el('label', { class: 'cpr' }, k, el('span', { text: s.n + (s.y ? ' (' + s.y + ')' : '') }));
        }));
        if (!sub.children.length) sub.textContent = 'Sin series';
      } catch (e) { sub.textContent = e.message; }
    };
    return el('div', { class: 'cpi', 'data-n': c.n.toLowerCase() }, el('label', { class: 'cpr' }, cb, el('span', { text: c.n })), more, sub);
  }
  filter.oninput = () => { const q = filter.value.trim().toLowerCase(); list.querySelectorAll('.cpi').forEach(x => { x.hidden = q && !x.dataset.n.includes(q); }); };
  api('/api/cloud/cats').then(d => {
    all = d.cats || [];
    list.replaceChildren(...(all.length ? all.map(row) : [el('p', { class: 'mut', text: 'Tu proveedor no tiene categorías de series.' })]));
  }).catch(e => list.replaceChildren(el('p', { class: 'mut', text: e.message })));
  modal('Series de tu Xtream' + (l && l.name ? ' · ' + l.name : ''), el('div', null, filter, list),
    [{ label: 'Cancelar' }, { label: 'Guardar', pri: true, fn: async () => {
      if (!cats.size && !sers.size) { toast('Elige al menos una categoría', true); return false; }
      try { await onSave({ cats: [...cats], sers: [...sers] }); } catch (e) { toast(e.message, true); return false; }
    } }]);
}

async function setLibs(body, tok) {
  const list = el('div'), status = el('div', { class: 'mut', style: 'margin-top:8px' });
  async function draw() {
    const d = await api('/api/libraries');
    if (tok !== navToken) return false;
    S.libs = d.libs; S.scan = d.scan;
    list.replaceChildren(...(d.libs.length ? d.libs.map(l => el('div', { class: 'lr' },
      el('button', { class: 'ib', title: 'Cambiar icono', onclick: () => pickIcon(l, draw) }, icon(libIcon(l))),
      el('div', { class: 'nm' }, el('b', { text: l.name }), el('small', { text: l.type === 'cloud' ? 'Catálogo de series de tu Xtream' : l.path })),
      el('span', { class: 'pill', text: l.type === 'cloud' ? 'Series en la nube' : l.type === 'series' ? 'Series' : 'Películas' }), l.type === 'cloud' ? el('span', { class: 'pill', text: (l.cats || []).length + ((l.cats || []).length === 1 ? ' categoría' : ' categorías') + ((l.sers || []).length ? ' · ' + l.sers.length + ' series' : '') }) : el('span', { class: 'pill', text: l.count + ' vídeos' }),
      l.type === 'cloud' ? el('button', { class: 'btn sm', onclick: () => cloudPicker(l, async r => { await api('/api/libraries/' + l.id, { method: 'PUT', body: { setCloud: true, cats: r.cats, sers: r.sers } }); toast('Guardado'); await refreshMeta(); draw(); }) }, icon('cloud'), 'Elegir series') : null,
      el('button', { class: 'btn sm', onclick: () => {
        const inp = el('input', { class: 'inp', value: l.name, 'aria-label': 'Nombre' });
        inp.onkeydown = e => { if (e.key === 'Enter') e.target.closest('.md').querySelector('.btn.pri').click(); };
        modal('Renombrar biblioteca', inp, [{ label: 'Cancelar' }, { label: 'Guardar', pri: true, fn: async () => {
          const v = inp.value.trim(); if (!v) { toast('Escribe un nombre', true); return false; }
          try { await api('/api/libraries/' + l.id, { method: 'PUT', body: { name: v } }); await refreshMeta(); draw(); buildNav(); } catch (e) { toast(e.message, true); return false; }
        } }]);
        setTimeout(() => { inp.focus(); inp.select(); }, 30);
      } }, 'Renombrar'),
      l.type === 'cloud' ? null : el('button', { class: 'btn sm', onclick: async () => { await post(`/api/libraries/${l.id}/scan`); toast('Escaneando…'); poll(); } }, icon('refresh'), 'Escanear'),
      el('button', { class: 'btn sm bad', onclick: () => modal('Quitar biblioteca', `Se quitará «${l.name}» de Lumina. Tus archivos no se borran.`, [{ label: 'Cancelar' }, { label: 'Quitar', bad: true, fn: async () => { await api('/api/libraries/' + l.id, { method: 'DELETE' }); await refreshMeta(); draw(); } }]) }, icon('trash'), 'Quitar'))) : [el('p', { class: 'mut', text: 'Todavía no hay bibliotecas.' })]));
    const s = d.scan;
    status.textContent = s.running ? `Escaneando… ${s.found} vídeos encontrados` : (s.pending > 0 ? `Preparando miniaturas y duración… quedan ${s.pending}` : '');
    return s.running || s.pending > 0;
  }
  let pt = 0;
  async function poll() { clearTimeout(pt); try { if (await draw()) pt = setTimeout(poll, 1500); else buildNav(); } catch { /* ignorar */ } }
  const name = el('input', { class: 'inp', placeholder: 'p. ej. Cine' }), path = el('input', { class: 'inp', placeholder: '/mnt/videos/cine   o   D:\\Videos\\Cine' });
  const type = el('select', { class: 'inp' }, el('option', { value: 'movies', text: 'Películas / vídeos sueltos' }), el('option', { value: 'series', text: 'Series (carpeta por serie)' }), el('option', { value: 'cloud', text: 'Series de tu Xtream (en la nube)' }));
  let pathField;
  body.append(
    el('div', { class: 'panel' }, el('h3', { text: 'Tus bibliotecas' }), el('p', { class: 'hint', text: 'Carpetas del servidor. Los títulos se reconocen por el nombre del archivo; si hay una carpeta con 4 o más películas se muestra como colección.' }), list, status),
    el('div', { class: 'panel' }, el('h3', { text: 'Añadir biblioteca' }),
      el('div', { class: 'two' }, field('Nombre', name), field('Tipo', type)),
      pathField = field('Carpeta en el servidor', el('div', { style: 'display:flex;gap:8px' }, path, el('button', { class: 'btn', onclick: async () => { const p = await pickFolder(path.value); if (p) { path.value = p; if (!name.value) name.value = p.split(/[\\/]/).filter(Boolean).pop() || ''; } } }, icon('folder'), 'Examinar'))),
      el('button', { class: 'btn pri', onclick: async e => {
        const b = e.currentTarget;
        if (type.value === 'cloud') {
          const nm = name.value || 'Series en la nube';
          cloudPicker({ name: nm }, async r => { await post('/api/libraries', { name: nm, type: 'cloud', cats: r.cats, sers: r.sers }); name.value = ''; toast('Biblioteca añadida'); await refreshMeta(); poll(); });
          return;
        }
        b.disabled = true;
        try { await post('/api/libraries', { name: name.value, type: type.value, path: path.value }); name.value = path.value = ''; toast('Biblioteca añadida'); await refreshMeta(); poll(); } catch (er) { toast(er.message, true); }
        b.disabled = false;
      } }, icon('plus'), 'Añadir y escanear')));
  type.onchange = () => { pathField.hidden = type.value === 'cloud'; };
  poll();
}

async function setTV(body, tok) {
  let sub = sessionStorage.getItem('tvsub') || 'conn';
  const wrap = el('div');
  const seg = el('div', { class: 'seg' }, [['conn', 'Conexión'], ['pick', 'Elegir canales'], ['mine', 'Mis canales']].map(([k, t]) =>
    el('button', { class: k === sub ? 'on' : '', text: t, onclick: () => { sub = k; sessionStorage.setItem('tvsub', k); seg.querySelectorAll('button').forEach(b => b.classList.toggle('on', b.textContent === t)); draw(); } })));
  body.append(seg, wrap);
  const draw = () => ({ conn: tvConn, pick: tvPick, mine: tvMine }[sub])(wrap, tok);
  draw();
}

async function tvConn(wrap, tok) {
  let x = await api('/api/xtream/config');
  const url = el('input', { class: 'inp', value: x.url, placeholder: 'http://servidor:puerto' }), user = el('input', { class: 'inp', value: x.user, autocomplete: 'off' });
  const pass = el('input', { class: 'inp', type: 'password', placeholder: x.hasPass ? '•••••••• (sin cambios)' : '', autocomplete: 'new-password' });
  const maxc = el('input', { class: 'inp', type: 'number', min: '0', max: '20', value: x.maxConn || '', placeholder: `Automático (${x.effectiveMax})` });
  const ua = el('input', { class: 'inp', value: x.ua, placeholder: 'VLC/3.0.20 LibVLC/3.0.20' });
  const msg = el('div'), st = el('div');
  const fmtDate = t => t ? new Date(t * 1000).toLocaleDateString() : '—';
  function drawSt() {
    const i = x.info || {};
    st.replaceChildren(...nn(el('dl', { class: 'kv' },
      el('dt', { text: 'Estado de la cuenta' }), el('dd', { text: x.url ? (i.status || '—') + (i.exp ? ' · caduca ' + fmtDate(i.exp) : '') : 'Sin configurar' }),
      el('dt', { text: 'Conexiones permitidas' }), el('dd', { text: x.url ? `${x.effectiveMax} (el proveedor indica ${i.max || '?'})` : '—' }),
      el('dt', { text: 'Canales en el catálogo' }), el('dd', { text: `${x.channels} en ${x.cats} categorías` + (x.catAt ? ` · actualizado ${new Date(x.catAt * 1000).toLocaleString()}` : '') }),
      el('dt', { text: 'Canales elegidos' }), el('dd', { text: String(x.selected) }),
      el('dt', { text: 'Guía de programación' }), el('dd', { text: x.epgAt ? `${x.epgChannels} canales con guía · ${new Date(x.epgAt * 1000).toLocaleString()}` : 'Sin descargar' })),
      x.busy ? el('p', { class: 'mut', style: 'margin-top:10px', text: 'Descargando lista de canales y guía… (puede tardar un minuto)' }) : null,
      x.error ? el('p', { class: 'err', text: x.error }) : null));
  }
  async function pollSt() { x = await api('/api/xtream/config'); if (tok !== navToken) return; drawSt(); if (x.busy) setTimeout(pollSt, 1500); else buildNav(); }
  wrap.replaceChildren(
    el('div', { class: 'panel' }, el('h3', { text: 'Servicio Xtream' }),
      el('p', { class: 'hint', text: 'Puedes pegar la URL M3U completa que te dio tu proveedor: se extraen usuario y contraseña. La contraseña se guarda solo en el servidor.' }),
      field('URL del servidor', url), el('div', { class: 'two' }, field('Usuario', user), field('Contraseña', pass)),
      el('div', { class: 'two' }, field('Conexiones simultáneas (opcional)', maxc), field('User-Agent (opcional)', ua)), msg,
      el('div', { style: 'display:flex;gap:10px;flex-wrap:wrap' },
        el('button', { class: 'btn pri', onclick: async e => {
          const b = e.currentTarget; b.disabled = true; msg.replaceChildren(el('div', { class: 'mut', text: 'Comprobando…' }));
          try { x = await post('/api/xtream/config', { url: url.value, user: user.value, pass: pass.value, maxConn: parseInt(maxc.value) || 0, ua: ua.value }); msg.replaceChildren(el('div', { class: 'okc', text: 'Conectado correctamente.' })); pass.value = ''; drawSt(); pollSt(); }
          catch (er) { msg.replaceChildren(el('div', { class: 'err', text: er.message })); }
          b.disabled = false;
        } }, 'Probar y guardar'),
        el('button', { class: 'btn', onclick: async () => { try { x = await post('/api/xtream/refresh'); drawSt(); pollSt(); } catch (er) { toast(er.message, true); } } }, icon('refresh'), 'Actualizar lista y guía'))),
    el('div', { class: 'panel' }, el('h3', { text: 'Estado' }), st));
  drawSt(); if (x.busy) pollSt();
}

async function tvPick(wrap, tok) {
  const c = await api('/api/xtream/categories');
  if (!c.total) { wrap.replaceChildren(el('div', { class: 'panel' }, el('p', { text: 'Primero configura la conexión y espera a que se descargue la lista de canales.' }))); return; }
  let cat = '', q = '', off = 0, total = 0, cats = c.cats;
  const catsEl = el('div', { class: 'cats' }), listEl = el('div', { class: 'cl' }), info = el('span', { class: 'mut' });
  const moreBtn = el('button', { class: 'btn sm', text: 'Cargar más', onclick: () => load(false) });
  function drawCats() {
    const sel = cats.reduce((a, k) => a + k.sel, 0);
    catsEl.replaceChildren(el('button', { class: cat === '' ? 'on' : '', onclick: () => { cat = ''; reset(); } }, el('span', { text: 'Todas las categorías' }), el('small', { text: `${sel}/${c.total}` })),
      ...cats.map(k => el('button', { class: cat === k.id ? 'on' : '', onclick: () => { cat = k.id; reset(); } }, el('span', { text: k.n }), el('small', { text: `${k.sel}/${k.total}` }))));
  }
  async function recount() { cats = (await api('/api/xtream/categories')).cats; drawCats(); }
  function reset() { off = 0; listEl.replaceChildren(); drawCats(); load(true); }
  async function load() {
    const d = await api(`/api/xtream/channels?cat=${encodeURIComponent(cat)}&q=${encodeURIComponent(q)}&off=${off}&lim=100`);
    if (tok !== navToken) return;
    total = d.total; off += d.items.length;
    for (const ch of d.items) {
      const lg = el('div', { class: 'lg' });
      if (ch.ic) { const im = el('img', { src: '/api/logo/' + ch.id, alt: '', loading: 'lazy' }); im.onerror = () => { im.remove(); lg.textContent = initials(ch.n); }; lg.append(im); } else lg.textContent = initials(ch.n);
      const cb = el('input', { type: 'checkbox', onchange: async () => { try { await post('/api/xtream/select', { ids: [ch.id], on: cb.checked }); recount(); } catch (e) { toast(e.message, true); cb.checked = !cb.checked; } } });
      cb.checked = ch.s;
      listEl.append(el('label', { class: 'cr' }, cb, lg, el('span', { class: 'nm', text: ch.n }), el('span', { class: 'pill' + (ch.e ? ' ok' : ''), text: ch.e ? 'Guía' : 'Sin guía' })));
    }
    info.textContent = `${total} canales`;
    moreBtn.hidden = off >= total;
  }
  const bulk = on => async () => {
    try { const r = await post('/api/xtream/select-query', { cat, q, on }); toast(`${r.changed} canales ${on ? 'añadidos' : 'quitados'}`); reset(); recount(); } catch (e) { toast(e.message, true); }
  };
  wrap.replaceChildren(el('div', { class: 'panel' }, el('h3', { text: 'Elegir canales' }), el('p', { class: 'hint', text: 'Marca los canales que quieres ver. Solo esos aparecerán en «TV en directo» y solo de ellos se descargará la guía.' }),
    el('div', { class: 'tools' }, el('input', { class: 'inp', type: 'search', placeholder: 'Buscar canal (p. ej. «deportes hd»)', oninput: debounce(e => { q = e.target.value.trim(); reset(); }, 250) }),
      el('button', { class: 'btn sm', onclick: bulk(true), text: 'Añadir todos los resultados' }), el('button', { class: 'btn sm', onclick: bulk(false), text: 'Quitar todos los resultados' }), info),
    el('div', { class: 'pick' }, catsEl, el('div', null, listEl, el('div', { style: 'padding:10px 0' }, moreBtn)))));
  drawCats(); load(true);
}

async function tvMine(wrap, tok) {
  const draw = async () => {
    const d = await api('/api/xtream/channels?sel=1&lim=300');
    if (tok !== navToken) return;
    const mv = async (id, to) => { await post('/api/xtream/move', { id, to }); draw(); };
    wrap.replaceChildren(el('div', { class: 'panel' }, el('h3', { text: `Mis canales (${d.total})` }), el('p', { class: 'hint', text: 'Este es el orden en que se muestran. Usa las flechas para reordenar.' }),
      d.items.length ? el('div', { class: 'cl', style: 'max-height:none' }, d.items.map((ch, i) => el('div', { class: 'cr', style: 'grid-template-columns:34px 44px minmax(0,1fr) auto' },
        el('span', { class: 'mut', style: 'text-align:center', text: i + 1 }), (() => { const lg = el('div', { class: 'lg' }); lg.textContent = initials(ch.n); if (ch.ic) { const im = el('img', { src: '/api/logo/' + ch.id, alt: '', loading: 'lazy' }); im.onerror = () => im.remove(); lg.textContent = ''; lg.append(im); } return lg; })(),
        el('span', { class: 'nm', text: ch.n }),
        el('span', { style: 'display:flex;gap:2px' },
          el('button', { class: 'pbtn', title: 'Al principio', onclick: () => mv(ch.id, 'top') }, icon('top')),
          el('button', { class: 'pbtn', title: 'Subir', onclick: () => mv(ch.id, 'up') }, icon('up')),
          el('button', { class: 'pbtn', title: 'Bajar', onclick: () => mv(ch.id, 'down') }, icon('down')),
          el('button', { class: 'pbtn', title: 'Quitar', onclick: async () => { await post('/api/xtream/select', { ids: [ch.id], on: false }); draw(); } }, icon('close')))))) : el('p', { class: 'mut', text: 'No has elegido ningún canal todavía.' })));
  };
  draw();
}

function updPanel() {
  const box = el('div', { class: 'panel' });
  let u = null;
  const ver = v => (v || '').replace(/^v/, '');
  async function load(check) {
    try { u = await api('/api/update' + (check ? '?check=1' : '')); } catch (e) { toast(e.message, true); return; }
    draw();
    if (u.busy) setTimeout(() => load(false), 1500);
  }
  function ask() {
    modal('Instalar ' + ver(u.latest), el('p', { text: 'Se descargará la versión, se verificará y Lumina se reiniciará (unos segundos). Los dispositivos conectados se reconectarán solos.' }),
      [{ label: 'Cancelar' }, { label: 'Actualizar', pri: true, fn: apply }]);
  }
  async function apply() {
    try { await post('/api/update/apply'); } catch (e) { toast(e.message, true); return; }
    box.replaceChildren(el('h3', { text: 'Actualizando…' }), el('p', { class: 'hint', text: 'Lumina se está reiniciando con la versión nueva. Esta página se recargará sola.' }));
    for (let i = 0; i < 60; i++) { await sleep(1500); try { const r = await fetch('/api/auth', { cache: 'no-store' }); if (r.ok) return location.reload(); } catch { /* reiniciando */ } }
    toast('No se ha podido confirmar el reinicio; recarga la página', true);
  }
  function draw() {
    const repo = el('input', { class: 'inp', value: u.repo || '', placeholder: 'usuario/repositorio', 'aria-label': 'Repositorio de GitHub' });
    const auto = el('input', { type: 'checkbox' }); auto.checked = !!u.auto;
    const pre = el('input', { type: 'checkbox' }); pre.checked = !!u.pre;
    const saveCfg = async () => { try { u = await post('/api/update/config', { repo: repo.value, auto: auto.checked, pre: pre.checked }); toast('Guardado'); draw(); } catch (e) { toast(e.message, true); } };
    const state = u.dev ? 'Compilación de desarrollo (no se actualiza sola)'
      : u.available ? `Hay una versión nueva: ${ver(u.latest)}`
      : u.latest ? 'Estás al día' : 'Aún no se ha comprobado';
    box.replaceChildren(...nn(
      el('h3', { text: 'Actualizaciones' }),
      el('dl', { class: 'kv', style: 'margin-top:12px' },
        el('dt', { text: 'Versión instalada' }), el('dd', { text: ver(u.version) }),
        el('dt', { text: 'Estado' }), el('dd', null, el('span', { class: 'pill ' + (u.available ? 'ok' : ''), text: state })),
        u.latest ? el('dt', { text: 'Última versión' }) : null, u.latest ? el('dd', { text: ver(u.latest) }) : null,
        u.checked ? el('dt', { text: 'Última comprobación' }) : null, u.checked ? el('dd', { text: new Date(u.checked * 1000).toLocaleString() }) : null),
      u.error ? el('p', { class: 'err', text: u.error }) : null,
      u.available && u.notes ? el('pre', { class: 'notes', text: u.notes }) : null,
      u.available && !u.hasAsset ? el('p', { class: 'err', text: 'La release no incluye el archivo para tu sistema (' + u.platform + ').' }) : null,
      el('div', { class: 'tools', style: 'margin-top:12px' },
        el('button', { class: 'btn', onclick: () => load(true), disabled: u.busy }, icon('refresh'), 'Buscar actualizaciones'),
        u.available && u.hasAsset && !u.dev ? el('button', { class: 'btn pri', onclick: ask, disabled: u.busy || !u.canApply }, u.busy ? (u.state || 'Actualizando') + '…' : 'Instalar ' + ver(u.latest)) : null),
      u.available && !u.canApply ? el('p', { class: 'hint', text: 'Para instalar desde la web hay que arrancar Lumina con contraseña (-password). También puedes actualizar descargando el binario de la release.' }) : null,
      el('div', { class: 'fr', style: 'margin-top:14px' }, el('label', { text: 'Repositorio de GitHub' }), repo),
      el('label', { class: 'chk' }, auto, ' Instalar automáticamente cuando nadie esté viendo nada', u.canApply ? '' : ' (requiere contraseña)'),
      el('label', { class: 'chk' }, pre, ' Incluir versiones de prueba (pre-releases)'),
      el('div', { class: 'tools', style: 'margin-top:10px' }, el('button', { class: 'btn sm', onclick: saveCfg }, 'Guardar'))));
  }
  load(false);
  return box;
}

async function metaPanel() {
  const c = await api('/api/system/meta');
  const key = el('input', { class: 'inp', placeholder: c.hasKey ? '•••••••• (clave guardada)' : 'Clave de la API de TMDB', autocomplete: 'off' });
  const st = el('span', { class: 'pill ' + (c.hasKey ? 'ok' : ''), text: c.hasKey ? 'TMDB activo' : 'Usando Wikipedia' });
  return el('div', { class: 'panel' }, el('h3', { text: 'Información de películas y series' }),
    el('p', { class: 'hint', text: 'En la ficha de cada título se muestra la sinopsis, los géneros y el reparto en castellano. Sin clave se usa la Wikipedia (solo sinopsis). Con una clave gratuita de TMDB (themoviedb.org → Ajustes → API) se obtiene la ficha completa.' }),
    el('div', { style: 'display:flex;gap:8px;align-items:center;flex-wrap:wrap' }, key,
      el('button', { class: 'btn pri', onclick: async e => {
        e.currentTarget.disabled = true;
        try { const r = await post('/api/system/meta', { key: key.value }); st.textContent = r.hasKey ? 'TMDB activo' : 'Usando Wikipedia'; st.className = 'pill ' + (r.hasKey ? 'ok' : ''); key.value = ''; toast(r.hasKey ? 'Clave de TMDB guardada' : 'Clave quitada'); } catch (er) { toast(er.message, true); }
        e.currentTarget.disabled = false;
      } }, 'Guardar'), st));
}

async function setSys(body, tok) {
  const d = await api('/api/system');
  const t = s => { s = Math.floor(s); const h = Math.floor(s / 3600), m = Math.floor(s % 3600 / 60); return h ? `${h} h ${m} min` : `${m} min`; };
  body.append(...nn(updPanel(), await metaPanel(), el('div', { class: 'panel' }, el('h3', { text: 'Sistema' }), el('dl', { class: 'kv', style: 'margin-top:12px' },
    el('dt', { text: 'Versión' }), el('dd', { text: 'Lumina ' + d.version }),
    el('dt', { text: 'Memoria en uso' }), el('dd', { text: `${d.heapMB.toFixed(1)} MB (reservada al sistema: ${d.sysMB.toFixed(0)} MB)` }),
    el('dt', { text: 'Tiempo encendido' }), el('dd', { text: t(d.uptime) }),
    el('dt', { text: 'Vídeos indexados' }), el('dd', { text: String(d.items) }),
    el('dt', { text: 'ffmpeg / ffprobe' }), el('dd', null, el('span', { class: 'pill ' + (d.ffmpeg ? 'ok' : 'bad'), text: d.ffmpeg ? 'ffmpeg disponible' : 'ffmpeg no encontrado' }), ' ', el('span', { class: 'pill ' + (d.ffprobe ? 'ok' : 'bad'), text: d.ffprobe ? 'ffprobe disponible' : 'ffprobe no encontrado' })),
    el('dt', { text: 'Acceso' }), el('dd', { text: d.password ? 'Protegido con contraseña' : 'Sin contraseña (cualquiera en tu red puede entrar)' }),
    el('dt', { text: 'Directos activos' }), el('dd', { text: d.live.length ? d.live.map(s => `${s.n} · ${s.v} ${s.v === 1 ? 'espectador' : 'espectadores'} · ${s.kbps} kbps`).join(' | ') : 'Ninguno' }))),
    !d.ffmpeg ? el('div', { class: 'panel' }, el('h3', { text: 'Recomendado: instala ffmpeg' }), el('p', { class: 'hint', text: 'Sin ffmpeg no hay miniaturas generadas, duración, ni conversión automática de audio AC3/DTS o contenedores AVI. Copia ffmpeg y ffprobe junto al ejecutable de Lumina, o instálalos en el sistema.' })) : null));
}

/* ================= perfiles ================= */
async function loadProfiles() {
  const d = await api('/api/profiles');
  S.profiles = d.profiles; S.maxProf = d.max;
  S.profile = d.profiles.find(p => p.id === d.current) || null;
  return d;
}
async function enterProfile(id) {
  await fetch('/api/profiles/' + id + '/select', { method: 'POST' });
  await loadProfiles();
  const w = $('#who'); if (w) w.remove();
  Player.closeSilent();
  await refreshMeta();
  if (location.hash && location.hash !== '#/' && location.hash !== '#') location.hash = '#/'; else route();
}

function showProfiles() {
  const old = $('#who'); if (old) old.remove();
  const root = el('div', { id: 'who', class: 'who', role: 'dialog', 'aria-label': '¿Quién está viendo?' });
  document.body.append(root);
  const logo = () => { const s = el('span'); s.innerHTML = '<svg viewBox="0 0 32 32"><rect width="32" height="32" rx="8" fill="#1a98ff"/><path d="M12 9l11 7-11 7z" fill="#fff"/></svg>'; return s.firstChild; };

  function editor(p) {
    const st = { name: p ? p.name : '', color: p ? p.color : (S.profiles.length % 6), kind: p ? p.kind : 'u' };
    const nameIn = el('input', { class: 'inp', value: st.name, maxlength: 20, placeholder: 'Nombre', 'aria-label': 'Nombre del perfil', oninput: e => { st.name = e.target.value; } });
    const prev = el('div', { class: 'avprev' }), pick = el('div', { class: 'avpick' }), kinds = el('div', { class: 'avpick' });
    const redraw = () => {
      prev.replaceChildren(avatar(st, 96));
      pick.replaceChildren(...AVC.map((_, i) => el('button', { class: i === st.color ? 'on' : '', 'aria-label': 'Color ' + (i + 1), onclick: () => { st.color = i; redraw(); } }, avatar({ color: i, kind: st.kind }, 46))));
      kinds.replaceChildren(...[['u', 'Silueta'], ['k', 'Niños']].map(([k, t]) => el('button', { class: 'btn sm' + (st.kind === k ? ' pri' : ''), text: t, onclick: () => { st.kind = k; redraw(); } })));
    };
    redraw();
    const body = el('div', null, prev, el('div', { class: 'fr' }, el('label', { text: 'Nombre' }), nameIn), el('div', { class: 'fr' }, el('label', { text: 'Avatar' }), kinds, pick));
    const acts = [{ label: 'Cancelar' }];
    if (p && S.profiles.length > 1) acts.unshift({ label: 'Eliminar', bad: true, fn: () => { askDelete(p); } });
    acts.push({ label: p ? 'Guardar' : 'Crear perfil', pri: true, fn: async () => {
      if (!st.name.trim()) { toast('Escribe un nombre', true); return false; }
      try {
        if (p) await fetch('/api/profiles/' + p.id, { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(st) }).then(r => { if (!r.ok) throw new Error('No se pudo guardar'); });
        else await post('/api/profiles', st);
        await loadProfiles(); draw();
      } catch (e) { toast(e.message, true); return false; }
    } });
    modal(p ? 'Editar perfil' : 'Nuevo perfil', body, acts);
    nameIn.focus();
  }
  function askDelete(p) {
    setTimeout(() => modal('Eliminar «' + p.name + '»', el('p', { text: 'Se borrarán su historial de «seguir viendo» y sus canales favoritos. Esta acción no se puede deshacer.' }), [{ label: 'Cancelar' }, { label: 'Eliminar perfil', bad: true, fn: async () => {
      try {
        const r = await fetch('/api/profiles/' + p.id, { method: 'DELETE' }); if (!r.ok) throw new Error((await r.json()).error || 'No se pudo eliminar');
        await loadProfiles(); if (!S.profile) { /* el activo era este */ } draw();
      } catch (e) { toast(e.message, true); return false; }
    } }]), 60);
  }
  function draw() {
    const cur = S.profile && S.profile.id;
    root.replaceChildren(
      el('div', { class: 'brand' }, logo(), 'Lumina'),
      el('h1', { text: '¿Quién está viendo Lumina?' }),
      el('div', { class: 'sub', text: 'Crea hasta ' + (S.maxProf || 6) + ' perfiles' }),
      el('div', { class: 'row' },
        S.profiles.map(p => el('div', { class: 'pfw' },
          el('button', { class: 'pfb' + (p.id === cur ? ' cur' : ''), 'aria-label': p.name, onclick: () => enterProfile(p.id) }, avatar(p), el('span', { text: p.name })),
          el('button', { class: 'pfe', onclick: () => editor(p) }, 'Editar'))),
        S.profiles.length < (S.maxProf || 6) ? el('div', { class: 'pfw' }, el('button', { class: 'pfb', onclick: () => editor(null), 'aria-label': 'Nuevo perfil' }, el('span', { class: 'add' }, icon('plus')), el('span', { text: 'Nuevo' }))) : null));
    const f = root.querySelector('.pfb.cur') || root.querySelector('.pfb'); if (f) f.focus();
  }
  draw();
}

/* ================= acceso ================= */
async function showLogin() {
  let acc = false;
  try { acc = !!(await fetch('/api/auth').then(r => r.json())).accounts; } catch { /* sin conexión */ }
  const form = el('form', { onsubmit: async e => {
    e.preventDefault();
    const er = form.querySelector('.err');
    try { await post('/api/login', { user: acc ? form.elements.user.value : '', password: form.elements.password.value }); location.hash = '#/'; location.reload(); } catch (x) { er.textContent = x.message; }
  } }, el('h1', { text: 'Lumina' }),
  acc ? el('input', { class: 'inp', name: 'user', type: 'text', placeholder: 'Usuario', autocomplete: 'username', autofocus: true, 'aria-label': 'Usuario' }) : null,
  el('input', { class: 'inp', name: 'password', type: 'password', placeholder: 'Contraseña', autocomplete: 'current-password', autofocus: !acc, 'aria-label': 'Contraseña' }),
  el('div', { class: 'err' }), el('button', { class: 'btn pri', type: 'submit', text: 'Entrar' }));
  document.body.replaceChildren(el('div', { class: 'login' }, form));
}

async function logout() { await fetch('/api/logout', { method: 'POST' }); location.hash = '#/'; location.reload(); }

function accountMenu() {
  const p = S.profile || {};
  modal(p.name || 'Mi cuenta', el('div', { style: 'text-align:center' }, avatar(p, 96), el('p', { class: 'mut', style: 'margin-top:12px', text: S.admin ? 'Administrador' : 'Usuario' })),
    [{ label: 'Cerrar' }, { label: 'Cerrar sesión', bad: true, fn: logout }]);
}

/* ================= usuarios (administradores) ================= */
async function setUsers(body, tok) {
  const d = await api('/api/users');
  if (tok !== navToken) return;
  const list = el('div');
  const libName = id => (d.libs.find(l => l.id === id) || {}).name || '?';
  function editor(u) {
    const st = u ? { ...u, password: '' } : { name: '', password: '', admin: !d.accounts, allLibs: true, libs: [], live: true, color: d.users.length % 6, kind: 'u' };
    const first = !d.accounts;
    const nameIn = el('input', { class: 'inp', value: st.name, maxlength: 24, placeholder: 'Usuario', autocomplete: 'off', 'aria-label': 'Usuario', oninput: e => { st.name = e.target.value; } });
    const passIn = el('input', { class: 'inp', type: 'password', placeholder: u ? 'Dejar vacío para no cambiarla' : 'Mínimo 6 caracteres', autocomplete: 'new-password', 'aria-label': 'Contraseña', oninput: e => { st.password = e.target.value; } });
    const chk = (label, get, set) => { const c = el('input', { type: 'checkbox', checked: get(), onchange: e => { set(e.target.checked); redraw(); } }); return el('label', { class: 'chk' }, c, ' ' + label); };
    const prev = el('div', { class: 'avprev' }), pick = el('div', { class: 'avpick' }), perms = el('div');
    const redraw = () => {
      prev.replaceChildren(avatar(st, 72));
      pick.replaceChildren(...AVC.map((_, i) => el('button', { class: i === st.color ? 'on' : '', 'aria-label': 'Color ' + (i + 1), onclick: () => { st.color = i; redraw(); } }, avatar({ color: i, kind: st.kind }, 38))));
      perms.replaceChildren(...nn(
        first ? el('p', { class: 'hint', text: 'Esta será la cuenta de administrador. Podrá gestionar bibliotecas, TV, usuarios y ajustes.' }) : chk('Administrador (puede gestionar todo)', () => st.admin, v => { st.admin = v; }),
        st.admin ? null : el('div', null,
          chk('Puede ver todas las bibliotecas', () => st.allLibs, v => { st.allLibs = v; }),
          st.allLibs ? null : el('div', { style: 'margin:4px 0 8px 22px' }, d.libs.length ? d.libs.map(l => chk(l.name, () => st.libs.includes(l.id), v => { st.libs = v ? [...st.libs, l.id] : st.libs.filter(x => x !== l.id); })) : el('p', { class: 'mut', text: 'Aún no hay bibliotecas.' })),
          chk('Puede ver la TV en directo', () => st.live, v => { st.live = v; }))));
    };
    redraw();
    const kinds = el('div', null, ...[['u', 'Normal'], ['k', 'Niños']].map(([k, t]) => el('button', { class: 'btn sm' + (st.kind === k ? ' pri' : ''), text: t, style: 'margin-right:6px', onclick: e => { st.kind = k; [...e.target.parentNode.children].forEach(b => b.classList.toggle('pri', b === e.target)); redraw(); } })));
    const bodyEl = el('div', null, prev, field('Usuario', nameIn), field(u ? 'Nueva contraseña' : 'Contraseña', passIn), field('Avatar', el('div', null, kinds, pick)), field('Permisos', perms));
    const acts = [{ label: 'Cancelar' }];
    if (u && !u.self) acts.unshift({ label: 'Eliminar', bad: true, fn: () => { setTimeout(() => modal('Eliminar «' + u.name + '»', el('p', { text: 'Se borrarán su cuenta, su historial de «seguir viendo» y sus favoritos. No se puede deshacer.' }), [{ label: 'Cancelar' }, { label: 'Eliminar usuario', bad: true, fn: async () => { try { await api('/api/users/' + u.id, { method: 'DELETE' }); toast('Usuario eliminado'); vSettings(['usr'], null, navToken); } catch (e) { toast(e.message, true); return false; } } }]), 60); } });
    acts.push({ label: u ? 'Guardar' : (first ? 'Activar cuentas' : 'Crear usuario'), pri: true, fn: async () => {
      if (st.name.trim().length < 2) { toast('Escribe un nombre de usuario', true); return false; }
      if (!u && st.password.length < 6) { toast('La contraseña debe tener al menos 6 caracteres', true); return false; }
      try {
        const b = { name: st.name, password: st.password, admin: st.admin, allLibs: st.allLibs, libs: st.libs, live: st.live, color: st.color, kind: st.kind };
        if (u) await api('/api/users/' + u.id, { method: 'PUT', body: b }); else await post('/api/users', b);
        toast(u ? 'Cambios guardados' : (first ? 'Cuentas activadas' : 'Usuario creado'));
        if (first) { location.reload(); return; }
        route(true);
      } catch (e) { toast(e.message, true); return false; }
    } });
    modal(u ? 'Editar usuario' : (first ? 'Activar cuentas: crea el administrador' : 'Nuevo usuario'), bodyEl, acts);
    nameIn.focus();
  }
  const me = S.me && S.me.id;
  const row = u => el('div', { class: 'lr' }, avatar(u, 40),
    el('div', { class: 'nm' }, el('b', { text: u.name + (u.id === me ? ' (tú)' : '') }), el('div', { class: 'mut', text:
      u.admin ? 'Administrador' : ((u.allLibs ? 'Todas las bibliotecas' : (u.libs.length ? u.libs.map(libName).join(', ') : 'Sin bibliotecas')) + (u.live ? ' · TV en directo' : '')) })),
    el('button', { class: 'btn sm', onclick: () => editor({ ...u, self: u.id === me }) }, 'Editar'));
  if (!d.accounts) {
    body.append(el('div', { class: 'panel' }, el('h3', { text: 'Cuentas de usuario' }),
      el('p', { class: 'hint', text: 'Ahora mismo Lumina funciona sin cuentas: quien entre ve todo. Activa las cuentas para que cada persona tenga su usuario y contraseña, su propio historial y permisos distintos (qué bibliotecas ve y si accede a la TV en directo).' }),
      el('button', { class: 'btn pri', onclick: () => editor(null) }, icon('plus'), 'Activar cuentas')));
    return;
  }
  body.append(el('div', { class: 'panel' }, el('h3', { text: 'Usuarios' }), list, el('div', { style: 'margin-top:12px' }, el('button', { class: 'btn pri', onclick: () => editor(null) }, icon('plus'), 'Nuevo usuario'))));
  list.replaceChildren(...d.users.map(row));
}

/* ================= arranque ================= */
(async function boot() {
  try {
    const a = await fetch('/api/auth').then(r => r.json());
    if (a.required && !a.ok) return showLogin();
    await loadProfiles();
    if (!S.profile && S.profiles.length === 1) { await fetch('/api/profiles/' + S.profiles[0].id + '/select', { method: 'POST' }); await loadProfiles(); }
    await refreshMeta();
    if (!S.profile) { showProfiles(); return; }
  } catch (e) { view.replaceChildren(el('div', { class: 'empty' }, el('h1', { text: 'No se puede conectar' }), el('p', { text: e.message }))); return; }
  route();
})();
})();
