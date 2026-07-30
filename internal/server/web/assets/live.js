// Live viewer: polls the session revision and swaps the frame when it changes.
(function () {
  const parts = location.pathname.split("/").filter(Boolean); // ["l", "<id>"]
  const id = parts[1] || "";
  const img = document.getElementById("live-img");
  const status = document.getElementById("live-status");
  const nameEl = document.getElementById("live-name");
  const label = document.getElementById("live-label");
  const dot = document.getElementById("live-dot");

  let lastRev = -1;
  let timer = null;

  function showStatus(text) {
    status.textContent = text;
    status.classList.remove("hidden");
  }

  function ended() {
    label.textContent = "BEENDET";
    dot.classList.add("dead");
    img.classList.add("hidden");
    showStatus("Live-Session beendet.");
    if (timer) clearInterval(timer);
  }

  img.addEventListener("error", () => {
    // Non-image frame (or failed load): show a note instead of a broken image.
    img.classList.add("hidden");
    showStatus("Empfangen: " + (nameEl.textContent || "Datei") + " (keine Bildvorschau)");
  });

  async function poll() {
    try {
      const r = await fetch("/l/" + id + "/rev", { cache: "no-store" });
      if (!r.ok) throw new Error("http " + r.status);
      const j = await r.json();
      if (!j.alive) { ended(); return; }
      nameEl.textContent = j.name || "";
      if (!j.has_image) {
        showStatus("Warte auf erstes Bild…");
        return;
      }
      if (j.rev !== lastRev) {
        lastRev = j.rev;
        img.src = "/l/" + id + "/image?v=" + j.rev;
        img.classList.remove("hidden");
        status.classList.add("hidden");
      }
    } catch (e) {
      showStatus("Verbindung verloren – neuer Versuch…");
    }
  }

  poll();
  timer = setInterval(poll, 1000);
})();
