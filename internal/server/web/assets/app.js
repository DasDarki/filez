// Index page: access-key gate + drag & drop upload.
(function () {
  Filez.initChrome();

  const $ = (id) => document.getElementById(id);
  const KEY_STORE = "filez-key";

  let selectedFile = null;
  let mode = "permanent";
  let maxUploadSize = 0;
  let cleanupEnabled = false;
  let cleanupAfter = "";

  function getKey() {
    return localStorage.getItem(KEY_STORE) || sessionStorage.getItem(KEY_STORE) || "";
  }
  function setKey(key, remember) {
    // Cookie lets plain /d and /p links authenticate without JS.
    document.cookie = "filez_key=" + encodeURIComponent(key) + "; path=/; max-age=" +
      (remember ? 60 * 60 * 24 * 365 : "") + "; samesite=lax";
    if (remember) localStorage.setItem(KEY_STORE, key);
    else sessionStorage.setItem(KEY_STORE, key);
  }

  async function authCheck(key) {
    try {
      const r = await fetch("/api/auth/check", { headers: { "X-Access-Key": key } });
      return r.ok;
    } catch { return false; }
  }

  // ---- Bootstrap from /api/info ----
  async function boot() {
    let info = { public: true, admin_enabled: false, default_upload: "permanent" };
    try { info = await (await fetch("/api/info")).json(); } catch {}

    maxUploadSize = info.max_upload_size || 0;
    cleanupEnabled = !!info.cleanup;
    cleanupAfter = info.cleanup_after || "";
    if (info.admin_enabled) $("admin-link").classList.remove("hidden");

    // Default mode from server config (permanent | temp:...).
    if (info.default_upload && info.default_upload.startsWith("temp")) {
      selectMode("temp");
      const parts = info.default_upload.split(":");
      if (parts[1]) $("opt-ttl").value = parts[1];
    }

    if (info.public) { showApp(); return; }

    const key = getKey();
    if (key && (await authCheck(key))) showApp();
    else showGate();
  }

  function showGate() { $("gate").classList.add("show"); $("app").classList.add("hidden"); $("sync-card").classList.add("hidden"); }
  function showApp() {
    $("gate").classList.remove("show");
    $("app").classList.remove("hidden");
    $("sync-card").classList.remove("hidden");
    setupKeepUI();
  }

  // ---- Sync bucket ----
  $("create-sync").addEventListener("click", async () => {
    const btn = $("create-sync");
    btn.disabled = true;
    try {
      const key = getKey();
      const r = await fetch("/api/sync", { method: "POST", headers: key ? { "X-Access-Key": key } : {} });
      if (!r.ok) throw new Error("HTTP " + r.status);
      const j = await r.json();
      localStorage.setItem("filez-sync-owner-" + j.code, j.owner_token);
      location.href = j.url;
    } catch (e) {
      Filez.toast("Konnte Sync-Bucket nicht erstellen");
      btn.disabled = false;
    }
  });

  // Show the "keep permanent" checkbox when the current user may create permanent
  // files, otherwise a hint that permanent uploads get cleaned up when idle.
  async function setupKeepUI() {
    const keepLabel = $("keep-label");
    const hint = $("cleanup-hint");
    keepLabel.classList.add("hidden");
    hint.classList.add("hidden");
    if (!cleanupEnabled) return; // permanent really means permanent
    let allow = false;
    try {
      const key = getKey();
      const r = await fetch("/api/auth/check", { headers: key ? { "X-Access-Key": key } : {} });
      if (r.ok) allow = !!(await r.json()).allow_permanent;
    } catch {}
    if (allow) {
      keepLabel.classList.remove("hidden");
    } else {
      hint.textContent = "Permanente Uploads werden nach " + cleanupAfter + " ohne Abruf automatisch gelöscht.";
      hint.classList.remove("hidden");
    }
  }

  // ---- Gate ----
  $("gate-submit").addEventListener("click", async () => {
    const key = $("gate-key").value.trim();
    const err = $("gate-error");
    err.classList.add("hidden");
    if (!key) return;
    if (await authCheck(key)) {
      setKey(key, $("gate-remember").checked);
      showApp();
    } else {
      err.textContent = "Ungültiger Access Key.";
      err.classList.remove("hidden");
    }
  });
  $("gate-key").addEventListener("keydown", (e) => { if (e.key === "Enter") $("gate-submit").click(); });

  // ---- Mode selection ----
  function selectMode(m) {
    mode = m;
    document.querySelectorAll("#mode button").forEach((b) =>
      b.classList.toggle("active", b.dataset.mode === m));
    document.querySelectorAll(".input-row").forEach((row) =>
      row.classList.toggle("show", row.dataset.for === m));
  }
  document.querySelectorAll("#mode button").forEach((b) =>
    b.addEventListener("click", () => selectMode(b.dataset.mode)));

  // ---- File selection ----
  const dz = $("dropzone");
  const fileInput = $("file-input");
  dz.addEventListener("click", () => fileInput.click());
  fileInput.addEventListener("change", () => { if (fileInput.files[0]) setFile(fileInput.files[0]); });
  ["dragover", "dragenter"].forEach((ev) =>
    dz.addEventListener(ev, (e) => { e.preventDefault(); dz.classList.add("dragover"); }));
  ["dragleave", "dragend"].forEach((ev) =>
    dz.addEventListener(ev, () => dz.classList.remove("dragover")));
  dz.addEventListener("drop", (e) => {
    e.preventDefault();
    dz.classList.remove("dragover");
    if (e.dataTransfer.files[0]) setFile(e.dataTransfer.files[0]);
  });

  function setFile(f) {
    if (maxUploadSize && f.size > maxUploadSize) {
      Filez.toast("Datei zu groß (max " + Filez.fmtBytes(maxUploadSize) + ")");
      return;
    }
    selectedFile = f;
    const ext = f.name.includes(".") ? f.name.split(".").pop() : "";
    $("fc-name").textContent = f.name;
    $("fc-size").textContent = Filez.fmtBytes(f.size);
    $("file-chip").querySelector(".fc-icon").textContent = Filez.iconFor(f.type, ext);
    $("file-chip").classList.remove("hidden");
    dz.classList.add("hidden");
    $("upload-btn").disabled = false;
    hideResult();
  }
  $("fc-remove").addEventListener("click", resetFile);
  function resetFile() {
    selectedFile = null;
    fileInput.value = "";
    $("file-chip").classList.add("hidden");
    dz.classList.remove("hidden");
    $("upload-btn").disabled = true;
  }

  // ---- Upload ----
  $("upload-btn").addEventListener("click", doUpload);
  function doUpload() {
    if (!selectedFile) return;
    const errBox = $("upload-error");
    errBox.classList.add("hidden");

    const fd = new FormData();
    fd.append("file", selectedFile);
    // "password" is a UI mode that maps to a permanent file with a password.
    fd.append("mode", mode === "password" ? "permanent" : mode);
    if (mode === "temp") fd.append("ttl", $("opt-ttl").value.trim());
    if (mode === "limited") fd.append("downloads", $("opt-downloads").value);
    if (mode === "password") fd.append("password", $("opt-password").value);
    if (mode === "permanent" && $("opt-keep").checked) fd.append("keep", "true");

    const xhr = new XMLHttpRequest();
    xhr.open("POST", "/api/upload");
    const key = getKey();
    if (key) xhr.setRequestHeader("X-Access-Key", key);

    const prog = $("progress");
    const bar = prog.querySelector("span");
    prog.classList.add("show");
    $("upload-btn").disabled = true;

    xhr.upload.onprogress = (e) => {
      if (e.lengthComputable) bar.style.width = Math.round((e.loaded / e.total) * 100) + "%";
    };
    xhr.onerror = () => finishError("Netzwerkfehler beim Upload.");
    xhr.onload = () => {
      prog.classList.remove("show");
      bar.style.width = "0";
      if (xhr.status >= 200 && xhr.status < 300) {
        showResult(JSON.parse(xhr.responseText));
      } else {
        let msg = "Upload fehlgeschlagen (" + xhr.status + ").";
        try { msg = JSON.parse(xhr.responseText).error || msg; } catch {}
        finishError(msg);
      }
    };
    xhr.send(fd);

    function finishError(msg) {
      prog.classList.remove("show");
      $("upload-btn").disabled = false;
      errBox.textContent = msg;
      errBox.classList.remove("hidden");
    }
  }

  function showResult(res) {
    const url = new URL(res.url, location.origin).href;
    $("result-link").value = url;
    $("preview-link").href = res.preview_url;
    $("result").classList.add("show");
    $("upload-btn").disabled = false;
  }
  function hideResult() { $("result").classList.remove("show"); }

  $("copy-btn").addEventListener("click", async () => {
    const v = $("result-link").value;
    try { await navigator.clipboard.writeText(v); }
    catch { $("result-link").select(); document.execCommand("copy"); }
    Filez.toast("Link kopiert");
  });
  $("another-btn").addEventListener("click", () => { resetFile(); hideResult(); });

  boot();
})();
