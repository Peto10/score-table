(() => {
  const scoreboard = document.getElementById("scoreboard");
  const idleMessage = document.getElementById("idleMessage");
  const displayTimer = document.getElementById("displayTimer");
  const team1Name = document.getElementById("team1Name");
  const team2Name = document.getElementById("team2Name");
  const team1Score = document.getElementById("team1Score");
  const team2Score = document.getElementById("team2Score");

  let timerState = null;
  let raf = null;

  const fmtMMSS = (ms) => {
    const total = Math.max(0, Math.floor(ms / 1000));
    const m = Math.floor(total / 60);
    const s = total % 60;
    return String(m).padStart(2, "0") + ":" + String(s).padStart(2, "0");
  };

  const setState = (active) => {
    if (scoreboard) scoreboard.classList.toggle("isHidden", !active);
    if (idleMessage) idleMessage.classList.toggle("isHidden", !!active);
  };

  const setTimerVisible = (visible) => {
    if (!displayTimer) return;
    displayTimer.classList.toggle("isHidden", !visible);
  };

  const tickTimer = () => {
    if (!timerState || !displayTimer) return;
    if (!timerState.show) {
      setTimerVisible(false);
      return;
    }
    let remaining = timerState.remainingMs;
    if (timerState.running) {
      const elapsed = Date.now() - timerState.serverMs;
      remaining = Math.max(0, timerState.remainingMs - elapsed);
    }
    displayTimer.textContent = fmtMMSS(remaining);
    setTimerVisible(true);

    raf = requestAnimationFrame(tickTimer);
  };

  const startTimerLoop = () => {
    if (raf) cancelAnimationFrame(raf);
    raf = requestAnimationFrame(tickTimer);
  };

  const applySnapshot = (snap) => {
    if (!snap || !snap.active) {
      setState(false);
      timerState = null;
      setTimerVisible(false);
      return;
    }
    team1Name.textContent = snap.team1Name || "";
    team2Name.textContent = snap.team2Name || "";
    team1Score.textContent = String(snap.team1Score ?? 0);
    team2Score.textContent = String(snap.team2Score ?? 0);
    setState(true);

    timerState = {
      show: !!snap.showTimer,
      running: !!snap.timerRunning,
      remainingMs: Number(snap.timerRemainingMs ?? 0),
      serverMs: Number(snap.timerServerMs ?? Date.now()),
    };
    startTimerLoop();
  };

  // Default to idle until we receive a snapshot.
  setState(false);
  setTimerVisible(false);

  const es = new EventSource("/events/score");
  es.addEventListener("snapshot", (e) => {
    try {
      applySnapshot(JSON.parse(e.data));
    } catch {
      // ignore
    }
  });
  es.addEventListener("clear", () => {
    setState(false);
    timerState = null;
    setTimerVisible(false);
  });
  es.onerror = () => {
    // EventSource will reconnect; keep last known state.
  };
})();

