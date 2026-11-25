(() => {
'use strict';
const { el, icon, api, post, toast, modal, fmtT, clock, initials, sleep, $ } = window.L;
const root = $('#player');
let st = null;           // sesión de reproducción activa
let lastLiveId = 0;      // último canal que ESTE cliente tuvo abierto (para reintentos al hacer zapping)
let mpegtsP = null;

const btn = (ic, label, fn, cls = '') => el('button', { class: 'pbtn ' + cls, 'aria-label': label, title: label, onclick: fn }, icon(ic));
const btnT = (txt, label, fn) => el('button', { class: 'pbtn t', 'aria-label': label, title: label, onclick: fn, text: txt });

function loadMpegts() {
  if (window.mpegts) return Promise.resolve(window.mpegts);
  return mpegtsP || (mpegtsP = new Promise((res, rej) => {
    const s = el('script', { src: '/vendor/mpegts.min.js' });
    s.onload = () => res(window.mpegts);
    s.onerror = () => { mpegtsP = null; rej(new Error('No se pudo cargar el reproductor de TV en directo')); };
    document.head.append(s);
  }));
}

/* ---------- interfaz común ---------- */
function openUI(kind) {
  teardown(false);
  // poster transparente: sin él, el WebView de Android y webOS pintan un icono de «play» gris de fondo
  const v = el('video', { playsinline: '', preload: 'auto', poster: 'data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7' });
  const ui = { v };
  ui.title = el('div', { class: 'ptit' });
  ui.sub = el('div', { class: 'psub' });
  ui.spin = el('div', { class: 'spin' });
  ui.msg = el('div', { class: 'pmsg', hidden: true });
  ui.top = el('div', { class: 'ptop' }, btn('back', 'Cerrar', close), el('div', { style: 'min-width:0' }, ui.title, ui.sub),
    kind === 'live' ? el('span', { class: 'liveb', style: 'margin-left:auto', text: 'EN DIRECTO' }) : null);
  ui.wrap = el('div', { class: 'pw' }, v, ui.top, ui.spin, ui.msg);
  st = { kind, ui, tok: 0, retries: 0 };

  const vol = parseFloat(localStorage.getItem('lvol') ?? '1');
  v.volume = isNaN(vol) ? 1 : vol;
  v.addEventListener('waiting', () => ui.spin.hidden = false);
  v.addEventListener('playing', () => { ui.spin.hidden = true; if (st) st.retries = 0; syncPP(); });
  v.addEventListener('canplay', () => ui.spin.hidden = true);
  v.addEventListener('pause', () => { syncPP(); if (st && st.kind === 'vod') saveProgress(); });
  v.addEventListener('play', syncPP);
  v.addEventListener('volumechange', () => { if (!st || !st.ui.vol) return; st.ui.vol.value = v.muted ? 0 : v.volume; st.ui.mute.replaceChildren(icon(v.muted || v.volume === 0 ? 'mute' : 'vol')); });
  ui.wrap.addEventListener('mousemove', wake);
  ui.wrap.addEventListener('pointerdown', wake);
  ui.wrap.addEventListener('click', e => { if ((e.target === v || e.target === ui.wrap) && st && st.kind === 'vod') toggle(); });
  ui.wrap.addEventListener('dblclick', e => { if (e.target === v) toggleFS(); });
  document.addEventListener('keydown', onKey, true);
  root.replaceChildren(ui.wrap);
  root.hidden = false;
  document.body.classList.add('noscroll');
  wake();
  return ui;
}

function wake() {
  if (!st) return;
  const w = st.ui.wrap;
  w.classList.remove('idle');
  clearTimeout(st.idleT);
  st.idleT = setTimeout(() => { if (st && (st.kind === 'live' || !st.ui.v.paused) && st.ui.msg.hidden && (!st.ui.tm || st.ui.tm.hidden)) st.ui.wrap.classList.add('idle'); }, 3200);
}
function syncPP() { if (st && st.ui.pp) st.ui.pp.replaceChildren(icon(st.ui.v.paused ? 'play' : 'pause')); }
function toggle() { const v = st.ui.v; v.paused ? v.play().catch(() => {}) : v.pause(); }
function toggleFS() {
  if (document.fullscreenElement) document.exitFullscreen();
  else (root.requestFullscreen || root.webkitRequestFullscreen || (() => {})).call(root);
}
function showMsg(text, actions) {
  const m = st.ui.msg;
  m.replaceChildren(el('p', { text }), el('div', { class: 'acts' }, (actions || []).map(a => el('button', { class: 'btn' + (a.pri ? ' pri' : ''), text: a.label, onclick: () => { hideMsg(); a.fn && a.fn(); } }))));
  m.hidden = false; st.ui.spin.hidden = true; st.ui.wrap.classList.remove('idle');
  (m.querySelector('.btn.pri') || m.querySelector('.btn'))?.focus();
}
function hideMsg() { if (st) st.ui.msg.hidden = true; }

function volumeControls(ui) {
  ui.mute = btn('vol', 'Silenciar', () => { ui.v.muted = !ui.v.muted; });
  ui.vol = el('input', { class: 'vol', type: 'range', min: '0', max: '1', step: '0.05', 'aria-label': 'Volumen', value: ui.v.volume,
    oninput: e => { ui.v.muted = false; ui.v.volume = +e.target.value; localStorage.setItem('lvol', e.target.value); } });
  return [ui.mute, ui.vol];
}

function onKey(e) {
  if (!st || !$('#modal').hidden) return;
  if (e.target.tagName === 'INPUT' && e.target.type !== 'range') return;
  const v = st.ui.v, live = st.kind === 'live';
  if (st.ui.tm && !st.ui.tm.hidden) {   // menú de audio/subtítulos abierto
    if (e.key === 'Escape' || e.key === 'c' || e.keyCode === 461 || e.keyCode === 10009) { e.preventDefault(); e.stopPropagation(); toggleMenu(false); return; }
    if (e.target.closest && e.target.closest('.tmenu')) return;
  }
  let h = true;
  const kc = { 461: 'Escape', 10009: 'Escape', 179: 'MediaPlayPause', 415: 'MediaPlay', 19: 'MediaPause', 417: 'MediaFastForward', 412: 'MediaRewind', 413: 'Escape', 405: 'i' }[e.keyCode];
  switch (kc || e.key) {
    case 'Escape': case 'GoBack': case 'BrowserBack': close(); break;
    case 'Enter': if (e.target === document.body || e.target === v || e.target === st.ui.wrap) { if (live) toggleGuide(); else toggle(); } else h = false; break;
    case 'MediaPlayPause': case 'MediaPlay': case 'MediaPause': if (!live) toggle(); break;
    case 'MediaTrackNext': if (live) zap(1); else if (st.it && st.it.next) next(); break;
    case 'MediaTrackPrevious': if (live) zap(-1); break;
    case 'MediaFastForward': if (!live) skip(30); break;
    case 'MediaRewind': if (!live) skip(-30); break;
    case ' ': case 'k': if (!live) toggle(); break;
    case 'ArrowLeft': if (!live) skip(e.shiftKey ? -30 : -10); break;
    case 'ArrowRight': if (!live) skip(e.shiftKey ? 30 : 10); break;
    case 'ArrowUp': if (live) zap(1); else { v.muted = false; v.volume = Math.min(1, v.volume + .05); } break;
    case 'ArrowDown': if (live) zap(-1); else v.volume = Math.max(0, v.volume - .05); break;
    case 'PageUp': if (live) zap(1); break;
    case 'PageDown': if (live) zap(-1); break;
    case 'f': toggleFS(); break;
    case 'm': v.muted = !v.muted; break;
    case 'c': if (!live) toggleMenu(); break;
    case 'i': if (live) toggleGuide(); break;
    case 'n': if (!live && st.it && st.it.next) next(); break;
    default: h = false;
  }
  if (h) { e.preventDefault(); e.stopPropagation(); wake(); }
}

function close() { const had = !!st; teardown(true); if (had && window.L.refresh) { window.L.refresh(); setTimeout(() => { if (!st && window.L.refresh) window.L.refresh(); }, 1500); } }

function teardown(save) {
  if (!st) return;
  if (st.kind === 'vod') saveProgress();
  const s = st; st = null;
  clearTimeout(s.idleT); clearTimeout(s.pendT); clearInterval(s.timer); clearInterval(s.wd);
  if (s.mp) { try { s.mp.pause(); s.mp.unload(); s.mp.detachMediaElement(); s.mp.destroy(); } catch { /* ya liberado */ } }
  try { s.ui.v.pause(); s.ui.v.removeAttribute('src'); s.ui.v.load(); } catch { /* nada */ }
  root.replaceChildren(); root.hidden = true;
  document.body.classList.remove('noscroll');
  document.removeEventListener('keydown', onKey, true);
  if (document.fullscreenElement) document.exitFullscreen().catch(() => {});
}

/* ================= VÍDEO BAJO DEMANDA ================= */
const dur = () => (st.dur || (isFinite(st.ui.v.duration) ? st.ui.v.duration : 0));
const cur = () => st.wait != null ? st.wait : st.base + (st.ui.v.currentTime || 0); // st.wait: buscando el fotograma clave

function saveProgress() {
  if (!st || st.kind !== 'vod' || !st.ui.v.src) return;
  const c = cur(), d = dur();
  if (c < 1) return;
  st.lastSave = Date.now();
  api('/api/progress', { method: 'POST', body: { id: st.id, pos: c, dur: d }, keepalive: true }).catch(() => {});
}
addEventListener('pagehide', () => { if (st) saveProgress(); });

// Episodio de la nube: ocupa la conexión del proveedor. Si hay otra cosa en marcha, se pregunta antes de cortarla.
async function cloudSlot(id, it, force = false) {
  try { await post('/api/vodtune', { id, force }); return true; }
  catch (e) {
    if (e.status !== 409 || !e.data) { toast(e.message, true); return false; }
    const who = e.data.playing.map(p => `«${p.n}»${p.v > 1 ? ` (${p.v} personas)` : ''}`).join(', ');
    const n = e.data.max;
    return new Promise(res => {
      let done = false; const fin = v => { if (!done) { done = true; res(v); } };
      modal('Conexión ocupada', `Tu proveedor permite ${n} ${n === 1 ? 'conexión' : 'conexiones'} a la vez y ahora se está viendo ${who}. Si reproduces «${it.series || it.title}», esa emisión se cortará.`, [
        { label: 'Cancelar', fn: () => fin(false) },
        { label: 'Cambiar', pri: true, fn: async () => { fin(await cloudSlot(id, it, true)); } }]);
    });
  }
}

async function vod(id, opts = {}) {
  let it;
  try { it = await api('/api/item/' + id); } catch (e) { toast(e.message, true); return; }
  if (it.cloud && !(await cloudSlot(id, it))) return;
  const fin = it.dur > 0 && it.pos >= it.dur * .95;
  let start = opts.start;
  if (start == null || start < 0) start = (it.pos > 10 && !fin) ? it.pos : 0;

  const ui = openUI('vod');
  Object.assign(st, { id, it, mode: it.plan, dur: it.dur, base: 0, startAt: 0, lastSave: 0 });
  // en televisores (LG webOS) ni el .mkv directo ni el MP4 por copia de vídeo se ven (audio sin imagen o cortes): se recodifica a H.264 estándar
  if (tvX264(it) && (st.mode === 'remux' || (st.mode === 'direct' && it.ext === '.mkv'))) st.mode = 'x264';
  const isEp = it.kind === 'episode';
  ui.title.textContent = isEp ? it.series : it.title;
  ui.sub.textContent = isEp ? `${it.season ? 'T' + it.season + ' · ' : ''}E${it.ep || ''} — ${it.title}` : [it.year || '', it.dur ? Math.max(1, Math.round(it.dur / 60)) + ' min' : ''].filter(Boolean).join(' · ');

  // barra de progreso
  ui.bf = el('div', { class: 'bf' }); ui.cu = el('div', { class: 'cu' });
  ui.kn = el('div', { class: 'kn' }); ui.tip = el('div', { class: 'tip' });
  ui.skb = el('div', { class: 'skb', role: 'slider', 'aria-label': 'Posición' }, el('div', { class: 'tr' }, ui.bf, ui.cu), ui.kn, ui.tip);
  let drag = false;
  const ratio = e => { const r = ui.skb.getBoundingClientRect(); return Math.min(1, Math.max(0, (e.clientX - r.left) / r.width)); };
  const paint = r => { ui.cu.style.width = r * 100 + '%'; ui.kn.style.left = r * 100 + '%'; };
  ui.skb.addEventListener('pointerdown', e => { ui.skb.setPointerCapture(e.pointerId); drag = true; ui.skb.classList.add('drag'); paint(ratio(e)); });
  ui.skb.addEventListener('pointermove', e => { const r = ratio(e); ui.tip.style.left = r * 100 + '%'; ui.tip.textContent = fmtT(r * dur()); if (drag) paint(r); });
  ui.skb.addEventListener('pointerup', e => { if (!drag) return; drag = false; ui.skb.classList.remove('drag'); seek(ratio(e) * dur()); });
  st.paint = r => { if (!drag) paint(r); };

  ui.pp = btn('play', 'Reproducir / pausa', toggle);
  ui.time = el('span', { class: 'tm', text: '0:00' });
  ui.cc = btn('cc', 'Audio y subtítulos (C)', () => toggleMenu()); ui.cc.hidden = true;
  ui.fsb = btn('fs', 'Pantalla completa (F)', toggleFS);
  ui.nextb = el('button', { class: 'btn pri nextb', hidden: true, onclick: next }, icon('next'), 'Siguiente episodio');
  ui.bot = el('div', { class: 'pbot' }, ui.skb,
    el('div', { class: 'ctl' }, ui.pp, btnT('−10', 'Retroceder 10 s', () => skip(-10)), btnT('+10', 'Avanzar 10 s', () => skip(10)),
      ...volumeControls(ui), ui.time, el('span', { class: 'grow' }), ui.cc, ui.fsb));
  ui.wrap.append(ui.bot, ui.nextb);

  const v = ui.v;
  it.audio = it.audio || []; it.esubs = it.esubs || []; it.subs = it.subs || [];
  st.audio = 0; st.sub = 'off';
  // idioma preferido de la última vez (se recuerda al elegir)
  const pa = localStorage.getItem('alang'), ps = localStorage.getItem('slang');
  if (pa && it.audio.length > 1) { const k = it.audio.findIndex(t => lc(t.l) === pa); if (k > 0) { st.audio = k; st.mode = it.plan === 'x264' || tvX264(it) ? 'x264' : 'remux'; } }
  if (ps && ps !== 'off') {
    const e = it.esubs.find(t => lc(t.l) === ps); const f = it.subs.findIndex(t => lc(t.l) === ps);
    if (e) st.sub = 'e' + e.i; else if (f >= 0) st.sub = 'f' + it.subs[f].n;
  }
  ui.cc.hidden = !(it.subs.length || it.esubs.length || it.audio.length > 1);
  buildMenu();
  v.addEventListener('loadedmetadata', () => {
    if (!st) return;
    if (st.mode === 'direct' && st.startAt > 0) { v.currentTime = st.startAt; st.startAt = 0; }
    if (!st.dur && isFinite(v.duration)) st.dur = v.duration;
  });
  v.addEventListener('timeupdate', tick);
  v.addEventListener('ended', onEnded);
  v.addEventListener('error', () => onVideoErr('error del vídeo: ' + verr(v)));
  // vigilante de imagen negra: si el audio avanza pero el decodificador no da vídeo (videoWidth 0), se pasa al modo siguiente
  let bt = 0, at = 0;
  const blk = setInterval(() => {
    if (!st || st.kind !== 'vod' || st.ui.v !== v) { clearInterval(blk); return; }
    if (v.paused || v.ended || v.readyState < 2 || v.currentTime < 0.5) { bt = 0; return; }
    // sin audio: el vídeo se decodifica pero el audio no (AC3/E-AC3/DTS que el navegador no admite) → conversión del audio
    if (st.mode === 'direct' && st.it.ffmpeg && v.currentTime > 2.5 && noAudio(v)) { if (++at >= 2) { at = 0; onVideoErr('sin audio'); return; } } else at = 0;
    if (v.videoWidth > 0) { bt = 0; return; }
    if (++bt >= 3) { bt = 0; onVideoErr('imagen negra'); }
  }, 1000);
  load(start);
  if (start > 10 && it.dur) { // aviso de reanudación con opción de empezar de nuevo
    ui.rs = el('button', { class: 'btn rsb', onclick: () => { ui.rs.remove(); seek(0); } }, icon('refresh'), 'Empezar desde el principio');
    ui.wrap.append(ui.rs); setTimeout(() => ui.rs && ui.rs.remove(), 9000);
  }
}

function tvX264(it) { return !!it.ffmpeg && document.documentElement.classList.contains('tv'); }

function killMp() { if (st && st.mp) { try { st.mp.pause(); st.mp.unload(); st.mp.detachMediaElement(); st.mp.destroy(); } catch { /* nada */ } st.mp = null; } }

// televisores: la conversión se envía como MPEG-TS al reproductor MSE (el mismo que el directo, que sí funciona en LG)
function loadTS(t) {
  const v = st.ui.v, tok = st.tsTok = (st.tsTok || 0) + 1;
  killMp(); st.base = t; st.wait = null; st.pend = null; st.tsDone = false; st.ui.spin.hidden = false; hideMsg();
  const fallback = () => { if (!st || tok !== st.tsTok) return; killMp(); st.noTS = true; load(cur()); };
  loadMpegts().then(mp => {
    if (!st || tok !== st.tsTok) return;
    if (!mp.isSupported()) return fallback();
    const p = mp.createPlayer({ type: 'mpegts', isLive: true, url: `${location.origin}/remux/${st.id}?ss=${t.toFixed(1)}&a=${st.audio}&m=x264&f=ts` },
      { enableWorker: true, liveBufferLatencyChasing: false, lazyLoad: false, autoCleanupSourceBuffer: true, autoCleanupMaxBackwardDuration: 30, autoCleanupMinBackwardDuration: 15 });
    st.mp = p;
    p.on(mp.Events.ERROR, (a, b, c) => { console.warn('[Lumina] error MSE:', a, b, c && (c.msg || c.code)); report(`${what()} error del reproductor MSE: ${a} ${b} ${c && (c.msg || c.code) || ''}`); fallback(); });
    p.on(mp.Events.LOADING_COMPLETE, () => { if (st && tok === st.tsTok) st.tsDone = true; });
    p.attachMediaElement(v); p.load();
    const pr = v.play();
    if (pr) pr.catch(e => { if (e.name === 'NotAllowedError') showMsg('Pulsa para reproducir', [{ label: 'Reproducir', pri: true, fn: () => v.play() }]); });
  }, fallback);
}

// informe al registro del servidor (Ajustes → Depuración)
function report(msg) {
  try { fetch('/api/clientlog', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ msg }), keepalive: true }).catch(() => {}); } catch { /* nada */ }
}
const what = () => st && st.it ? (st.it.series ? `«${st.it.series}» ${st.it.season}x${st.it.ep}` : `«${st.it.title}»`) : '?';
const verr = v => { const e = v && v.error; return e ? `código ${e.code}${e.message ? ' ' + e.message : ''}` : 'sin detalle'; };

