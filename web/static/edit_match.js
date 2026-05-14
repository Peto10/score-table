(() => {
  document.addEventListener("click", (e) => {
    const btn = e.target?.closest?.("[data-confirm]");
    if (!btn) return;
    const msg = btn.getAttribute("data-confirm") || "Are you sure?";
    if (!confirm(msg)) {
      e.preventDefault();
      e.stopPropagation();
    }
  });

  const backLink = document.getElementById("backLink");
  if (backLink) {
    backLink.addEventListener("click", (e) => {
      const ok = confirm("Leave edit mode and discard changes?");
      if (!ok) {
        e.preventDefault();
        return;
      }
      const discardUrl = backLink.getAttribute("data-discard-url");
      if (discardUrl) {
        fetch(discardUrl, { method: "POST" })
          .catch(() => {})
          .finally(() => {
            window.location.href = backLink.getAttribute("href") || "/control_panel/history";
          });
        e.preventDefault();
      }
    });
  }

  // Intercept browser back button too.
  try {
    history.pushState({ editMatch: true }, "", window.location.href);
  } catch {
    // ignore
  }
  window.addEventListener("popstate", () => {
    const ok = confirm("Leave edit mode and discard changes?");
    if (!ok) {
      try {
        history.pushState({ editMatch: true }, "", window.location.href);
      } catch {
        // ignore
      }
      return;
    }
    const discardUrl = backLink?.getAttribute?.("data-discard-url");
    if (discardUrl) {
      fetch(discardUrl, { method: "POST" })
        .catch(() => {})
        .finally(() => {
          window.location.href = "/control_panel/history";
        });
    } else {
      window.location.href = "/control_panel/history";
    }
  });
})();

