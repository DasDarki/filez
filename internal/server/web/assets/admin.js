// Admin page: manage access keys. The page is protected by HTTP Basic Auth, so
// the browser resends credentials to /api/admin/* automatically.
(function () {
  Filez.initChrome();
  const $ = (id) => document.getElementById(id);

  function fmtDate(unix) {
    if (!unix) return "—";
    return new Date(unix * 1000).toLocaleString();
  }

  async function loadKeys() {
    const err = $("admin-error");
    err.classList.add("hidden");
    let keys = [];
    try {
      const r = await fetch("/api/admin/keys");
      if (!r.ok) throw new Error("HTTP " + r.status);
      keys = await r.json();
    } catch (e) {
      err.textContent = "Konnte Keys nicht laden: " + e.message;
      err.classList.remove("hidden");
      return;
    }
    render(keys || []);
  }

  function render(keys) {
    const body = $("keys-body");
    body.innerHTML = "";
    $("empty-hint").classList.toggle("hidden", keys.length > 0);
    const now = Date.now() / 1000;
    for (const k of keys) {
      const expired = k.revoked || (k.expires_at && k.expires_at <= now);
      const tr = document.createElement("tr");
      tr.innerHTML =
        '<td><code>' + k.key + '</code></td>' +
        '<td>' + (k.label ? escapeHtml(k.label) : "—") + '</td>' +
        '<td>' + fmtDate(k.created_at) + '</td>' +
        '<td>' + fmtDate(k.expires_at) + '</td>' +
        '<td>' + (k.allow_permanent ? '<span class="badge">✓</span>' : '<span class="badge expired">–</span>') + '</td>' +
        '<td><span class="badge' + (expired ? " expired" : "") + '">' +
          (expired ? "abgelaufen" : "aktiv") + '</span></td>' +
        '<td></td>';
      const del = document.createElement("button");
      del.className = "icon-btn";
      del.title = "Löschen";
      del.textContent = "🗑";
      del.addEventListener("click", () => deleteKey(k.key));
      tr.lastElementChild.appendChild(del);
      body.appendChild(tr);
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
      created.innerHTML = "Neuer Key erstellt: <code>" + k.key + "</code>";
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

  function escapeHtml(s) {
    return s.replace(/[&<>"']/g, (c) =>
      ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
  }

  loadKeys();
})();