function noAudio(v) {
  if (typeof v.webkitAudioDecodedByteCount === 'number') return v.webkitAudioDecodedByteCount === 0 && v.webkitVideoDecodedByteCount > 0;
  if (typeof v.mozHasAudio === 'boolean') return !v.mozHasAudio;
  return false;
}

function load(t) {
  const v = st.ui.v;
  if (st.mode !== 'direct' && !st.noTS && tvX264(st.it)) return loadTS(t);
  killMp();
  st.base = 0; st.pend = null; st.ui.spin.hidden = false; hideMsg();
  v.preload = 'auto';
  const lt = st.loadTok = (st.loadTok || 0) + 1;
  const go = k => {
    if (!st || lt !== st.loadTok) return;
    st.wait = null; st.base = k; v.src = `/remux/${st.id}?ss=${k.toFixed(3)}&a=${st.audio}` + (st.mode === 'x264' ? '&m=x264' : '');
    applySub(); playV(v);
  };
  st.wait = null;
  if (st.mode === 'direct') { v.src = '/stream/' + st.id + (t > 1 ? '#t=' + t.toFixed(1) : ''); st.startAt = 0; }  // #t=: el navegador pide directamente ese tramo
  // copia del vídeo: se empieza en el fotograma clave anterior (si no, el audio queda desfasado)
  else if (st.mode === 'remux' && t > 1) { st.wait = t; v.pause(); v.removeAttribute('src'); api(`/api/keyframe/${st.id}?t=${t.toFixed(3)}`).then(r => go(typeof r.k === 'number' ? r.k : t), () => go(t)); return; }
  else return go(t);
  applySub(); playV(v);
}

