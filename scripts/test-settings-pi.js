#!/usr/bin/env node
"use strict";

const fs = require("fs");
const vm = require("vm");

function assert(condition, msg) {
  if (!condition) {
    throw new Error(msg);
  }
}

class FakeElement {
  constructor(initial = {}) {
    this.value = initial.value || "";
    this.checked = initial.checked || false;
    this.textContent = initial.textContent || "";
    this.style = initial.style || {};
    this.handlers = {};
    this.children = [];
    this.disabled = initial.disabled || false;
    this.selected = initial.selected || false;
    this.dataset = initial.dataset || {};
    this._innerHTML = "";
  }
  addEventListener(evt, fn) {
    this.handlers[evt] = this.handlers[evt] || [];
    this.handlers[evt].push(fn);
  }
  trigger(evt) {
    const fns = this.handlers[evt] || [];
    fns.forEach((fn) => fn({
      target: this,
      preventDefault() {},
      stopPropagation() {},
    }));
  }
  appendChild(child) {
    this.children.push(child);
  }
  querySelectorAll() {
    return [];
  }
  get options() {
    return this.children;
  }
  set innerHTML(value) {
    this._innerHTML = value;
    if (value === "") {
      this.children = [];
    }
  }
  get innerHTML() {
    return this._innerHTML;
  }
}

function loadSandbox(opts = {}) {
  const elements = {
    pollInterval: new FakeElement({ value: "1000" }),
    currentRate: new FakeElement({ textContent: "" }),
    tileBackground: new FakeElement({ value: "#112233" }),
    tileTextColor: new FakeElement({ value: "#aabbcc" }),
    showLabel: new FakeElement({ checked: true }),
    connectionStatus: new FakeElement({ textContent: "", style: {} }),
    sourceProfileSelect: new FakeElement(),
    defaultProfileSelect: new FakeElement(),
    deleteProfileBtn: new FakeElement(),
    saveProfileBtn: new FakeElement(),
    profileName: new FakeElement(),
    lhmHost: new FakeElement({ value: "127.0.0.1" }),
    lhmPort: new FakeElement({ value: "8085" }),
    profileKind: new FakeElement({ value: "" }),
    sparkDashUnit: new FakeElement({ value: "" }),
    sparkDashUnitRow: new FakeElement({ style: { display: "none" } }),
    sparkDashPortHint: new FakeElement({ style: { display: "none" } }),
    sparkDashUnitStatus: new FakeElement({ textContent: "" }),
    refreshSparkDashUnitsBtn: new FakeElement(),
    cliProxyKeyRow: new FakeElement({ style: { display: "none" } }),
    cliProxyPortHint: new FakeElement({ style: { display: "none" } }),
    cliProxyManagementKey: new FakeElement({ value: "" }),
    cliProxyAccountsStatus: new FakeElement({ textContent: "" }),
    listCliProxyAccountsBtn: new FakeElement(),
    connectionHeading: new FakeElement({ textContent: "LHM Connection" }),
    profileIcon: new FakeElement({ value: "server" }),
    profileAccent: new FakeElement({ value: "#6E9EFF" }),
    addGlobalThresholdBtn: new FakeElement(),
    newGlobalThresholdName: new FakeElement(),
  };
  const sent = [];
  const intervalFns = [];

  const sandbox = {
    console,
    JSON,
    URL,
    URLSearchParams,
    setTimeout: (fn) => {
      fn();
      return 1;
    },
    setInterval: (fn) => {
      intervalFns.push(fn);
      return intervalFns.length;
    },
    clearInterval: () => {},
    window: null,
    websocket: null,
    WebSocket: function () {
      return opts.mockSocket || { readyState: 1, send() {} };
    },
    document: {
      readyState: "complete",
      addEventListener() {},
      activeElement: null,
      getElementById(id) {
        return elements[id] || null;
      },
      createElement() {
        return new FakeElement();
      },
      querySelector() {
        return null;
      },
    },
  };
  sandbox.window = sandbox;

  vm.createContext(sandbox);
  vm.runInContext(fs.readFileSync("com.moeilijk.lhm.sdPlugin/settings_pi.js", "utf8"), sandbox);

  sandbox.websocket = {
    readyState: 1,
    send(msg) {
      sent.push(JSON.parse(msg));
    },
  };

  return { sandbox, elements, sent, getIntervalFns: () => intervalFns };
}

