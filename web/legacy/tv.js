(() => {
  "use strict";
  const ua = navigator.userAgent;
  window.LuminaBack = () => {
    const pl = document.querySelector("#player"), md = document.querySelector("#modal");
    if (pl && !pl.hidden || md && !md.hidden) {
      document.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", keyCode: 27, bubbles: true }));
      return true;
    }
    const h = location.hash;
    if (h && h !== "#/" && h !== "#") {
      history.length > 1 ? history.back() : location.hash = "#/";
      return true;
    }
    return false;
  };
  const qs = new URLSearchParams(location.search);
  if (qs.get("tv") === "0") localStorage.removeItem("tv");
  if (qs.get("tv") === "1") localStorage.setItem("tv", "1");
  const isTV = localStorage.getItem("tv") === "1" || /Web0S|webOS|SMART-TV|SmartTV|AFT[A-Z]|Android TV|GoogleTV|LumTV/i.test(ua);
  if (!isTV) return;
  document.documentElement.classList.add("tv");
  const SEL = 'a[href],button:not([disabled]),input:not([type=hidden]):not([disabled]),select,textarea,[tabindex="0"]';
  const $ = (s) => document.querySelector(s);
  const visible = (e) => {
    const r = e.getBoundingClientRect();
    return r.width > 0 && r.height > 0 && getComputedStyle(e).visibility !== "hidden";
  };
  const playerOpen = () => {
    const p = $("#player");
    return p && !p.hidden;
  };
  const modalOpen = () => {
    const m = $("#modal");
    return m && !m.hidden;
  };
  const scope = () => modalOpen() ? $("#modal") : playerOpen() ? $("#player") : document;
  function cands() {
    return [...scope().querySelectorAll(SEL)].filter(visible);
  }
  function move(dir) {
    const list = cands();
    let cur = document.activeElement;
    if (!cur || cur === document.body || !list.includes(cur)) {
      const f = firstFocus(list);
      if (f) f.focus();
      return true;
    }
    const a = cur.getBoundingClientRect();
    const ac = { x: a.left + a.width / 2, y: a.top + a.height / 2 };
    let best = null, bs = Infinity;
    for (const e of list) {
      if (e === cur) continue;
      const b = e.getBoundingClientRect();
      const bc = { x: b.left + b.width / 2, y: b.top + b.height / 2 };
      let along, perp, ok;
      if (dir === "ArrowRight") {
        along = b.left - a.right;
        ok = bc.x > ac.x + 2;
        perp = Math.abs(bc.y - ac.y);
        if (along < 0) along = 0;
      } else if (dir === "ArrowLeft") {
        along = a.left - b.right;
        ok = bc.x < ac.x - 2;
        perp = Math.abs(bc.y - ac.y);
        if (along < 0) along = 0;
      } else if (dir === "ArrowDown") {
        along = b.top - a.bottom;
        ok = bc.y > ac.y + 2;
        perp = Math.abs(bc.x - ac.x);
        if (along < 0) along = 0;
      } else {
        along = a.top - b.bottom;
        ok = bc.y < ac.y - 2;
        perp = Math.abs(bc.x - ac.x);
        if (along < 0) along = 0;
      }
      if (!ok) continue;
      const s = along + perp * 2.2;
      if (s < bs) {
        bs = s;
        best = e;
      }
    }
    if (best) {
      best.focus({ preventScroll: true });
      reveal(best);
      return true;
    }
    if (dir === "ArrowDown" && scope() === document) {
      window.scrollBy(0, innerHeight * 0.6);
      return true;
    }
    return dir === "ArrowLeft" || dir === "ArrowRight" ? true : false;
  }
  function reveal(e) {
    const r = e.getBoundingClientRect();
    const m = 120;
    if (r.top < 90 || r.bottom > innerHeight - m) window.scrollBy({ top: r.top - innerHeight * 0.35 });
    const row = e.closest(".rail,.track,.scroller,.cards");
    if (row && row.scrollWidth > row.clientWidth) {
      const rr = row.getBoundingClientRect();
      if (r.left < rr.left + 40) row.scrollLeft -= rr.left + 40 - r.left;
      else if (r.right > rr.right - 40) row.scrollLeft += r.right - rr.right + 40;
    }
  }
  function firstFocus(list) {
    const root = scope();
    return root.querySelector(".hero .btn.pri") || root.querySelector(".card,.ch,.ep") || root.querySelector(".btn.pri") || list[0];
  }
  function focusFirst() {
    if (playerOpen() || modalOpen()) return;
    const a = document.activeElement;
    if (a && a !== document.body && visible(a)) return;
    const f = firstFocus(cands());
    if (f) f.focus({ preventScroll: true });
  }
  let t;
  new MutationObserver(() => {
    clearTimeout(t);
    t = setTimeout(focusFirst, 160);
  }).observe($("#view"), { childList: true });
  addEventListener("hashchange", () => {
    document.activeElement && document.activeElement.blur();
  });
  const KEYS = { 461: "back", 10009: "back", 8: "back", 27: "back", 405: "yellow", 406: "blue", 403: "red", 404: "green", 179: "pp", 415: "play", 19: "pause", 413: "stop", 412: "rew", 417: "ff", 85: "pp" };
  addEventListener("keydown", (e) => {
    const typing = e.target.tagName === "INPUT" && e.target.type !== "range" || e.target.tagName === "TEXTAREA";
    const k = KEYS[e.keyCode] || (e.key === "GoBack" || e.key === "BrowserBack" ? "back" : "");
    if (k === "yellow") {
      e.preventDefault();
      try {
        parent.postMessage({ lumina: "setup" }, "*");
      } catch (e2) {
      }
      return;
    }
    if (k === "back") {
      if (typing && e.keyCode === 8) return;
      if (modalOpen() || playerOpen()) return;
      if (e.keyCode === 27 || e.key === "Escape") return;
      const h = location.hash;
      if (h && h !== "#/" && h !== "#") {
        e.preventDefault();
        history.length > 1 ? history.back() : location.hash = "#/";
      }
      return;
    }
    if (typing) {
      if (e.key === "ArrowDown" || e.key === "ArrowUp") {
        e.target.blur();
      } else return;
    }
    if (playerOpen() && !modalOpen()) {
      if (!(e.target.closest && e.target.closest(".pmsg,.tmenu"))) return;
    }
    if (e.key === "ArrowUp" || e.key === "ArrowDown" || e.key === "ArrowLeft" || e.key === "ArrowRight") {
      if (e.target.type === "range" && (e.key === "ArrowLeft" || e.key === "ArrowRight")) return;
      if (move(e.key)) e.preventDefault();
    } else if (e.key === "Enter" && e.target.getAttribute && e.target.getAttribute("role") === "button") {
      e.preventDefault();
      e.target.click();
    }
  }, true);
  const st = document.createElement("style");
  st.textContent = ".tv,.tv *{cursor:none!important}";
  document.head.appendChild(st);
})();