function playV(v) {
  const p = v.play();
  if (p) p.catch(e => { if (e.name === 'NotAllowedError') showMsg('Pulsa para reproducir', [{ label: 'Reproducir', pri: true, fn: () => v.play() }]); });
}

function seek(t) {
  const d = dur();
  t = Math.max(0, d ? Math.min(t, d - 1) : t);
  if (st.mode === 'direct') st.ui.v.currentTime = t; else load(t);
}
// En modo conversión cada salto reinicia ffmpeg: se agrupan las pulsaciones seguidas en un único salto.
function skip(s) {
  if (!st) return;
  if (st.mode === 'direct') return seek(cur() + s);
  st.pend = (st.pend != null ? st.pend : cur()) + s;
  if (st.ui.time) st.ui.time.textContent = fmtT(Math.max(0, st.pend)) + (dur() ? ' / ' + fmtT(dur()) : '');
  clearTimeout(st.pendT);
  st.pendT = setTimeout(() => { if (!st || st.pend == null) return; const t = st.pend; st.pend = null; seek(t); }, 450);
}

function tick() {
  if (!st || st.kind !== 'vod') return;
  const d = dur(), c = cur(), v = st.ui.v;
  st.paint(d ? Math.min(1, c / d) : 0);
  st.ui.time.textContent = d ? `${fmtT(c)} / ${fmtT(d)}` : fmtT(c);
  if (v.buffered.length && d) st.ui.bf.style.width = Math.min(100, (st.base + v.buffered.end(v.buffered.length - 1)) / d * 100) + '%';
  if (Date.now() - st.lastSave > 10000) saveProgress();
  if (st.it.next) st.ui.nextb.hidden = !(d && d - c < 30);
  // fin del flujo MPEG-TS: el reproductor MSE no dispara «ended» en este modo
  if (st.tsDone && !st.ended && d && c > d - 4 && v.buffered.length && v.buffered.end(v.buffered.length - 1) - v.currentTime < 0.6) { st.ended = true; onEnded(); }
}

