// Preview page: renders a viewer per file kind, handles per-file passwords, and
// (for limited files) shows a download button instead of an inline stream so the
// download count stays predictable.
(function () {
  Filez.initChrome();
  const $ = (id) => document.getElementById(id);

  const el = $("pv-config");
  const cfg = {
    id: el.dataset.id,
    name: el.dataset.name,
    ext: el.dataset.ext,
    mime: el.dataset.mime,
    size: parseInt(el.dataset.size, 10) || 0,
    kind: el.dataset.kind,
    hasPw: el.dataset.haspw === "true",
    limited: el.dataset.limited === "true",
    url: el.dataset.url,
    entries: JSON.parse(el.dataset.entries || "[]"),
  };

  let pw = "";
  $("pv-icon").textContent = Filez.iconFor(cfg.mime, cfg.ext);

  function downloadURL() {
    return cfg.url + (pw ? "?pw=" + encodeURIComponent(pw) : "");
  }
  function fetchHeaders() {
    return pw ? { "X-File-Password": pw } : {};
  }

  // ---- Password gate ----
  function needsPassword() { return cfg.hasPw && !pw; }

  function showPwForm(errMsg) {
    $("pw-form").classList.remove("hidden");
    $("viewer").classList.add("hidden");
    $("dl-top").classList.add("hidden");
    const e = $("pw-error");
    if (errMsg) { e.textContent = errMsg; e.classList.remove("hidden"); }
    else e.classList.add("hidden");
  }
  $("pw-submit").addEventListener("click", () => {
    const v = $("pw-input").value;
    if (!v) return;
    pw = v;
    $("pw-form").classList.add("hidden");
    $("dl-top").classList.remove("hidden");
    render();
  });
  $("pw-input").addEventListener("keydown", (e) => { if (e.key === "Enter") $("pw-submit").click(); });

  function onAuthFail() {
    pw = "";
    showPwForm("Falsches Passwort.");
  }

  // ---- Rendering ----
  function render() {
    if (needsPassword()) { showPwForm(); return; }
    $("dl-top").href = downloadURL();

    const v = $("viewer");
    v.innerHTML = "";
    v.classList.remove("hidden");

    if (cfg.limited) { renderDownload(v, "Diese Datei hat ein Download-Limit — der Klick zählt als ein Download."); return; }

    switch (cfg.kind) {
      case "image": return renderMedia(v, "img");
      case "video": return renderMedia(v, "video");
      case "audio": return renderMedia(v, "audio");
      case "pdf": return renderPdf(v);
      case "text": return renderText(v);
      case "archive": return renderArchive(v);
      default: return renderDownload(v, "");
    }
  }

  function renderMedia(v, tag) {
    const m = document.createElement(tag);
    if (tag !== "img") m.controls = true;
    m.src = downloadURL();
    m.onerror = () => { if (cfg.hasPw) onAuthFail(); else fail(v); };
    v.appendChild(m);
  }

  function renderPdf(v) {
    const f = document.createElement("iframe");
    f.src = downloadURL() + "#view=FitH";
    v.appendChild(f);
  }

  async function renderText(v) {
    if (cfg.size > 5 * 1024 * 1024) {
      renderDownload(v, "Textdatei zu groß für die Vorschau (>5 MB).");
      return;
    }
    v.innerHTML = '<div class="code-loading">Lade…</div>';
    try {
      const r = await fetch(downloadURL(), { headers: fetchHeaders() });
      if (r.status === 401 && cfg.hasPw) return onAuthFail();
      if (!r.ok) throw new Error("HTTP " + r.status);
      const text = await r.text();
      v.innerHTML = "";
      buildCodeView(v, text);
    } catch (e) {
      fail(v);
    }
  }

  function buildCodeView(v, text) {
    const wrap = document.createElement("div");
    wrap.className = "code-view";
    const lines = text.split("\n");
    // Drop a trailing empty line from the final newline so numbering matches.
    if (lines.length > 1 && lines[lines.length - 1] === "") lines.pop();

    const gutter = document.createElement("div");
    gutter.className = "code-gutter";
    gutter.setAttribute("aria-hidden", "true");
    gutter.textContent = lines.map((_, i) => i + 1).join("\n");

    const pre = document.createElement("pre");
    pre.className = "code-body";
    pre.textContent = lines.join("\n");

    // Keep the gutter aligned while scrolling the code body vertically.
    pre.addEventListener("scroll", () => { gutter.scrollTop = pre.scrollTop; });

    wrap.appendChild(gutter);
    wrap.appendChild(pre);
    v.appendChild(wrap);
  }

  function renderArchive(v) {
    if (!cfg.entries.length) { renderDownload(v, "Archiv — Inhalt kann nicht aufgelistet werden."); return; }
    const ul = document.createElement("ul");
    ul.className = "archive-list";
    for (const e of cfg.entries) {
      const li = document.createElement("li");
      const name = document.createElement("span");
      name.textContent = e.Name;
      const size = document.createElement("span");
      size.className = "ae-size";
      size.textContent = Filez.fmtBytes(e.Size);
      li.appendChild(name);
      li.appendChild(size);
      ul.appendChild(li);
    }
    v.appendChild(ul);
  }

  function renderDownload(v, note) {
    const wrap = document.createElement("div");
    wrap.className = "center-actions";
    wrap.innerHTML =
      '<div class="ca-icon">' + Filez.iconFor(cfg.mime, cfg.ext) + "</div>" +
      "<p style='color:var(--fg-muted);margin:8px 0 16px'>" +
        escapeHtml(cfg.name) + " · " + Filez.fmtBytes(cfg.size) + "</p>";
    const a = document.createElement("a");
    a.className = "btn btn-primary";
    a.style.width = "auto";
    a.style.display = "inline-flex";
    a.href = downloadURL();
    a.textContent = "⬇ Herunterladen";
    a.setAttribute("download", cfg.name);
    wrap.appendChild(a);
    if (note) {
      const p = document.createElement("p");
      p.className = "hint";
      p.style.marginTop = "12px";
      p.textContent = note;
      wrap.appendChild(p);
    }
    v.appendChild(wrap);
  }

  function fail(v) {
    v.innerHTML = "";
    renderDownload(v, "Vorschau nicht verfügbar.");
  }

  function escapeHtml(s) {
    return String(s).replace(/[&<>"']/g, (c) =>
      ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
  }

  render();
})();
