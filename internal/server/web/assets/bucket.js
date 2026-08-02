// Sync bucket page: anyone with the code can upload, list and download. The
// creator (owner token in localStorage) additionally sees a "close" button.
(function () {
  Filez.initChrome();
  const $ = (id) => document.getElementById(id);

  const code = (location.pathname.split("/").filter(Boolean)[1] || "").trim();
  const ownerToken = localStorage.getItem("filez-sync-owner-" + code) || "";
  let timer = null;
  let currentFiles = [];

  $("sync-code").textContent = code;
  $("sync-url").value = location.href;
  if (ownerToken) $("sync-close").classList.remove("hidden");

  function showClosed() {
    if (timer) clearInterval(timer);
    $("bucket").classList.add("hidden");
    $("bucket-closed").classList.remove("hidden");
  }

  // ---- file list (polled) ----
  async function loadFiles() {
    let data;
    try {
      const r = await fetch("/api/sync/" + code, { cache: "no-store" });
      data = await r.json();
    } catch { return; }
    if (!data.alive) { showClosed(); return; }
    render(data.files || []);
  }

  function render(files) {
    currentFiles = files;
    const list = $("files-list");
    $("files-empty").classList.toggle("hidden", files.length > 0);
    $("files-label").textContent = "Dateien" + (files.length ? " (" + files.length + ")" : "");
    const many = files.length > 1;
    const dl = $("download-all");
    dl.href = "/api/sync/" + code + "/zip";
    dl.classList.toggle("hidden", !many);
    $("download-each").classList.toggle("hidden", !many);
    list.innerHTML = "";
    for (const f of files) {
      const ext = f.name.includes(".") ? f.name.split(".").pop() : "";
      const row = document.createElement("div");
      row.className = "file-chip";
      row.style.marginTop = "8px";
      row.innerHTML =
        '<span class="fc-icon">' + Filez.iconFor(f.mime, ext) + "</span>" +
        '<div style="min-width:0"><div class="fc-name"></div>' +
        '<div class="fc-size">' + Filez.fmtBytes(f.size) + "</div></div>";
      row.querySelector(".fc-name").textContent = f.name;
      const a = document.createElement("a");
      a.className = "btn btn-ghost";
      a.style.marginLeft = "auto";
      a.href = "/api/sync/" + code + "/" + f.id;
      a.setAttribute("download", f.name);
      a.textContent = "⬇";
      a.title = "Herunterladen";
      row.appendChild(a);
      list.appendChild(row);
    }
  }

  // ---- upload ----
  const dz = $("dropzone");
  const fileInput = $("file-input");
  dz.addEventListener("click", () => fileInput.click());
  fileInput.addEventListener("change", () => uploadAll([...fileInput.files]));
  ["dragover", "dragenter"].forEach((ev) =>
    dz.addEventListener(ev, (e) => { e.preventDefault(); dz.classList.add("dragover"); }));
  ["dragleave", "dragend"].forEach((ev) => dz.addEventListener(ev, () => dz.classList.remove("dragover")));
  dz.addEventListener("drop", (e) => {
    e.preventDefault();
    dz.classList.remove("dragover");
    uploadAll([...e.dataTransfer.files]);
  });

  async function uploadAll(files) {
    if (!files.length) return;
    const err = $("sync-error");
    err.classList.add("hidden");
    const prog = $("upload-progress");
    const bar = prog.querySelector("span");
    prog.classList.add("show");
    let done = 0;
    for (const f of files) {
      try {
        await uploadOne(f);
      } catch (e) {
        err.textContent = f.name + ": " + e.message;
        err.classList.remove("hidden");
      }
      done++;
      bar.style.width = Math.round((done / files.length) * 100) + "%";
    }
    prog.classList.remove("show");
    bar.style.width = "0";
    fileInput.value = "";
    Filez.toast(files.length > 1 ? files.length + " Dateien hochgeladen" : "Hochgeladen");
    loadFiles();
  }

  function uploadOne(file) {
    return new Promise((resolve, reject) => {
      const fd = new FormData();
      fd.append("file", file);
      const xhr = new XMLHttpRequest();
      xhr.open("POST", "/api/sync/" + code);
      xhr.onload = () => {
        if (xhr.status >= 200 && xhr.status < 300) resolve();
        else {
          let msg = "Fehler " + xhr.status;
          try { msg = JSON.parse(xhr.responseText).error || msg; } catch {}
          reject(new Error(msg));
        }
      };
      xhr.onerror = () => reject(new Error("Netzwerkfehler"));
      xhr.send(fd);
    });
  }

  // ---- copy + close ----
  $("sync-copy").addEventListener("click", async () => {
    try { await navigator.clipboard.writeText(location.href); } catch {}
    Filez.toast("Link kopiert");
  });

  $("sync-close").addEventListener("click", async () => {
    if (!confirm("Diesen Sync-Bucket schließen? Alle Dateien gehen verloren.")) return;
    try {
      const r = await fetch("/api/sync/" + code, {
        method: "DELETE",
        headers: { "X-Sync-Owner": ownerToken },
      });
      if (!r.ok) throw new Error("HTTP " + r.status);
      localStorage.removeItem("filez-sync-owner-" + code);
      showClosed();
    } catch (e) {
      Filez.toast("Schließen fehlgeschlagen");
    }
  });

  // Download each file as its own download, one after another. Browsers may ask
  // once to allow multiple downloads.
  $("download-each").addEventListener("click", async () => {
    const btn = $("download-each");
    btn.disabled = true;
    for (const f of currentFiles.slice()) {
      const a = document.createElement("a");
      a.href = "/api/sync/" + code + "/" + f.id;
      a.download = f.name || f.id;
      document.body.appendChild(a);
      a.click();
      a.remove();
      await new Promise((r) => setTimeout(r, 400));
    }
    btn.disabled = false;
    Filez.toast("Downloads gestartet");
  });

  loadFiles();
  timer = setInterval(loadFiles, 2000);
})();