function onVideoErr(why) {
  if (!st || st.kind !== 'vod') return;
  const order = ['direct', 'remux', 'x264'], i = order.indexOf(st.mode), t = cur();
  why = typeof why === 'string' ? why : 'error';
  if (st.it.ffmpeg && i < 2) {
    report(`${what()} ${st.mode} → ${order[i + 1]}: ${why}`);
    st.mode = order[i + 1];
    st.ui.v.querySelectorAll('track').forEach(x => x.remove()); st.ui.cc.hidden = true; // los subtítulos solo van en reproducción directa
    toast(st.mode === 'remux' ? 'Adaptando el audio para tu navegador…' : 'Recodificando el vídeo para tu navegador…');
    load(t); return;
  }
  report(`${what()} no se puede reproducir (modo ${st.mode}): ${why}`);
  showMsg('Este vídeo no se puede reproducir en este navegador' + (st.it.ffmpeg ? '.' : ' (instala ffmpeg en el servidor para convertirlo al vuelo).'), [{ label: 'Cerrar', pri: true, fn: close }]);
}

function onEnded() {
  if (!st || st.kind !== 'vod') return;
  const it = st.it;
  api('/api/progress', { method: 'POST', body: { id: st.id, pos: dur() || cur(), dur: dur() || cur() }, keepalive: true }).catch(() => {});
  if (it.next) { const n = it.next; vod(n, { start: 0 }); } else close();
}

