// Admin page: manage access keys. The page is protected by HTTP Basic Auth, so
// the browser resends credentials to /api/admin/* automatically.
(function () {
  Filez.initChrome();
  const $ = (id) => document.getElementById(id);

  function fmtDate(unix) {
    if (!unix) return "nie";
    return new Date(unix * 1000).toLocaleString();
  }
  function escapeHtml(s) {
    return String(s).replace(/[&<>"']/g, (c) =>
      ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
  }

  async function loadKeys() {
    const err = $("admin-error");
    err.classList.add("hidden");
    let keys = [];
    try {
      const r = await fetch("/api/admin/keys");
      if (!r.ok) throw new Error("HTTP " + r.status);
      keys = (await r.json()) || [];
    } catch (e) {
      err.textContent = "Konnte Keys nicht laden: " + e.message;
      err.classList.remove("hidden");
      return;
    }
    render(keys);
  }

  function render(keys) {
    const list = $("keys-list");
    list.innerHTML = "";
    $("empty-hint").classList.toggle("hidden", keys.length > 0);
    const now = Date.now() / 1000;
    for (const k of keys) {
      list.appendChild(card(k, k.revoked || (k.expires_at && k.expires_at <= now)));
    }
  }

  function card(k, expired) {
    const el = document.createElement("div");
    el.className = "key-card";
    el.innerHTML =
      '<div class="kc-top">' +
        '<code class="kc-code" title="Klick zum Kopieren">' + escapeHtml(k.key) + "</code>" +
        '<div class="kc-actions">' +
          '<button class="icon-btn sm" data-act="edit" title="Bearbeiten">✎</button>' +
          '<button class="icon-btn sm" data-act="del" title="Löschen">🗑</button>' +
        "</div>" +
      "</div>" +
      '<div class="kc-badges">' +
        '<span class="badge ' + (expired ? "expired" : "ok") + '">' + (expired ? "abgelaufen" : "aktiv") + "</span>" +
        (k.allow_permanent ? '<span class="badge">permanent erlaubt</span>' : "") +
      "</div>" +
      '<div class="kc-meta">' +
        "<span>Label: " + (k.label ? escapeHtml(k.label) : "—") + "</span>" +
        "<span>erstellt: " + fmtDate(k.created_at) + "</span>" +
        "<span>Ablauf: " + fmtDate(k.expires_at) + "</span>" +
      "</div>";

    el.querySelector(".kc-code").addEventListener("click", () => copy(k.key));
    el.querySelector('[data-act="del"]').addEventListener("click", () => deleteKey(k.key));
    el.querySelector('[data-act="edit"]').addEventListener("click", () => toggleEdit(el, k));
    return el;
  }

  function toggleEdit(card, k) {
    const existing = card.querySelector(".key-edit");
    if (existing) { existing.remove(); return; }

    const box = document.createElement("div");
    box.className = "key-edit";
    box.innerHTML =
      '<input class="ke-label" type="text" placeholder="Bezeichnung" />' +
      '<label class="check"><input class="ke-perm" type="checkbox" /> Darf permanente Dateien hochladen</label>' +
      '<input class="ke-expiry" type="text" placeholder="Ablauf ändern: leer = unverändert · \'nie\' = kein Ablauf · sonst z.B. 7d" />' +
      '<div class="ke-actions">' +
        '<button class="btn btn-primary ke-save">Speichern</button>' +
        '<button class="btn btn-ghost ke-cancel">Abbrechen</button>' +
      "</div>";
    box.querySelector(".ke-label").value = k.label || "";
    box.querySelector(".ke-perm").checked = !!k.allow_permanent;
    box.querySelector(".ke-cancel").addEventListener("click", () => box.remove());
    box.querySelector(".ke-save").addEventListener("click", () => saveKey(k.key, box));
    card.appendChild(box);
    box.querySelector(".ke-label").focus();
  }

  async function saveKey(key, box) {
    const body = {
      label: box.querySelector(".ke-label").value.trim(),
      allow_permanent: box.querySelector(".ke-perm").checked,
    };
    const exp = box.querySelector(".ke-expiry").value.trim();
    if (exp) body.expiry = exp; // empty = leave expiry unchanged
    try {
      const r = await fetch("/api/admin/keys/" + encodeURIComponent(key), {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
      });
      if (!r.ok) {
        let msg = "HTTP " + r.status;
        try { msg = (await r.json()).error || msg; } catch {}
        throw new Error(msg);
      }
      Filez.toast("Key aktualisiert");
      loadKeys();
    } catch (e) {
      Filez.toast("Speichern fehlgeschlagen: " + e.message);
    }
  }

  async function deleteKey(key) {
    try {
      const r = await fetch("/api/admin/keys/" + encodeURIComponent(key), { method: "DELETE" });
      if (!r.ok) throw new Error("HTTP " + r.status);
      Filez.toast("Key gelöscht");
      loadKeys();
    } catch (e) {
      Filez.toast("Löschen fehlgeschlagen");
    }
  }

  async function copy(text) {
    try { await navigator.clipboard.writeText(text); }
    catch { return; }
    Filez.toast("Key kopiert");
  }

  $("create-btn").addEventListener("click", async () => {
    const label = $("new-label").value.trim();
    const expiry = $("new-expiry").value.trim();
    const allowPermanent = $("new-allow-permanent").checked;
    const created = $("created");
    const err = $("admin-error");
    created.classList.add("hidden");
    err.classList.add("hidden");
    try {
      const r = await fetch("/api/admin/keys", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ label, expiry, allow_permanent: allowPermanent }),
      });
      if (!r.ok) {
        let msg = "HTTP " + r.status;
        try { msg = (await r.json()).error || msg; } catch {}
        throw new Error(msg);
      }
      const k = await r.json();
      created.innerHTML = "Neuer Key erstellt: <code>" + escapeHtml(k.key) + "</code>";
      created.classList.remove("hidden");
      $("new-label").value = "";
      $("new-expiry").value = "";
      $("new-allow-permanent").checked = false;
      loadKeys();
    } catch (e) {
      err.textContent = "Erstellen fehlgeschlagen: " + e.message;
      err.classList.remove("hidden");
    }
  });

  loadKeys();
})();
