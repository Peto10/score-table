(() => {
  let suppressLeaveWarning = false;

  const confirmButtons = () => {
    document.addEventListener("click", (e) => {
      const btn = e.target?.closest?.("[data-confirm]");
      if (!btn) return;
      const msg = btn.getAttribute("data-confirm") || "Are you sure?";
      if (!confirm(msg)) {
        e.preventDefault();
        e.stopPropagation();
      } else {
        suppressLeaveWarning = true;
      }
    });
  };

  const warnOnLeave = () => {
    window.addEventListener("beforeunload", (e) => {
      if (suppressLeaveWarning) return;
      // Most browsers ignore custom text; returning a string still triggers the warning.
      e.preventDefault();
      e.returnValue = "If you leave now, the match will not be saved.";
      return e.returnValue;
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
      suppressLeaveWarning = true;
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
      suppressLeaveWarning = true;
      // Best effort discard before navigating.
      fetch("/control_panel/active_match/discard", { method: "POST" })
        .catch(() => {})
        .finally(() => {
          window.location.href = backLink.getAttribute("href") || "/control_panel";
        });
      e.preventDefault();
    });
  }

  const saveBtn = document.getElementById("saveBtn");
  if (saveBtn) {
    saveBtn.addEventListener("click", () => {
      suppressLeaveWarning = true;
    });
  }

  confirmButtons();
  warnOnLeave();
  interceptBrowserBack();
})();