function next() { if (!st || !st.it.next) return; const n = st.it.next; saveProgress(); vod(n, { start: 0 }); }

/* ---- audio y subtítulos ---- */
const LN = { es: 'Español', en: 'Inglés', fr: 'Francés', de: 'Alemán', it: 'Italiano', pt: 'Portugués', ja: 'Japonés', ko: 'Coreano', zh: 'Chino', ru: 'Ruso', ca: 'Catalán', eu: 'Euskera', gl: 'Gallego', ar: 'Árabe', nl: 'Neerlandés', pl: 'Polaco', tr: 'Turco', sv: 'Sueco', hi: 'Hindi' };
const L3 = { spa: 'es', esl: 'es', eng: 'en', fre: 'fr', fra: 'fr', ger: 'de', deu: 'de', ita: 'it', por: 'pt', jpn: 'ja', kor: 'ko', chi: 'zh', zho: 'zh', rus: 'ru', cat: 'ca', eus: 'eu', baq: 'eu', glg: 'gl', ara: 'ar', dut: 'nl', nld: 'nl', pol: 'pl', tur: 'tr', swe: 'sv', hin: 'hi' };
function lc(l) { l = (l || '').toLowerCase().trim(); if (!l || l === 'und') return ''; if (L3[l]) return L3[l]; const m = l.match(/^[a-z]{2,3}/); return m ? (L3[m[0]] || m[0].slice(0, 2)) : ''; }
const lname = l => { const c = lc(l); return c ? (LN[c] || c.toUpperCase()) : ''; };
const CH = { 1: '1.0', 2: '2.0', 6: '5.1', 8: '7.1' };
const audLabel = (t, i) => [lname(t.l) || 'Pista ' + (i + 1), t.n && !/^\s*$/.test(t.n) && t.n.toLowerCase() !== (lname(t.l) || '').toLowerCase() ? t.n : '', (t.c || '').toUpperCase().replace('EAC3', 'E-AC3') + (t.ch ? ' ' + (CH[t.ch] || t.ch + 'ch') : '')].filter(Boolean).join(' · ');
const subLabel = (t) => [lname(t.l) || 'Subtítulos', t.n && t.n.toLowerCase() !== (lname(t.l) || '').toLowerCase() ? t.n : ''].filter(Boolean).join(' · ');