function testShowLabelPayload() {
  const { sandbox, elements, sent } = loadSandbox();
  sandbox.context = "ctx-settings";
  sandbox.uuid = "ctx-fallback";

  sandbox.saveTileSettings("force");
  elements.showLabel.checked = false;
  sandbox.saveTileSettings("force");

  const updates = sent.filter(
    (m) => m.event === "sendToPlugin" && m.payload && m.payload.updateTileAppearance
  );
  assert(updates.length >= 2, "expected at least two updateTileAppearance events");
  assert(updates[0].payload.updateTileAppearance.showLabel === true, "first payload should be true");
  assert(updates[updates.length - 1].payload.updateTileAppearance.showLabel === false, "last payload should be false");
}

function testContextFanout() {
  const { sandbox, sent } = loadSandbox();
  sandbox.context = "ctx-action";
  sandbox.uuid = "ctx-pi";

  sandbox.saveTileSettings("force");
  const setSettings = sent.filter((m) => m.event === "setSettings");
  const updateMsgs = sent.filter((m) => m.event === "sendToPlugin" && m.payload && m.payload.updateTileAppearance);
  assert(setSettings.length === 1, "expected one setSettings message");
  assert(updateMsgs.length === 1, "expected one updateTileAppearance message");
  assert(setSettings[0].context === "ctx-pi", "setSettings should use PI uuid context");
  assert(updateMsgs[0].context === "ctx-pi", "sendToPlugin should use PI uuid context");
}

function testPollIntervalEvents() {
  const { sandbox, elements, sent } = loadSandbox();
  sandbox.context = "ctx-settings";
  sandbox.uuid = "ctx-fallback";
  elements.pollInterval.value = "2000";
  elements.pollInterval.trigger("change");

  const pollMsgs = sent.filter((m) => m.event === "sendToPlugin" && m.payload && m.payload.setPollInterval === 2000);
  assert(pollMsgs.length >= 1, "expected setPollInterval via sendToPlugin");
}

function testDidReceiveSettingsAppliesUi() {
  const ws = {
    readyState: 1,
    send() {},
    onopen: null,
    onmessage: null,
  };
  const { sandbox, elements } = loadSandbox({ mockSocket: ws });
  sandbox.connectElgatoStreamDeckSocket("12345", "uuid-x", "registerPropertyInspector", "{}", JSON.stringify({
    action: "com.moeilijk.lhm.settings",
    context: "ctx-x",
  }));
  ws.onmessage({
    data: JSON.stringify({
      event: "didReceiveSettings",
      payload: {
        settings: {
          tileBackground: "#334455",
          tileTextColor: "#fedcba",
          showLabel: false,
        },
      },
    }),
  });
  assert(elements.tileBackground.value === "#334455", "tileBackground not applied");
  assert(elements.tileTextColor.value === "#fedcba", "tileTextColor not applied");
  assert(elements.showLabel.checked === false, "showLabel not applied");
}

function testMalformedInputsDoNotCrash() {
  const ws = {
    readyState: 1,
    send() {},
    onopen: null,
    onmessage: null,
  };
  const { sandbox } = loadSandbox({ mockSocket: ws });
  sandbox.connectElgatoStreamDeckSocket("12345", "uuid-x", "registerPropertyInspector", "{bad", "{bad");
  ws.onmessage({ data: "{bad" });
}

function testPollingFallbackSave() {
  const { sandbox, elements, sent, getIntervalFns } = loadSandbox();
  sandbox.context = "ctx-settings";
  sandbox.uuid = "ctx-fallback";

  const ws = {
    readyState: 1,
    send(msg) {
      sent.push(JSON.parse(msg));
    },
    onopen: null,
    onmessage: null,
  };
  sandbox.WebSocket = function () {
    return ws;
  };
  sandbox.connectElgatoStreamDeckSocket("12345", "uuid-x", "registerPropertyInspector", "{}", JSON.stringify({
    action: "com.moeilijk.lhm.settings",
    context: "ctx-settings",
  }));
  ws.onopen();

  elements.tileBackground.value = "#445566";
  const tick = getIntervalFns()[1];
  assert(typeof tick === "function", "interval fallback function missing");
  tick();

  const updates = sent.filter((m) => m.event === "sendToPlugin" && m.payload && m.payload.updateTileAppearance);
  assert(updates.length >= 1, "polling fallback did not send update");
}

