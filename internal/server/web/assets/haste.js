// Hastebin page: a big text area that uploads its content as a text file and
// opens the resulting preview.
(function () {
  Filez.initChrome();
  const $ = (id) => document.getElementById(id);
  const ta = $("haste-text");
  ta.focus();

  function keyHeader() {
    const k = localStorage.getItem("filez-key") || sessionStorage.getItem("filez-key") || "";
    return k ? { "X-Access-Key": k } : {};
  }

  async function save() {
    const text = ta.value;
    if (!text.trim()) { Filez.toast("Nichts zu speichern"); return; }

    let name = ($("haste-name").value || "").trim() || "paste.txt";
    if (!name.includes(".")) name += ".txt"; // ensure an extension for the preview

    const btn = $("haste-save");
    btn.disabled = true;
    const fd = new FormData();
    fd.append("file", new Blob([text], { type: "text/plain" }), name);
    fd.append("mode", "permanent");
    try {
      const r = await fetch("/api/upload", { method: "POST", headers: keyHeader(), body: fd });
      if (r.status === 401) {
        Filez.toast("Access Key nötig — auf der Startseite freischalten");
        btn.disabled = false;
        return;
      }
      if (!r.ok) throw new Error("HTTP " + r.status);
      const j = await r.json();
      location.href = j.preview_url; // open the paste
    } catch (e) {
      Filez.toast("Speichern fehlgeschlagen");
      btn.disabled = false;
    }
  }

  $("haste-save").addEventListener("click", save);
  document.addEventListener("keydown", (e) => {
    if ((e.ctrlKey || e.metaKey) && (e.key === "s" || e.key === "S")) {
      e.preventDefault();
      save();
    }
  });
})();
