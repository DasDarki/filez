// Shared helpers for all Filez pages: theme toggle, toast, formatting.
(function () {
  const BRAND = "Filez — made with ♥ by DasDarki (github.com/DasDarki)";

  // ---- Theme ----
  const stored = localStorage.getItem("filez-theme");
  if (stored) document.documentElement.setAttribute("data-theme", stored);

  function currentTheme() {
    const attr = document.documentElement.getAttribute("data-theme");
    if (attr) return attr;
    return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
  }
  function applyToggleIcon() {
    const btn = document.getElementById("theme-toggle");
    if (btn) btn.textContent = currentTheme() === "dark" ? "☀" : "🌙";
  }
  function toggleTheme() {
    const next = currentTheme() === "dark" ? "light" : "dark";
    document.documentElement.setAttribute("data-theme", next);
    localStorage.setItem("filez-theme", next);
    applyToggleIcon();
  }

  // ---- Toast ----
  let toastTimer;
  function toast(msg) {
    const el = document.getElementById("toast");
    if (!el) return;
    el.textContent = msg;
    el.classList.add("show");
    clearTimeout(toastTimer);
    toastTimer = setTimeout(() => el.classList.remove("show"), 2200);
  }

  // ---- Formatting ----
  function fmtBytes(n) {
    if (n < 1024) return n + " B";
    const units = ["KB", "MB", "GB", "TB"];
    let i = -1;
    do { n /= 1024; i++; } while (n >= 1024 && i < units.length - 1);
    return n.toFixed(n < 10 ? 1 : 0) + " " + units[i];
  }

  function iconFor(mime, ext) {
    mime = mime || "";
    if (mime.startsWith("image/")) return "🖼";
    if (mime.startsWith("video/")) return "🎬";
    if (mime.startsWith("audio/")) return "🎵";
    if (mime === "application/pdf") return "📕";
    if (/zip|tar|rar|7z|gzip/.test(mime) || /^(zip|tar|gz|rar|7z)$/.test(ext || "")) return "🗜";
    if (mime.startsWith("text/") || /^(txt|md|json|xml|csv|log|yml|yaml|js|ts|go|py|rs|c|cpp|html|css)$/.test(ext || "")) return "📄";
    return "📦";
  }

  window.Filez = {
    BRAND, toast, fmtBytes, iconFor, toggleTheme, applyToggleIcon,
    initChrome() {
      const brand = document.getElementById("brand");
      if (brand) brand.textContent = BRAND;
      const btn = document.getElementById("theme-toggle");
      if (btn) btn.addEventListener("click", toggleTheme);
      applyToggleIcon();
    },
  };
})();