function buildMenu() {
  const ui = st.ui, it = st.it;
  const opt = (label, on, fn) => el('button', { class: 'tmo' + (on ? ' on' : ''), role: 'menuitemradio', 'aria-checked': on ? 'true' : 'false', onclick: fn }, el('span', { class: 'ck', text: on ? '✓' : '' }), label);
  const cols = [];
  if (it.audio.length > 1) cols.push(el('div', { class: 'tcol' }, el('h4', { text: 'Audio' }),
    it.audio.map((t, i) => opt(audLabel(t, i), st.audio === i, () => setAudio(i)))));
  if (it.subs.length || it.esubs.length) cols.push(el('div', { class: 'tcol' }, el('h4', { text: 'Subtítulos' }),
    opt('Desactivados', st.sub === 'off', () => setSub('off')),
    it.subs.map(t => opt(t.l, st.sub === 'f' + t.n, () => setSub('f' + t.n, lc(t.l)))),
    it.esubs.map(t => opt(subLabel(t), st.sub === 'e' + t.i, () => setSub('e' + t.i, lc(t.l))))));
  const tm = el('div', { class: 'tmenu', role: 'menu', hidden: true }, cols);
  tm.addEventListener('keydown', e => {   // en PC: ↑/↓ recorren las opciones (en TV lo hace tv.js)
    if (document.documentElement.classList.contains('tv')) return;
    if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp') return;
    const b = [...tm.querySelectorAll('button')], i = b.indexOf(document.activeElement);
    b[(i + (e.key === 'ArrowDown' ? 1 : -1) + b.length) % b.length].focus(); e.preventDefault();
  });
  if (ui.tm) ui.tm.replaceWith(tm); else ui.wrap.append(tm);
  ui.tm = tm;
}
function toggleMenu(open) {
  const tm = st && st.ui.tm; if (!tm) return;
  const show = open === undefined ? tm.hidden : open;
  tm.hidden = !show;
  if (show) { st.ui.wrap.classList.remove('idle'); (tm.querySelector('.on') || tm.querySelector('button'))?.focus(); }
  else st.ui.pp && st.ui.pp.focus();
}
function setAudio(i) {
  if (!st || i === st.audio) return toggleMenu(false);
  st.audio = i;
  const lang = lc(st.it.audio[i] && st.it.audio[i].l); if (lang) localStorage.setItem('alang', lang);
  // cambiar de pista exige conversión en el servidor (los navegadores no permiten elegir audio); la pista 0 vuelve al modo original
  st.mode = i === 0 ? st.it.plan : (st.it.plan === 'x264' || tvX264(st.it) ? 'x264' : 'remux');
  const t = cur(); buildMenu(); toggleMenu(false); load(t);
  toast('Audio: ' + audLabel(st.it.audio[i], i));
}
function setSub(k, lang) {
  if (!st) return;
  st.sub = k; localStorage.setItem('slang', k === 'off' ? 'off' : (lang || 'off'));
  applySub(); buildMenu(); toggleMenu(false);
  toast(k === 'off' ? 'Subtítulos desactivados' : 'Subtítulos activados');
}
// las pistas de texto se recrean al cambiar de subtítulo o de punto de inicio (en modo conversión los tiempos se desplazan)
function applySub() {
  if (!st || st.kind !== 'vod') return;
  const v = st.ui.v;
  [...v.querySelectorAll('track')].forEach(t => t.remove());
  if (!st.sub || st.sub === 'off') return;
  const off = st.mode === 'direct' ? 0 : st.base;
  const tr = el('track', { kind: 'subtitles', label: 'Lumina', src: `/sub/${st.id}/${st.sub[0] === 'e' ? st.sub : st.sub.slice(1)}.vtt` + (off > 0.5 ? `?off=${off.toFixed(1)}` : '') });
  v.append(tr);
  tr.track.mode = 'showing';
}

/* ================= TV EN DIRECTO ================= */
async function live(chan, list) {
  const ui = openUI('live');
  st.chan = chan; st.list = list && list.length ? list : [chan];
  try { localStorage.setItem('lumina.lastlive', String(chan.id)); } catch { /* nada */ }
  ui.lg = el('div', { class: 'lg' });
  ui.name = el('b'); ui.nowT = el('div'); ui.bar = el('div', { class: 'bar' }, el('i')); ui.nextT = el('div', { class: 'nx' });
  ui.guide = el('div', { class: 'guide', hidden: true });
  const zp = el('div', { class: 'zp' }, btn('up', 'Canal siguiente (↑)', () => zap(1)), btn('down', 'Canal anterior (↓)', () => zap(-1)));
  ui.lvi = el('div', { class: 'lvi' }, ui.lg, el('div', { class: 'tx' }, ui.name, ui.nowT, ui.bar, ui.nextT),
    el('div', { style: 'display:flex;align-items:center;gap:2px' }, ...volumeControls(ui), btn('info', 'Guía (I)', toggleGuide), btn('fs', 'Pantalla completa (F)', toggleFS), zp));
  ui.wrap.append(ui.lvi, ui.guide);
  const v = ui.v;
  v.addEventListener('ended', () => onLiveErr(st && st.tok));
  st.timer = setInterval(refreshInfo, 15000);
  try { st.clean = Number(localStorage.getItem('lumina.livemode')) || 0; } catch { st.clean = 0; } // empieza por el modo que funcionó la última vez
  await tune(false);
}

