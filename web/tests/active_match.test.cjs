const assert = require("node:assert/strict");
const { readFileSync } = require("node:fs");
const { join } = require("node:path");
const test = require("node:test");
const vm = require("node:vm");

const source = readFileSync(join(__dirname, "../static/active_match.js"), "utf8");

function setup() {
  const buttons = [..."p1234", undefined].map((shortcut) => ({
    dataset: shortcut ? { shortcut } : {},
    disabled: false,
    visible: true,
    clicks: 0,
    getClientRects() { return this.visible ? [{}] : []; },
    click() { this.clicks += 1; },
  }));
  const listeners = {};
  vm.runInNewContext(source, {
    document: {
      getElementById: () => null,
      querySelectorAll: () => buttons.filter((button) => button.dataset.shortcut),
      addEventListener: (type, listener) => { listeners[type] = listener; },
    },
    window: { location: { href: "/match" }, addEventListener() {} },
    history: { pushState() {} },
  });
  return {
    buttons,
    press(key, options = {}) {
      buttons.forEach((button) => { button.clicks = 0; });
      const event = {
        key, code: "", target: null, defaultPrevented: false,
        preventDefault() { this.defaultPrevented = true; },
        ...options,
      };
      listeners.keydown(event);
      return {
        clicked: buttons.filter((button) => button.clicks).map((button) => button.dataset.shortcut),
        prevented: event.defaultPrevented,
      };
    },
  };
}

function target(tag, { role, editable = false, parent = null } = {}) {
  return {
    tag, role, parent, isContentEditable: editable,
    closest(selectors) {
      for (let node = this; node; node = node.parent) {
        if (selectors.split(",").some((selector) => {
          const match = selector.trim();
          return match === node.tag || (match === "[role='textbox']" && node.role === "textbox");
        })) return node;
      }
      return null;
    },
  };
}

test("mapped letter and numeric keys click only their matching button", () => {
  const { press, buttons } = setup();
  for (const key of ["p", "P", "1", "2", "3", "4"]) {
    assert.deepEqual(press(key), { clicked: [key.toLowerCase()], prevented: true }, key);
  }
  for (const key of "1234") {
    assert.deepEqual(press(key, { code: `Numpad${key}` }), { clicked: [key], prevented: true });
  }
  assert.deepEqual(press("P", { shiftKey: true }), { clicked: ["p"], prevented: true });
  for (const key of ["x", "5", "Enter", " "]) {
    assert.deepEqual(press(key), { clicked: [], prevented: false }, key);
  }
  assert.equal(buttons.at(-1).clicks, 0);
});

test("unshifted physical number row works on Slovak layouts; shifted punctuation is ignored", () => {
  const { press } = setup();
  for (const [index, key] of ["+", "ľ", "š", "č"].entries()) {
    const digit = String(index + 1);
    assert.deepEqual(press(key, { code: `Digit${digit}` }), { clicked: [digit], prevented: true });
  }
  for (const [index, key] of ["!", "@", "#", "$"].entries()) {
    assert.deepEqual(press(key, { code: `Digit${index + 1}`, shiftKey: true }), { clicked: [], prevented: false });
  }
});

test("typing targets and nested textbox content leave shortcuts untouched", () => {
  const { press } = setup();
  const targets = [
    ...["input", "textarea", "select"].map((tag) => target(tag)),
    target("div", { role: "textbox" }),
    target("span", { parent: target("div", { role: "textbox" }) }),
    target("div", { editable: true }),
  ];
  for (const typingTarget of targets) {
    assert.deepEqual(press("p", { target: typingTarget }), { clicked: [], prevented: false });
  }
  assert.deepEqual(press("1", { target: target("button") }), { clicked: ["1"], prevented: true });
});

test("claimed, repeating, composing and modified events and unavailable buttons are ignored", () => {
  const { press, buttons } = setup();
  for (const flag of ["defaultPrevented", "repeat", "isComposing", "ctrlKey", "altKey", "metaKey"]) {
    assert.deepEqual(press("p", { [flag]: true }), { clicked: [], prevented: flag === "defaultPrevented" }, flag);
  }
  buttons[0].disabled = true;
  buttons[1].visible = false;
  for (const key of ["p", "1"]) {
    assert.deepEqual(press(key), { clicked: [], prevented: false });
  }
  assert.deepEqual(press("2"), { clicked: ["2"], prevented: true });
});
