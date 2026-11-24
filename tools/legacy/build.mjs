// Genera web/legacy/* a partir de web/*: JS y CSS rebajados para Chromium 53+ (LG webOS 4/5/6).
// Uso: node tools/legacy/build.mjs <ruta-a-esbuild>
import { execFileSync } from 'node:child_process';
import fs from 'node:fs';
const esb = process.argv[2] || 'esbuild';
const W = 'web/', L = 'web/legacy/';
fs.mkdirSync(L, { recursive: true });
const tr = (src, out, extra = []) => execFileSync(esb, [src, '--target=chrome53', '--outfile=' + out, '--log-level=warning', ...extra], { stdio: 'inherit' });

// JS
for (const f of ['app.js', 'player.js', 'tv.js']) tr(W + f, L + f);
fs.copyFileSync('tools/legacy/polyfills.js', L + 'polyfills.js');
fs.copyFileSync(W + 'vendor/mpegts.min.js', L + 'mpegts.min.js');

// CSS
tr(W + 'app.css', L + 'app.tmp.css');
let css = fs.readFileSync(L + 'app.tmp.css', 'utf8'); fs.unlinkSync(L + 'app.tmp.css');
const VW = 1536; // TV 1920 px con zoom 1,25
// clamp()/min()/max(): se evalúan para un televisor (1536x864 css)
const ev = (s) => {
  s = s.trim(); let m;
  if ((m = s.match(/^(-?[\d.]+)px$/))) return +m[1];
  if ((m = s.match(/^(-?[\d.]+)vw$/))) return +m[1] * VW / 100;
  if ((m = s.match(/^(-?[\d.]+)vh$/))) return +m[1] * 864 / 100;
  if ((m = s.match(/^(-?[\d.]+)(rem|em)$/))) return +m[1] * 16;
  const fn = s.match(/^(clamp|min|max)\((.*)\)$/s);
  if (fn) {
    const parts = []; let d = 0, cur = '';
    for (const ch of fn[2]) { if (ch === '(') d++; if (ch === ')') d--; if (ch === ',' && d === 0) { parts.push(cur); cur = ''; } else cur += ch; }
    parts.push(cur); const v = parts.map(ev); if (v.some(x => x == null)) return null;
    return fn[1] === 'clamp' ? Math.max(v[0], Math.min(v[1], v[2])) : fn[1] === 'min' ? Math.min(...v) : Math.max(...v);
  }
  return null;
};
const pending = [];
for (let i = 0; i < 4; i++) css = css.replace(/(?<![\w-])(clamp|min|max)\(([^()]*(?:\([^()]*\)[^()]*)*)\)/g, (all) => { const v = ev(all); if (v == null) { pending.push(all); return all; } return Math.round(v) + 'px'; });
css = css.replace(/(max-)?width:\s*min\((\d+)px,\s*100%\)/g, (a, mx, n) => `${mx || ''}width:${n}px;max-width:100%`);
if (pending.length) console.warn('CSS sin resolver:', [...new Set(pending)]);
css = css.replace(/(^|[;{\s])inset:\s*([^;}]+)/g, (all, pre, v) => { // inset (Chrome 87+)
  const a = v.trim().split(/\s+/), [t, r = t, b = t, l = r] = a;
  return `${pre}top:${t};right:${r};bottom:${b};left:${l}`;
});
css = css.replace(/:focus-visible/g, ':focus'); // :focus-visible no existe antes de Chrome 86 y anula toda la regla
// aspect-ratio (Chrome 88+): relleno proporcional
css = css.replace(/aspect-ratio:\s*16\s*\/\s*9/g, 'height:0;padding-top:56.25%');
// gap en flex: márgenes de reserva activados por JS (.ngap)
const fb = [];
css.replace(/([^{}@]+)\{([^{}]*)\}/g, (all, sel, body) => {
  if (!/display:\s*(inline-)?flex/.test(body)) return all;
  const g = body.match(/(?:^|[;\s{])gap:\s*([\d.]+)px(?:\s+([\d.]+)px)?/); if (!g) return all;
  const col = /flex-direction:\s*column/.test(body);
  const val = col || !g[2] ? g[1] : g[2];
  const sels = sel.split(',').map(s => s.trim()).filter(Boolean).filter(s => !s.includes('@'));
  fb.push(sels.map(s => `.ngap ${s}>*`).join(',') + `{margin-${col ? 'bottom' : 'right'}:${val}px}`);
  return all;
});
css += '\n/* gap en flex (reserva) */\n' + fb.join('\n') + '\n';
fs.writeFileSync(L + 'app.css', css);

// HTML
let h = fs.readFileSync(W + 'index.html', 'utf8');
h = h.replace('<script src="/app.js"></script>', '<script src="/polyfills.js"></script>\n<script src="/app.js"></script>');
fs.writeFileSync(L + 'index.html', h);
console.log('legacy listo:', fs.readdirSync(L).join(', '));