function testStatusHeartbeatIsLightweight() {
  const { sandbox, sent, getIntervalFns } = loadSandbox();
  sandbox.context = "ctx-settings";
  sandbox.uuid = "ctx-pi";

  const ws = {
    readyState: 1,
    send(msg) {
      sent.push(JSON.parse(msg));
    },
    onopen: null,
    onmessage: null,
  };
  sandbox.WebSocket = function () {
    return ws;
  };
  sandbox.connectElgatoStreamDeckSocket("12345", "uuid-x", "registerPropertyInspector", "{}", JSON.stringify({
    action: "com.moeilijk.lhm.settings",
    context: "ctx-settings",
  }));
  ws.onopen();

  const statusTick = getIntervalFns()[0];
  assert(typeof statusTick === "function", "status heartbeat function missing");
  statusTick();

  const statusMessages = sent.filter(
    (m) => m.event === "sendToPlugin" && m.payload && (m.payload.settingsConnected || m.payload.requestSettingsStatus)
  );
  assert(statusMessages.length >= 2, "expected initial and heartbeat status messages");
  assert(statusMessages[0].payload.settingsConnected === true, "initial message should request full settings state");
  assert(statusMessages[statusMessages.length - 1].payload.requestSettingsStatus === true, "heartbeat should request lightweight status only");
}

function testFocusedProfileInputIsNotOverwritten() {
  const { sandbox, elements } = loadSandbox();
  sandbox.sourceProfiles = [
    { id: "source-1", name: "Primary", host: "10.0.0.1", port: 9000 },
  ];
  sandbox.selectedProfileId = "source-1";

  elements.profileName.value = "Primary";
  elements.lhmHost.value = "editing-host";
  elements.lhmPort.value = "8085";
  sandbox.document.activeElement = elements.lhmHost;

  sandbox.applySelectedProfileToUI();

  assert(elements.profileName.value === "Primary", "name should still sync when not focused");
  assert(elements.lhmHost.value === "editing-host", "focused host input should not be overwritten");
  assert(elements.lhmPort.value === "9000", "port should sync when not focused");
}

function testAddGlobalThresholdButtonSendsCommand() {
  const { sandbox, elements, sent } = loadSandbox();
  sandbox.context = "ctx-settings";
  sandbox.uuid = "ctx-pi";
  elements.newGlobalThresholdName.value = "Global CPU";

  elements.addGlobalThresholdBtn.trigger("click");

  const add = sent.find((m) => m.event === "sendToPlugin" && m.payload && m.payload.addGlobalThreshold === "Global CPU");
  assert(add, "add global threshold command missing");
  assert(elements.newGlobalThresholdName.value === "", "name input should clear after add");
}

function testGlobalThresholdWithoutEnabledRendersOpen() {
  const { sandbox } = loadSandbox();
  const elements = {};
  [
    ".threshold-item",
    ".threshold-name",
    ".threshold-text",
    ".threshold-reading-type",
    ".threshold-operator",
    ".threshold-value",
    ".threshold-hysteresis",
    ".threshold-dwell",
    ".threshold-cooldown",
    ".threshold-bg",
    ".threshold-fg",
    ".threshold-hl",
    ".threshold-vt",
    ".threshold-tc",
    ".threshold-toggle",
    ".threshold-settings",
    ".threshold-sticky-toggle",
    ".threshold-advanced-toggle",
    ".threshold-advanced-panel",
    ".threshold-remove",
  ].forEach((selector) => {
    elements[selector] = new FakeElement();
  });
  elements[".threshold-item"].dataset = {};

  const fakeClone = {
    querySelector(selector) {
      return elements[selector] || null;
    },
  };
  sandbox.document.querySelector = (selector) => {
    if (selector !== "#globalThresholdTemplate") return null;
    return { content: { cloneNode: () => fakeClone } };
  };

  sandbox.createGlobalThresholdElement({ id: "g1", name: "Global CPU" });

  assert(elements[".threshold-toggle"].textContent === "on", "missing enabled should render as on");
  assert(elements[".threshold-settings"].style.display === "block", "settings should be visible for enabled threshold");
}

function testSparkDashKindSwitchesPortAndShowsUnitRow() {
  const { sandbox, elements } = loadSandbox();
  elements.lhmPort.value = "8085";
  elements.profileKind.value = "sparkdash";
  elements.profileKind.trigger("change");

  assert(elements.lhmPort.value === "5555", "switching to SparkDash should default port 5555");
  assert(elements.sparkDashUnitRow.style.display === "", "unit row should show for SparkDash");
  assert(elements.sparkDashPortHint.style.display === "", "port hint should show for SparkDash");
  assert(elements.connectionHeading.textContent === "SparkDash Connection", "heading should change");

  elements.profileKind.value = "";
  elements.profileKind.trigger("change");
  assert(elements.lhmPort.value === "8085", "switching back to LHM should default port 8085");
  assert(elements.sparkDashUnitRow.style.display === "none", "unit row should hide for LHM");
}