function updateInfo() {
  const c = st.chan, ui = st.ui;
  ui.title.textContent = c.n; ui.sub.textContent = c.cn || '';
  ui.name.textContent = c.n;
  ui.lg.replaceChildren();
  if (c.ic) { const im = el('img', { src: '/api/logo/' + c.id, alt: '' }); im.onerror = () => { im.remove(); ui.lg.textContent = initials(c.n); }; ui.lg.append(im); } else ui.lg.textContent = initials(c.n);
  paintNow(c.now, c.next);
  refreshInfo();
}
function paintNow(now, nx) {
  const ui = st.ui;
  ui.nowT.textContent = now ? `${now.t || '—'}  ·  ${clock(now.s)} – ${clock(now.e)}` : 'Sin información de programación';
  ui.bar.hidden = !now;
  if (now) ui.bar.firstChild.style.width = Math.min(100, Math.max(0, (Date.now() / 1000 - now.s) / Math.max(1, now.e - now.s) * 100)) + '%';
  ui.nextT.textContent = nx ? 'A continuación · ' + (nx.t || '—') : '';
}
async function refreshInfo() {
  if (!st || st.kind !== 'live') return;
  const id = st.chan.id;
  try {
    const d = await api('/api/live/epg/' + id);
    if (!st || st.chan.id !== id) return;
    const now = Date.now() / 1000, p = d.progs;
    const cur = p.find(x => x.s <= now && x.e > now), nx = p.find(x => x.s >= (cur ? cur.e : now));
    paintNow(cur, nx);
    st.ui.guide.replaceChildren(el('h4', { text: 'Programación · ' + st.chan.n }), ...p.map(x => el('div', { class: 'g' }, el('b', { text: x.t || '—' }), el('small', { text: `${clock(x.s)} – ${clock(x.e)}` }), x.d ? el('div', { class: 'mut', style: 'font-size:.85rem', text: x.d }) : null)));
    if (!p.length) st.ui.guide.append(el('p', { class: 'mut', text: 'Este canal no tiene guía.' }));
  } catch { /* sin guía */ }
}
function toggleGuide() { if (st && st.ui.guide) st.ui.guide.hidden = !st.ui.guide.hidden; }

function zap(d) {
  if (!st || st.kind !== 'live' || st.list.length < 2) return;
  const i = st.list.findIndex(c => c.id === st.chan.id);
  st.chan = st.list[(i + d + st.list.length) % st.list.length];
  st.retries = 0;
  try { localStorage.setItem('lumina.lastlive', String(st.chan.id)); } catch { /* nada */ }
  tune(false);
}

function destroyMp() {
  if (st) { clearInterval(st.fl); clearTimeout(st.preT); if (st.ui && st.ui.v) st.ui.v.onwaiting = null; }
  if (st && st.ui && st.ui.v && st.ui.v.getAttribute('src')) { const v = st.ui.v; v.onerror = null; v.removeAttribute('src'); try { v.load(); } catch { /* nada */ } }
  if (st && st.mp) { try { st.mp.pause(); st.mp.unload(); st.mp.detachMediaElement(); st.mp.destroy(); } catch { /* nada */ } st.mp = null; }
}

// vigilante: si la imagen se congela 10 s sin error explícito, se reconecta
function liveWatchdog(tok) {
  clearInterval(st.wd); let lt = -1, lm = Date.now();
  st.wd = setInterval(() => {
    if (!st || st.kind !== 'live' || tok !== st.tok) return;
    const v = st.ui.v, t = v.currentTime;
    if (t !== lt || v.paused || !st.ui.msg.hidden) { if (t !== lt && !st.okAt) st.okAt = Date.now(); if (t !== lt && st.okAt && Date.now() - st.okAt > 20000) st.retries = 0; if (t !== lt && t > 3 && !st.saved) { st.saved = true; try { localStorage.setItem('lumina.livemode', String(st.clean || 0)); } catch { /* nada */ } } lt = t; lm = Date.now(); return; }
    if (Date.now() - lm > 10000) { lm = Date.now(); st.okAt = 0; onLiveErr(tok); }
  }, 2000);
}

