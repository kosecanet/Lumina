/* Compatibilidad con Chromium antiguo (LG webOS 4–6 / Chrome 53–79). */
(function () {
  var A = Array.prototype, E = Element.prototype, N = Node.prototype;
  if (!A.flat) { A.flat = function (d) { d = d === undefined ? 1 : d; return d < 1 ? A.slice.call(this) : A.reduce.call(this, function (a, v) { return a.concat(Array.isArray(v) && d > 0 ? v.flat(d - 1) : v); }, []); }; }
  if (!A.flatMap) { A.flatMap = function (f, t) { return this.map(f, t).flat(); }; }
  if (!Object.fromEntries) { Object.fromEntries = function (it) { var o = {}; Array.from(it).forEach(function (e) { o[e[0]] = e[1]; }); return o; }; }
  if (!String.prototype.replaceAll) { String.prototype.replaceAll = function (a, b) { return a instanceof RegExp ? this.replace(a, b) : this.split(a).join(b); }; }
  if (!A.at) { A.at = function (i) { i = Math.trunc(i) || 0; if (i < 0) i += this.length; return this[i]; }; }
  if (!E.replaceChildren) { E.replaceChildren = function () { while (this.firstChild) this.removeChild(this.firstChild); this.append.apply(this, arguments); }; }
  if (!E.append) { E.append = function () { for (var i = 0; i < arguments.length; i++) { var n = arguments[i]; this.appendChild(n && n.nodeType ? n : document.createTextNode(String(n))); } }; }
  if (!window.AbortController) {
    window.AbortController = function () { var s = this.signal = { aborted: false, _l: [], addEventListener: function (_, f) { s._l.push(f); } }; this.abort = function () { s.aborted = true; s._l.forEach(function (f) { f(); }); }; };
  }
  if (!window.IntersectionObserver) { // sin IO: se muestra todo de golpe
    window.IntersectionObserver = function (cb) { this.observe = function (t) { setTimeout(function () { cb([{ target: t, isIntersecting: true }], this); }.bind(this), 0); }; this.unobserve = this.disconnect = function () {}; };
  }
  // ¿soporta `gap` en flexbox? (Chrome 84+). Si no, el CSS legacy aplica márgenes (.ngap)
  try {
    var d = document.createElement('div'); d.style.cssText = 'display:flex;flex-direction:column;gap:1px;position:absolute;visibility:hidden';
    d.appendChild(document.createElement('i')); d.appendChild(document.createElement('i'));
    d.firstChild.style.height = d.lastChild.style.height = '1px';
    document.documentElement.appendChild(d);
    var ok = d.scrollHeight === 3; document.documentElement.removeChild(d);
    if (!ok) document.documentElement.className += ' ngap';
  } catch (e) { document.documentElement.className += ' ngap'; }
})();
