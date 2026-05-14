(() => {
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
})();

