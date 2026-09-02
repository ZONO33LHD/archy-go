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

  /* nodeEdges[nodeId] = [edgeId...] : 検索時に接続辺を強調するための対応表 */
  var nodeEdges = Object.create(null);
  if (data && Array.isArray(data.connections)) {
    for (var j = 0; j < data.connections.length; j++) {
      var cn = data.connections[j];
      if (!cn || typeof cn.id !== "string") { continue; }
      (nodeEdges[cn.from] = nodeEdges[cn.from] || []).push(cn.id);
      (nodeEdges[cn.to] = nodeEdges[cn.to] || []).push(cn.id);
    }
  }

  /* ---- pan / zoom: viewBox を直接操作する ---- */
  var vb0 = (svg.getAttribute("viewBox") || "0 0 100 100").split(/\s+/).map(Number);
  var vb = vb0.slice();
  function applyViewBox() { svg.setAttribute("viewBox", vb.join(" ")); }

  var stage = document.querySelector(".vd-stage") || svg;

  function unitsPerPixel() {
    var r = svg.getBoundingClientRect();
    return r.width > 0 ? vb[2] / r.width : 1;
  }
  function pointerRatio(clientX, clientY) {
    var r = svg.getBoundingClientRect();
    return {
      x: r.width > 0 ? (clientX - r.left) / r.width : 0.5,
      y: r.height > 0 ? (clientY - r.top) / r.height : 0.5
    };
  }
  function zoomBy(factor, px, py) {
    var nw = vb[2] * factor, nh = vb[3] * factor;
    if (nw < vb0[2] / 50 || nw > vb0[2] * 50) { return; }
    vb[0] += (vb[2] - nw) * px;
    vb[1] += (vb[3] - nh) * py;
    vb[2] = nw; vb[3] = nh;
    applyViewBox();
  }
  function resetView() { vb = vb0.slice(); applyViewBox(); }

  /* --- ポインタ: 1本なら pan、2本なら pinch zoom --- */
  var pointers = Object.create(null);
  var pointerCount = 0, lastX = 0, lastY = 0, pinchDist = 0;

  stage.addEventListener("pointerdown", function (ev) {
    pointers[ev.pointerId] = { x: ev.clientX, y: ev.clientY };
    pointerCount++;
    if (pointerCount === 1) { lastX = ev.clientX; lastY = ev.clientY; stage.classList.add("vd-panning"); }
    if (stage.setPointerCapture) { try { stage.setPointerCapture(ev.pointerId); } catch (e) { /* noop */ } }
  });
  stage.addEventListener("pointermove", function (ev) {
    if (!pointers[ev.pointerId]) { return; }
    pointers[ev.pointerId] = { x: ev.clientX, y: ev.clientY };
    var ids = Object.keys(pointers);
    if (ids.length >= 2) {
      var a = pointers[ids[0]], b = pointers[ids[1]];
      var dist = Math.hypot(a.x - b.x, a.y - b.y);
      if (pinchDist > 0 && dist > 0) {
        var mid = pointerRatio((a.x + b.x) / 2, (a.y + b.y) / 2);
        zoomBy(pinchDist / dist, mid.x, mid.y);
      }
      pinchDist = dist;
      return;
    }
    var k = unitsPerPixel();
    vb[0] -= (ev.clientX - lastX) * k;
    vb[1] -= (ev.clientY - lastY) * k;
    lastX = ev.clientX; lastY = ev.clientY;
    applyViewBox();
  });
  function endPointer(ev) {
    if (pointers[ev.pointerId]) { delete pointers[ev.pointerId]; pointerCount = Math.max(0, pointerCount - 1); }
    if (pointerCount < 2) { pinchDist = 0; }
    if (pointerCount === 0) { stage.classList.remove("vd-panning"); }
  }
  stage.addEventListener("pointerup", endPointer);
  stage.addEventListener("pointercancel", endPointer);

  /* wheel: 修飾キー (Ctrl/Cmd) 付きのみ zoom。通常 wheel はページスクロールに委ねる。 */
  stage.addEventListener("wheel", function (ev) {
    if (!ev.ctrlKey && !ev.metaKey) { return; }
    ev.preventDefault();
    var p = pointerRatio(ev.clientX, ev.clientY);
    zoomBy(ev.deltaY > 0 ? 1.12 : 1 / 1.12, p.x, p.y);
  }, { passive: false });

  /* キーボード: 矢印で pan、+/- で zoom、0/Home でリセット */
  stage.addEventListener("keydown", function (ev) {
    var step = vb[2] * 0.08;
    switch (ev.key) {
      case "ArrowLeft": vb[0] -= step; break;
      case "ArrowRight": vb[0] += step; break;
      case "ArrowUp": vb[1] -= step; break;
      case "ArrowDown": vb[1] += step; break;
      case "+": case "=": zoomBy(1 / 1.2, 0.5, 0.5); ev.preventDefault(); return;
      case "-": case "_": zoomBy(1.2, 0.5, 0.5); ev.preventDefault(); return;
      case "0": case "Home": resetView(); ev.preventDefault(); return;
      default: return;
    }
    applyViewBox();
    ev.preventDefault();
  });

  function bindClick(id, fn) {
    var el = document.getElementById(id);
    if (el) { el.addEventListener("click", fn); }
  }
  bindClick("vd-reset", resetView);
  bindClick("vd-zoom-in", function () { zoomBy(1 / 1.2, 0.5, 0.5); });
  bindClick("vd-zoom-out", function () { zoomBy(1.2, 0.5, 0.5); });

  /* ---- テーマ切替 ---- */
  var themeBtn = document.getElementById("vd-theme");
  function currentTheme() { return document.documentElement.getAttribute("data-theme") || ""; }
  function effectiveDark() {
    var cur = currentTheme();
    if (cur === "dark") { return true; }
    if (cur === "light") { return false; }
    try { return matchMedia("(prefers-color-scheme: dark)").matches; } catch (e) { return false; }
  }
  function syncThemePressed() {
    if (themeBtn) { themeBtn.setAttribute("aria-pressed", effectiveDark() ? "true" : "false"); }
  }
  function setTheme(t) {
    if (t === "dark" || t === "light") { document.documentElement.setAttribute("data-theme", t); }
    else { document.documentElement.removeAttribute("data-theme"); }
    syncThemePressed();
    try { localStorage.setItem("vd-theme", t); } catch (e) { /* 保存不可でも継続 */ }
  }
  if (themeBtn) {
    themeBtn.addEventListener("click", function () { setTheme(effectiveDark() ? "light" : "dark"); });
  }
  try {
    var saved = localStorage.getItem("vd-theme");
    if (saved === "dark" || saved === "light") { setTheme(saved); }
  } catch (e) { /* noop */ }
  syncThemePressed();

  /* ---- ノード検索 (接続辺・関連境界も強調) ---- */
  var nodes = document.querySelectorAll(".vd-node");
  var edgeGroups = document.querySelectorAll(".vd-edge-group");
  var boundaries = document.querySelectorAll(".vd-boundary");
  var status = document.getElementById("vd-search-status");

  function nodeId(el) { return (el.id || "").replace(/^vd-node-/, ""); }
  function edgeId(el) { return (el.id || "").replace(/^vd-edge-/, ""); }
  function unmark(el) { el.classList.remove("vd-dim"); el.classList.remove("vd-hit"); }

  function clearMarks() {
    var k;
    for (k = 0; k < nodes.length; k++) { unmark(nodes[k]); }
    for (k = 0; k < edgeGroups.length; k++) { unmark(edgeGroups[k]); }
    for (k = 0; k < boundaries.length; k++) { unmark(boundaries[k]); }
    if (status) { status.textContent = ""; }
  }

  function runSearch(q) {
    q = q.trim().toLowerCase();
    if (q === "") { clearMarks(); return; }
    var hitNodes = Object.create(null), hitCount = 0, i;
    for (i = 0; i < nodes.length; i++) {
      var el = nodes[i];
      var label = (el.getAttribute("data-label") || "").toLowerCase();
      var id = nodeId(el).toLowerCase();
      if (label.indexOf(q) >= 0 || id.indexOf(q) >= 0) {
        el.classList.add("vd-hit"); el.classList.remove("vd-dim");
        hitNodes[nodeId(el)] = true; hitCount++;
      } else {
        el.classList.add("vd-dim"); el.classList.remove("vd-hit");
      }
    }
    var hitEdges = Object.create(null), id2;
    for (id2 in hitNodes) {
      var es = nodeEdges[id2];
      if (es) { for (var m = 0; m < es.length; m++) { hitEdges[es[m]] = true; } }
    }
    for (i = 0; i < edgeGroups.length; i++) {
      var eg = edgeGroups[i];
      if (hitEdges[edgeId(eg)]) { eg.classList.add("vd-hit"); eg.classList.remove("vd-dim"); }
      else { eg.classList.add("vd-dim"); eg.classList.remove("vd-hit"); }
    }
    for (i = 0; i < boundaries.length; i++) { boundaries[i].classList.add("vd-dim"); boundaries[i].classList.remove("vd-hit"); }
    if (status) { status.textContent = hitCount + " / " + nodes.length; }
  }

  var searchEl = document.getElementById("vd-search");
  if (searchEl) { searchEl.addEventListener("input", function () { runSearch(searchEl.value); }); }

  /* ---- ディープリンク (#focus=node-id) ----
   * location.hash は攻撃者が制御できる。許可文字にマッチさせ、
   * 既知のノード ID と照合してから使う。マッチしなければ黙って無視する。 */
  function applyHashFocus() {
    var mt = /^#focus=([A-Za-z0-9][A-Za-z0-9_-]{0,63})$/.exec(location.hash || "");
    if (!mt || !knownIds[mt[1]]) { return; }
    if (searchEl) { searchEl.value = mt[1]; }
    runSearch(mt[1]);
  }
  window.addEventListener("hashchange", applyHashFocus);
  applyHashFocus();
})();