function testSaveSourceProfileIncludesKindAndUnit() {
  const { sandbox, elements, sent } = loadSandbox();
  sandbox.context = "ctx-settings";
  sandbox.uuid = "ctx-pi";
  sandbox.selectedProfileId = "source-1";
  elements.profileName.value = "unit-a";
  elements.lhmHost.value = "10.0.0.8";
  elements.lhmPort.value = "5555";
  elements.profileKind.value = "sparkdash";
  elements.sparkDashUnit.value = "unit-a";

  sandbox.saveSourceProfile();

  const msg = sent.find((m) => m.event === "sendToPlugin" && m.payload && m.payload.setSourceProfile);
  assert(msg, "setSourceProfile missing");
  const sp = msg.payload.setSourceProfile;
  assert(sp.kind === "sparkdash", "kind should be sparkdash");
  assert(sp.sparkId === "unit-a", "sparkId should be saved");
  assert(sp.host === "10.0.0.8" && sp.port === 5555, "host/port should be saved");
}

function testSparkDashHttpsUrlNormalizesTo443() {
  const { sandbox, elements, sent } = loadSandbox();
  sandbox.context = "ctx-settings";
  sandbox.uuid = "ctx-pi";
  sandbox.selectedProfileId = "source-1";
  elements.profileName.value = "unit-a";
  elements.profileKind.value = "sparkdash";
  elements.lhmHost.value = "https://sparkdash.example.com/";
  elements.lhmPort.value = "5555";
  elements.sparkDashUnit.value = "unit-a";

  sandbox.saveSourceProfile();

  const msg = sent.find((m) => m.event === "sendToPlugin" && m.payload && m.payload.setSourceProfile);
  assert(msg, "setSourceProfile missing");
  const sp = msg.payload.setSourceProfile;
  assert(sp.host === "sparkdash.example.com", "https URL should strip to hostname");
  assert(sp.port === 443, "pasted https URL should use port 443");
  assert(elements.lhmHost.value === "sparkdash.example.com", "host field should be cleaned");
  assert(elements.lhmPort.value === "443", "port field should show 443");
}

function testSparkDashNormalizeKeepsDirect5555() {
  const ep = loadSandbox().sandbox.normalizeEndpoint("10.0.0.8", 5555, 5555);
  assert(ep.host === "10.0.0.8" && ep.port === 5555, "direct SparkDash should stay on 5555");
}

function testCLIProxyKindSwitchesPortAndShowsKeyRow() {
  const { sandbox, elements } = loadSandbox();
  elements.lhmPort.value = "8085";
  elements.profileKind.value = "cliproxy";
  elements.profileKind.trigger("change");

  assert(elements.lhmPort.value === "8317", "switching to CLI Proxy should default port 8317");
  assert(elements.cliProxyKeyRow.style.display === "", "key row should show for CLI Proxy");
  assert(elements.cliProxyPortHint.style.display === "", "port hint should show for CLI Proxy");
  assert(elements.sparkDashUnitRow.style.display === "none", "unit row should hide for CLI Proxy");
  assert(elements.connectionHeading.textContent === "CLI Proxy Connection", "heading should change");

  elements.profileKind.value = "sparkdash";
  elements.profileKind.trigger("change");
  assert(elements.lhmPort.value === "5555", "switching to SparkDash should default port 5555");
  assert(elements.cliProxyKeyRow.style.display === "none", "key row should hide for SparkDash");
}

function testSaveSourceProfileIncludesManagementKey() {
  const { sandbox, elements, sent } = loadSandbox();
  sandbox.context = "ctx-settings";
  sandbox.uuid = "ctx-pi";
  sandbox.selectedProfileId = "source-1";
  elements.profileName.value = "lab proxy";
  elements.lhmHost.value = "192.168.1.133";
  elements.lhmPort.value = "8317";
  elements.profileKind.value = "cliproxy";
  elements.cliProxyManagementKey.value = "mgmt-secret";

  sandbox.saveSourceProfile();

  const msg = sent.find((m) => m.event === "sendToPlugin" && m.payload && m.payload.setSourceProfile);
  assert(msg, "setSourceProfile missing");
  const sp = msg.payload.setSourceProfile;
  assert(sp.kind === "cliproxy", "kind should be cliproxy");
  assert(sp.managementKey === "mgmt-secret", "managementKey should be saved");
  assert(sp.host === "192.168.1.133" && sp.port === 8317, "host/port should be saved");
}

