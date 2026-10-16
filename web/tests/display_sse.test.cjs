const assert = require("node:assert/strict");
const { readFileSync } = require("node:fs");
const { join } = require("node:path");
const test = require("node:test");
const vm = require("node:vm");

const source = readFileSync(join(__dirname, "../static/display_sse.js"), "utf8");

function setup() {
  const element = () => {
    const classes = new Set();
    return {
      textContent: "", scrollHeight: 0, style: { removeProperty() {} },
      getBoundingClientRect: () => ({ height: 0 }),
      classList: {
        contains: (name) => classes.has(name),
        toggle: (name, value) => value ? classes.add(name) : classes.delete(name),
      },
    };
  };
  const ids = ["scoreboard", "idleMessage", "displayTimer", "displayTimerValue",
    "displayPauseStatus", "displayTimeoutStatus", "team1Name", "team2Name", "team1Score", "team2Score"];
  const elements = Object.fromEntries(ids.map((id) => [id, element()]));
  const body = element();
  const listeners = {};
  const frames = new Map();
  let now = 1000;
  let nextFrame = 0;
  vm.runInNewContext(source, {
    document: { body, getElementById: (id) => elements[id], querySelector: () => null },
    window: { addEventListener() {} },
    getComputedStyle: () => ({ fontSize: "16px", lineHeight: "17px" }),
    Date: { now: () => now },
    requestAnimationFrame: (callback) => { frames.set(++nextFrame, callback); return nextFrame; },
    cancelAnimationFrame: (id) => frames.delete(id),
    EventSource: class {
      addEventListener(type, listener) { listeners[type] = listener; }
    },
  });
  const emit = (type, snapshot) => listeners[type]({ data: JSON.stringify(snapshot) });
  return {
    push(overrides = {}) {
      emit("snapshot", {
        active: true, showTimer: true, timerRunning: false, timerPaused: false,
        timerExpired: false, timerRemainingMs: 60000, timerServerMs: now, ...overrides,
      });
    },
    clear: () => emit("clear"),
    advance(ms) {
      now += ms;
      const pending = [...frames.values()];
      frames.clear();
      pending.forEach((callback) => callback());
    },
    expect({ visible = true, paused = false, expired = false, text } = {}) {
      const has = (id, name) => elements[id].classList.contains(name);
      assert.equal(!has("displayTimer", "isHidden"), visible, "timer visibility");
      assert.equal(has("displayTimer", "isPaused"), paused, "timer paused style");
      assert.equal(!has("displayPauseStatus", "isHidden"), paused, "PAUSED status");
      assert.equal(body.classList.contains("isTimedOut"), expired, "timeout style");
      assert.equal(!has("displayTimeoutStatus", "isHidden"), expired, "TIME OUT status");
      if (text !== undefined) assert.equal(elements.displayTimerValue.textContent, text);
    },
  };
}

test("ready and manually set timers show PAUSED, including zero and legacy snapshots", () => {
  const ui = setup();
  ui.expect({ visible: false });
  for (const remaining of [60000, 0]) {
    for (const timerPaused of [false, true]) {
      ui.push({ timerRemainingMs: remaining, timerPaused });
      ui.expect({ paused: true, text: remaining ? "01:00" : "00:00" });
    }
  }
});

test("starting and resuming remove PAUSED; a paused timer keeps its remaining time", () => {
  const ui = setup();
  ui.push();
  ui.expect({ paused: true });
  ui.push({ timerRunning: true });
  ui.expect({ text: "01:00" });
  ui.advance(12500);
  ui.expect({ text: "00:47" });
  ui.push({ timerRemainingMs: 47500, timerPaused: true });
  ui.advance(5000);
  ui.expect({ paused: true, text: "00:47" });
  ui.push({ timerRemainingMs: 47500, timerRunning: true, timerPaused: true });
  ui.advance(1000);
  ui.expect({ text: "00:46" });
});

test("expiry shows TIME OUT; reconnect preserves it and reset restores PAUSED", () => {
  const ui = setup();
  ui.push({ timerRunning: true, timerRemainingMs: 1000 });
  ui.advance(1000);
  ui.expect({ expired: true, text: "00:00" });
  ui.push({ timerRemainingMs: 0, timerExpired: true, timerPaused: true });
  ui.advance(2000);
  ui.expect({ expired: true, text: "00:00" });
  for (const remaining of [60000, 0]) {
    ui.push({ timerRemainingMs: 0, timerExpired: true });
    ui.push({ timerRemainingMs: remaining });
    ui.expect({ paused: true, text: remaining ? "01:00" : "00:00" });
  }
});

test("hidden timers, clear events and inactive snapshots remove both statuses", () => {
  for (const hide of [
    (ui) => ui.push({ showTimer: false }),
    (ui) => ui.clear(),
    (ui) => ui.push({ active: false }),
  ]) {
    for (const expired of [false, true]) {
      const ui = setup();
      ui.push({ timerRemainingMs: expired ? 0 : 60000, timerExpired: expired });
      hide(ui);
      ui.advance(2000);
      ui.expect({ visible: false });
    }
  }
});
