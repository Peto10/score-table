(() => {
  const scoreboard = document.getElementById("scoreboard");
  const idleMessage = document.getElementById("idleMessage");
  const displayTimer = document.getElementById("displayTimer");
  const displayTimerValue = document.getElementById("displayTimerValue");
  const displayPauseStatus = document.getElementById("displayPauseStatus");
  const displayTimeoutStatus = document.getElementById("displayTimeoutStatus");
  const displayFrame = document.querySelector(".displayFrame");
  const team1Name = document.getElementById("team1Name");
  const team2Name = document.getElementById("team2Name");
  const team1Score = document.getElementById("team1Score");
  const team2Score = document.getElementById("team2Score");

  let timerState = null;
  let raf = null;
  let nameFitRaf = null;

  const fmtMMSS = (ms) => {
    const total = Math.max(0, Math.floor(ms / 1000));
    const m = Math.floor(total / 60);
    const s = total % 60;
    return String(m).padStart(2, "0") + ":" + String(s).padStart(2, "0");
  };

  const fitNameToLines = (el, maxLines, availableHeight) => {
    if (!el) return;
    // Reset to CSS-driven size first.
    el.style.removeProperty("font-size");

    const cs = getComputedStyle(el);
    const fontSizePx0 = parseFloat(cs.fontSize) || 16;
    const lineHeightPx = parseFloat(cs.lineHeight) || fontSizePx0 * 1.05;
    const maxHeight = Math.min(lineHeightPx * maxLines, availableHeight) + 0.5;

    let fontSizePx = fontSizePx0;
    const minFontPx = 14;
    let guard = 0;
    while (el.scrollHeight > maxHeight && fontSizePx > minFontPx && guard < 80) {
      fontSizePx = Math.max(minFontPx, fontSizePx - 1);
      el.style.fontSize = `${fontSizePx}px`;
      guard++;
    }
  };

  const fitNames = () => {
    const scoreHeight = Math.max(
      team1Score?.getBoundingClientRect().height || 0,
      team2Score?.getBoundingClientRect().height || 0,
    );
    const rowGap = displayFrame ? parseFloat(getComputedStyle(displayFrame).rowGap) || 0 : 0;
    const frameHeight = displayFrame?.clientHeight ?? Infinity;
    const availableHeight = Math.max(0, frameHeight - scoreHeight - rowGap);
    fitNameToLines(team1Name, 3, availableHeight);
    fitNameToLines(team2Name, 3, availableHeight);
  };

  const scheduleFitNames = () => {
    if (nameFitRaf) cancelAnimationFrame(nameFitRaf);
    nameFitRaf = requestAnimationFrame(fitNames);
  };

  const setState = (active) => {
    if (scoreboard) scoreboard.classList.toggle("isHidden", !active);
    if (idleMessage) idleMessage.classList.toggle("isHidden", !!active);
  };

  const setTimerVisible = (visible) => {
    if (!displayTimer) return;
    displayTimer.classList.toggle("isHidden", !visible);
    if (!visible) {
      setTimerPaused(false);
      setTimerExpired(false);
    }
  };

  const setTimerPaused = (paused) => {
    if (displayTimer) displayTimer.classList.toggle("isPaused", paused);
    if (displayPauseStatus) displayPauseStatus.classList.toggle("isHidden", !paused);
  };

  const setTimerExpired = (expired) => {
    document.body.classList.toggle("isTimedOut", expired);
    if (displayTimeoutStatus) displayTimeoutStatus.classList.toggle("isHidden", !expired);
  };

  const tickTimer = () => {
    if (!timerState || !displayTimerValue) return;
    if (!timerState.show) {
      setTimerVisible(false);
      return;
    }
    let remaining = timerState.remainingMs;
    if (timerState.running) {
      const elapsed = Date.now() - timerState.serverMs;
      remaining = Math.max(0, timerState.remainingMs - elapsed);
    }
    const running = timerState.running && remaining > 0;
    const expired = remaining <= 0 && (
      timerState.expired || (timerState.running && timerState.remainingMs > 0)
    );
    displayTimerValue.textContent = fmtMMSS(remaining);
    setTimerVisible(true);
    setTimerPaused(!running && !expired);
    setTimerExpired(expired);

    raf = running ? requestAnimationFrame(tickTimer) : null;
  };

  const startTimerLoop = () => {
    if (raf) cancelAnimationFrame(raf);
    raf = null;
    tickTimer();
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
    scheduleFitNames();

    timerState = {
      show: !!snap.showTimer,
      running: !!snap.timerRunning,
      expired: !!snap.timerExpired,
      remainingMs: Number(snap.timerRemainingMs ?? 0),
      serverMs: Number(snap.timerServerMs ?? Date.now()),
    };
    startTimerLoop();
  };

  // Default to idle until we receive a snapshot.
  setState(false);
  setTimerVisible(false);
  window.addEventListener("resize", scheduleFitNames);
  if (displayFrame && typeof ResizeObserver !== "undefined") {
    new ResizeObserver(scheduleFitNames).observe(displayFrame);
  }

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
