/* veduta viewer — 完全オフラインで動作する最小ビューア。
 * 禁止事項 (CI で grep 検証): DOM 文字列注入 API・動的コード実行・location への代入。
 * DOM 構築は createElement + textContent のみ。 */
(function () {
  "use strict";

  var svg = document.querySelector(".vd-svg");
  if (!svg) { return; }

  /* ---- 埋め込み IR データ (application/json はスクリプトとして実行されない) ---- */
  var data = null;
  try {
    var dataEl = document.getElementById("vd-data");
    if (dataEl) { data = JSON.parse(dataEl.textContent); }
  } catch (e) { data = null; }

  var knownIds = Object.create(null);
  if (data && Array.isArray(data.components)) {
    for (var i = 0; i < data.components.length; i++) {
      var c = data.components[i];
      if (c && typeof c.id === "string") { knownIds[c.id] = true; }
    }
  }

  /* ---- pan / zoom: viewBox を直接操作する ---- */
  var vb0 = (svg.getAttribute("viewBox") || "0 0 100 100").split(/\s+/).map(Number);
  var vb = vb0.slice();

  function applyViewBox() {
    svg.setAttribute("viewBox", vb.join(" "));
  }

  var stage = document.querySelector(".vd-stage") || svg;
  var panning = false;
  var lastX = 0, lastY = 0;

  function unitsPerPixel() {
    var r = svg.getBoundingClientRect();
    if (r.width <= 0) { return 1; }
    return vb[2] / r.width;
  }

  stage.addEventListener("pointerdown", function (ev) {
    panning = true;
    lastX = ev.clientX;
    lastY = ev.clientY;
    stage.classList.add("vd-panning");
    if (stage.setPointerCapture) { stage.setPointerCapture(ev.pointerId); }
  });
  stage.addEventListener("pointermove", function (ev) {
    if (!panning) { return; }
    var k = unitsPerPixel();
    vb[0] -= (ev.clientX - lastX) * k;
    vb[1] -= (ev.clientY - lastY) * k;
    lastX = ev.clientX;
    lastY = ev.clientY;
    applyViewBox();
  });
  function endPan() {
    panning = false;
    stage.classList.remove("vd-panning");
  }
  stage.addEventListener("pointerup", endPan);
  stage.addEventListener("pointercancel", endPan);

  stage.addEventListener("wheel", function (ev) {
    ev.preventDefault();
    var r = svg.getBoundingClientRect();
    var px = (ev.clientX - r.left) / r.width;
    var py = (ev.clientY - r.top) / r.height;
    var factor = ev.deltaY > 0 ? 1.12 : 1 / 1.12;
    var nw = vb[2] * factor;
    var nh = vb[3] * factor;
    if (nw < vb0[2] / 40 || nw > vb0[2] * 40) { return; }
    vb[0] += (vb[2] - nw) * px;
    vb[1] += (vb[3] - nh) * py;
    vb[2] = nw;
    vb[3] = nh;
    applyViewBox();
  }, { passive: false });

  var resetBtn = document.getElementById("vd-reset");
  if (resetBtn) {
    resetBtn.addEventListener("click", function () {
      vb = vb0.slice();
      applyViewBox();
    });
  }

  /* ---- テーマ切替 ---- */
  var themeBtn = document.getElementById("vd-theme");
  function currentTheme() {
    return document.documentElement.getAttribute("data-theme") || "";
  }
  function setTheme(t) {
    if (t === "dark" || t === "light") {
      document.documentElement.setAttribute("data-theme", t);
    } else {
      document.documentElement.removeAttribute("data-theme");
    }
    try { localStorage.setItem("vd-theme", t); } catch (e) { /* 保存不可でも動作継続 */ }
  }
  if (themeBtn) {
    themeBtn.addEventListener("click", function () {
      var cur = currentTheme();
      var prefersDark = false;
      try { prefersDark = matchMedia("(prefers-color-scheme: dark)").matches; } catch (e) { /* noop */ }
      var effectiveDark = cur === "dark" || (cur === "" && prefersDark);
      setTheme(effectiveDark ? "light" : "dark");
    });
  }
  try {
    var saved = localStorage.getItem("vd-theme");
    if (saved === "dark" || saved === "light") { setTheme(saved); }
  } catch (e) { /* noop */ }

  /* ---- ノード検索 ---- */
  var nodes = document.querySelectorAll(".vd-node");
  function clearMarks() {
    for (var i = 0; i < nodes.length; i++) {
      nodes[i].classList.remove("vd-dim");
      nodes[i].classList.remove("vd-hit");
    }
  }
  var searchEl = document.getElementById("vd-search");
  if (searchEl) {
    searchEl.addEventListener("input", function () {
      var q = searchEl.value.trim().toLowerCase();
      if (q === "") { clearMarks(); return; }
      for (var i = 0; i < nodes.length; i++) {
        var el = nodes[i];
        var label = (el.getAttribute("data-label") || "").toLowerCase();
        var id = (el.id || "").replace(/^vd-node-/, "").toLowerCase();
        if (label.indexOf(q) >= 0 || id.indexOf(q) >= 0) {
          el.classList.add("vd-hit");
          el.classList.remove("vd-dim");
        } else {
          el.classList.add("vd-dim");
          el.classList.remove("vd-hit");
        }
      }
    });
  }

  /* ---- ディープリンク (#focus=node-id) ----
   * location.hash は攻撃者が制御できる。許可文字にマッチさせ、
   * 既知のノード ID と照合してから使う。マッチしなければ黙って無視する。 */
  function applyHashFocus() {
    var h = location.hash || "";
    var m = /^#focus=([A-Za-z0-9][A-Za-z0-9_-]{0,63})$/.exec(h);
    if (!m) { return; }
    var id = m[1];
    if (!knownIds[id]) { return; }
    var el = document.getElementById("vd-node-" + id);
    if (!el) { return; }
    clearMarks();
    for (var i = 0; i < nodes.length; i++) {
      if (nodes[i] !== el) { nodes[i].classList.add("vd-dim"); }
    }
    el.classList.add("vd-hit");
  }
  window.addEventListener("hashchange", applyHashFocus);
  applyHashFocus();
})();
