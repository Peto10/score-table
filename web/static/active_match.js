(() => {
  const startTimerTick = () => {
    const el = document.getElementById("timerValue");
    if (!el) return;

    const fmt = (ms) => {
      const total = Math.max(0, Math.floor(ms / 1000));
      const m = Math.floor(total / 60);
      const s = total % 60;
      return String(m).padStart(2, "0") + ":" + String(s).padStart(2, "0");
    };

    const show = el.getAttribute("data-show") === "1";
    if (!show) return;

    const running = el.getAttribute("data-running") === "1";
    const remainingMs = Number(el.getAttribute("data-remaining-ms") || "0");
    const serverMs = Number(el.getAttribute("data-server-ms") || String(Date.now()));

    if (!running) {
      el.textContent = fmt(remainingMs);
      return;
    }

    const render = () => {
      const elapsed = Date.now() - serverMs;
      const rem = Math.max(0, remainingMs - elapsed);
      el.textContent = fmt(rem);
      if (rem > 0) requestAnimationFrame(render);
    };

    requestAnimationFrame(render);
  };

  const confirmButtons = () => {
    document.addEventListener("click", (e) => {
      const btn = e.target?.closest?.("[data-confirm]");
      if (!btn) return;
      const msg = btn.getAttribute("data-confirm") || "Are you sure?";
      if (!confirm(msg)) {
        e.preventDefault();
        e.stopPropagation();
      }
    });
  };

  const interceptBrowserBack = () => {
    // Create an extra history entry so we can intercept Back button via popstate.
    try {
      history.pushState({ activeMatch: true }, "", window.location.href);
    } catch {
      // ignore
    }

    window.addEventListener("popstate", () => {
      const ok = confirm("Going back will discard the match and it will NOT be saved. Continue?");
      if (!ok) {
        try {
          history.pushState({ activeMatch: true }, "", window.location.href);
        } catch {
          // ignore
        }
        return;
      }
      fetch("/control_panel/active_match/discard", { method: "POST" })
        .catch(() => {})
        .finally(() => {
          window.location.href = "/control_panel";
        });
    });
  };

  const backLink = document.getElementById("backLink");
  if (backLink) {
    backLink.addEventListener("click", (e) => {
      const ok = confirm("Going back will discard the match and it will NOT be saved. Continue?");
      if (!ok) {
        e.preventDefault();
        return;
      }
      // Best effort discard before navigating.
      fetch("/control_panel/active_match/discard", { method: "POST" })
        .catch(() => {})
        .finally(() => {
          window.location.href = backLink.getAttribute("href") || "/control_panel";
        });
      e.preventDefault();
    });
  }

  confirmButtons();
  interceptBrowserBack();
  startTimerTick();
})();

