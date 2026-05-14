(() => {
  const scoreboard = document.getElementById("scoreboard");
  const team1Name = document.getElementById("team1Name");
  const team2Name = document.getElementById("team2Name");
  const team1Score = document.getElementById("team1Score");
  const team2Score = document.getElementById("team2Score");

  const setHidden = (hidden) => {
    if (!scoreboard) return;
    scoreboard.classList.toggle("isHidden", hidden);
  };

  const applySnapshot = (snap) => {
    if (!snap || !snap.active) {
      setHidden(true);
      return;
    }
    team1Name.textContent = snap.team1Name || "";
    team2Name.textContent = snap.team2Name || "";
    team1Score.textContent = String(snap.team1Score ?? 0);
    team2Score.textContent = String(snap.team2Score ?? 0);
    setHidden(false);
  };

  // Keep the page visually empty until a match is active.
  setHidden(true);

  const es = new EventSource("/events/score");
  es.addEventListener("snapshot", (e) => {
    try {
      applySnapshot(JSON.parse(e.data));
    } catch {
      // ignore
    }
  });
  es.addEventListener("clear", () => setHidden(true));
  es.onerror = () => {
    // EventSource will reconnect; keep last known state.
  };
})();