function testSaveSourceProfileClearsKindFields() {
  const { sandbox, elements, sent } = loadSandbox();
  sandbox.context = "ctx-settings";
  sandbox.uuid = "ctx-pi";
  sandbox.selectedProfileId = "source-1";
  elements.profileName.value = "lhm";
  elements.lhmHost.value = "10.0.0.8";
  elements.lhmPort.value = "8085";
  elements.profileKind.value = "";
  elements.sparkDashUnit.value = "unit-a";
  elements.cliProxyManagementKey.value = "leftover";

  sandbox.saveSourceProfile();

  const msg = sent.find((m) => m.event === "sendToPlugin" && m.payload && m.payload.setSourceProfile);
  const sp = msg.payload.setSourceProfile;
  assert(sp.kind === "", "kind should clear for LHM");
  assert(sp.sparkId === "", "sparkId should clear for LHM");
  assert(sp.managementKey === "", "managementKey should clear for LHM");
}

function testCLIProxyAccountsRequestSendsKey() {
  const { sandbox, elements, sent } = loadSandbox();
  sandbox.context = "ctx-settings";
  sandbox.uuid = "ctx-pi";
  elements.lhmHost.value = "https://cliproxy.example.com/";
  elements.lhmPort.value = "8317";
  elements.cliProxyManagementKey.value = "mgmt-secret";

  sandbox.requestCLIProxyAccounts();

  const msg = sent.find((m) => m.event === "sendToPlugin" && m.payload && m.payload.listCLIProxyAccounts);
  assert(msg, "listCLIProxyAccounts missing");
  const req = msg.payload.listCLIProxyAccounts;
  assert(req.host === "cliproxy.example.com", "https URL should strip to hostname");
  assert(req.port === 443, "pasted https URL should use port 443");
  assert(req.managementKey === "mgmt-secret", "management key should be sent");
}

function testCLIProxyAccountsReplyUpdatesStatus() {
  const ws = {
    readyState: 1,
    send() {},
    onopen: null,
    onmessage: null,
  };
  const { sandbox, elements } = loadSandbox({ mockSocket: ws });
  sandbox.connectElgatoStreamDeckSocket("12345", "uuid-x", "registerPropertyInspector", "{}", JSON.stringify({
    action: "com.moeilijk.lhm.settings",
    context: "ctx-x",
  }));
  ws.onmessage({
    data: JSON.stringify({
      event: "sendToPropertyInspector",
      payload: {
        cliProxyAccounts: [
          { name: "a.json", provider: "claude", status: "ready" },
          { name: "b.json", provider: "codex", status: "ready" },
          { name: "c.json", provider: "codex", status: "cooldown", unavailable: true },
        ],
      },
    }),
  });
  assert(elements.cliProxyAccountsStatus.textContent === "3 accounts (2 ready)",
    "status should count accounts: " + elements.cliProxyAccountsStatus.textContent);
}

function testSparkDashUnitsPayloadFillsDropdown() {
  const { sandbox, elements } = loadSandbox();
  sandbox.sourceProfiles = [
    { id: "source-1", name: "unit-a", host: "10.0.0.8", port: 5555, kind: "sparkdash", sparkId: "unit-a" },
  ];
  sandbox.selectedProfileId = "source-1";
  sandbox.sparkDashUnits = [
    { id: "gpu-host", name: "gpu-host" },
    { id: "unit-a", name: "unit-a" },
  ];
  sandbox.rebuildSparkDashUnitDropdown("unit-a");
  elements.sparkDashUnitStatus.textContent = sandbox.sparkDashUnits.length + " units";

  const values = elements.sparkDashUnit.children.map((c) => c.value);
  assert(values.indexOf("unit-a") !== -1, "unit-a should be an option");
  assert(elements.sparkDashUnitStatus.textContent === "2 units", "status should count units");
}

function main() {
  testShowLabelPayload();
  testContextFanout();
  testPollIntervalEvents();
  testDidReceiveSettingsAppliesUi();
  testMalformedInputsDoNotCrash();
  testPollingFallbackSave();
  testStatusHeartbeatIsLightweight();
  testFocusedProfileInputIsNotOverwritten();
  testAddGlobalThresholdButtonSendsCommand();
  testGlobalThresholdWithoutEnabledRendersOpen();
  testSparkDashKindSwitchesPortAndShowsUnitRow();
  testSaveSourceProfileIncludesKindAndUnit();
  testSparkDashHttpsUrlNormalizesTo443();
  testSparkDashNormalizeKeepsDirect5555();
  testSparkDashUnitsPayloadFillsDropdown();
  testCLIProxyKindSwitchesPortAndShowsKeyRow();
  testSaveSourceProfileIncludesManagementKey();
  testSaveSourceProfileClearsKindFields();
  testCLIProxyAccountsRequestSendsKey();
  testCLIProxyAccountsReplyUpdatesStatus();
  process.stdout.write("settings-pi tests ok (20 cases)\n");
}

main();