async function tune(force, attempt = 0) {
  if (!st) return;
  const c = st.chan, tok = ++st.tok;
  destroyMp(); hideMsg(); st.ui.spin.hidden = false; updateInfo();
  try { await post('/api/live/tune', { id: c.id, force }); }
  catch (e) {
    if (!st || tok !== st.tok) return;
    if (e.status === 409 && e.data) {
      // puede ser nuestra propia conexión recién cerrada: reintento breve antes de molestar al usuario
      if (!force && attempt < 3 && e.data.playing.every(p => p.id === lastLiveId)) { await sleep(500); return tune(false, attempt + 1); }
      return busy(e.data);
    }
    return showMsg(e.message, [{ label: 'Reintentar', pri: true, fn: () => tune(false) }, { label: 'Cerrar', fn: close }]);
  }
  if (!st || tok !== st.tok) return;
  if (st.clean === 2) {
    // MP4 fragmentado en un <video>: se deja acumular un colchón antes de reproducir y, si se vacía, se vuelve a llenar (sin esto
    // se corta en cada fragmento, porque el directo siempre va justo al borde de lo recibido)
    const v = st.ui.v; v.onerror = () => onLiveErr(tok); lastLiveId = c.id;
    v.src = location.origin + '/live/' + c.id + '.ts?clean=2'; v.load();
    const ahead = () => { const b = v.buffered; return b.length ? b.end(b.length - 1) - v.currentTime : 0; };
    const fill = (want, maxMs, then) => {
      clearInterval(st.fl); const t0 = Date.now(); st.ui.spin.hidden = false;
      const id = setInterval(() => {
        if (!st || tok !== st.tok) return clearInterval(id);
        if (ahead() >= want || Date.now() - t0 > maxMs) { clearInterval(id); st.ui.spin.hidden = true; then(); }
      }, 250);
      st.fl = id;
    };
    const go = () => { const pr2 = v.play(); if (pr2) pr2.catch(e => { if (e.name === 'NotAllowedError') showMsg('Pulsa para reproducir', [{ label: 'Reproducir', pri: true, fn: () => v.play() }]); }); };
    // el colchón inicial se adapta: si se corta, el siguiente arranque espera más; con emisiones estables vuelve a bajar
    const pre = Math.min(6, Math.max(2.5, Number(localStorage.getItem('lumina.pre')) || 2.5));
    v.onwaiting = () => {
      if (!st || tok !== st.tok || v.paused || ahead() > 0.3) return;
      try { localStorage.setItem('lumina.pre', String(Math.min(6, pre + 1))); } catch { /* nada */ }
      v.pause(); fill(Math.max(5, pre + 1), 20000, go);
    };
    const okT = setTimeout(() => { try { if (st && tok === st.tok && pre > 2.5) localStorage.setItem('lumina.pre', String(Math.max(2.5, pre - 0.5))); } catch { /* nada */ } }, 120000);
    st.preT = okT;
    liveWatchdog(tok);
    fill(pre, 9000 + pre * 1000, go); // con el colchón del servidor llega casi al instante; si se vacía, se vuelve a llenar
    return;
  }
  let mp;
  try { mp = await loadMpegts(); } catch (e) { return showMsg(e.message, [{ label: 'Cerrar', fn: close }]); }
  if (!st || tok !== st.tok) return;
  if (!mp.isSupported()) return showMsg('Este navegador no puede reproducir TV en directo (necesita Media Source Extensions).', [{ label: 'Cerrar', pri: true, fn: close }]);
  const p = mp.createPlayer({ type: 'mpegts', isLive: true, url: location.origin + '/live/' + c.id + '.ts' + (st.clean === 1 ? '?clean=1' : '') },
    // algo más de colchón (menos tirones con listas IPTV irregulares) y limpieza de memoria periódica
    { enableWorker: true, liveBufferLatencyChasing: true, liveBufferLatencyMaxLatency: 8, liveBufferLatencyMinRemain: 1.5, liveBufferLatencyChasingOnPaused: false,
      lazyLoad: false, autoCleanupSourceBuffer: true, autoCleanupMaxBackwardDuration: 20, autoCleanupMinBackwardDuration: 10 });
  st.mp = p; lastLiveId = c.id;
  p.on(mp.Events.ERROR, (t, d, i) => { const m = [t, d, i && (i.msg || i.code)].filter(Boolean).join(' / '); console.warn('[Lumina] error de reproducción:', m); if (st && tok === st.tok) st.lastErr = m; onLiveErr(tok); });
  // audio que este navegador no decodifica (AC3/E-AC3 en Chrome de escritorio): se pasa enseguida al modo limpio, que lo convierte a AAC
  p.on(mp.Events.MEDIA_INFO, mi => {
    const ac = (mi && mi.audioCodec) || '';
    if (st && tok === st.tok && !st.clean && ac && window.MediaSource && !MediaSource.isTypeSupported(`audio/mp4; codecs="${ac}"`)) {
      st.lastErr = 'audio ' + ac + ' no compatible con este navegador'; onLiveErr(tok);
    }
  });
  p.attachMediaElement(st.ui.v); p.load();
  liveWatchdog(tok);
  const pr = st.ui.v.play();
  if (pr) pr.catch(e => { if (e.name === 'NotAllowedError') showMsg('Pulsa para reproducir', [{ label: 'Reproducir', pri: true, fn: () => st.ui.v.play() }]); });
}

function busy(d) {
  const who = d.playing.map(p => `«${p.n}»${p.v > 1 ? ` (${p.v} personas)` : ''}`).join(', ');
  const n = d.max;
  showMsg(`Tu proveedor permite ${n} ${n === 1 ? 'canal distinto' : 'canales distintos'} a la vez y ahora se está viendo ${who}. Si cambias a «${st.chan.n}», esa emisión se cortará.`, [
    { label: 'Cambiar de canal', pri: true, fn: () => tune(true) }, { label: 'Cancelar', fn: close }]);
}

function onLiveErr(tok) {
  if (!st || st.kind !== 'live' || tok !== st.tok) return;
  const v = st.ui.v;
  report(`directo «${st.chan.n}» (modo ${st.clean || 0}, intento ${st.retries + 1}): ${st.lastErr || (v.error ? 'error del vídeo ' + verr(v) : 'no llega imagen')}; estado ${v.readyState}, t=${(v.currentTime || 0).toFixed(1)}`);
  if (st.retries < 3) { st.retries++; st.clean = Math.max(st.clean || 0, st.retries >= 2 ? 2 : 1); const t = tok; setTimeout(() => { if (st && st.tok === t) tune(false); }, 1500); return; }
  destroyMp();
  showMsg('No se puede reproducir este canal ahora. Puede que el proveedor no responda, que el canal esté caído o que otra persona haya cambiado de canal.' + (st.lastErr ? ' (' + st.lastErr + ')' : ''),
    [{ label: 'Reintentar', pri: true, fn: () => { st.retries = 0; tune(false); } }, { label: 'Cerrar', fn: close }]);
}

window.Player = { vod, live, close, closeSilent: () => teardown(false), isOpen: () => !!st };
})();
