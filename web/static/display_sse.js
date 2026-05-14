(() => {
  const scoreboard = document.getElementById("scoreboard");
  const idleMessage = document.getElementById("idleMessage");
  const team1Name = document.getElementById("team1Name");
  const team2Name = document.getElementById("team2Name");
  const team1Score = document.getElementById("team1Score");
  const team2Score = document.getElementById("team2Score");

  const setState = (active) => {
    if (scoreboard) scoreboard.classList.toggle("isHidden", !active);
    if (idleMessage) idleMessage.classList.toggle("isHidden", !!active);
  };

  const applySnapshot = (snap) => {
    if (!snap || !snap.active) {
      setState(false);
      return;
    }
    team1Name.textContent = snap.team1Name || "";
    team2Name.textContent = snap.team2Name || "";
    team1Score.textContent = String(snap.team1Score ?? 0);
    team2Score.textContent = String(snap.team2Score ?? 0);
    setState(true);
  };

  // Default to idle until we receive a snapshot.
  setState(false);

  const es = new EventSource("/events/score");
  es.addEventListener("snapshot", (e) => {
    try {
      applySnapshot(JSON.parse(e.data));
    } catch {
      // ignore
    }
  });
  es.addEventListener("clear", () => setState(false));
  es.onerror = () => {
    // EventSource will reconnect; keep last known state.
  };
})();

