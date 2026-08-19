// Settings Property Inspector for the Settings tile
var websocket = null,
  uuid = null,
  action = "com.moeilijk.lhm.settings",
  actionInfo = {},
  inInfo = {},
  context = null,
  appearanceSignature = null,
  appearancePollTimer = null,
  statusPollTimer = null,
  uiBound = false;
var allowedIntervals = [250, 500, 1000, 2000, 5000, 10000];
var sourceProfiles = [];
var selectedProfileId = "";
var defaultProfileId = "";
var globalThresholds = [];
var globalThresholdAdvancedOpen = {};
var sparkDashUnits = [];
var sparkDashUnitsHostPort = "";

function parseJSONOrEmpty(raw) {
  if (!raw || typeof raw !== "string") {
    return {};
  }
  try {
    return JSON.parse(raw);
  } catch (_err) {
    return {};
  }
}

function byId(id) {
  return document.getElementById(id);
}

function normalizeInterval(value) {
  var v = parseInt(value, 10);
  if (isNaN(v)) {
    return 1000;
  }
  for (var i = 0; i < allowedIntervals.length; i++) {
    if (allowedIntervals[i] === v) {
      return v;
    }
  }
  // Map unknown values to the nearest supported option.
  var nearest = allowedIntervals[0];
  var nearestDiff = Math.abs(v - nearest);
  for (var j = 1; j < allowedIntervals.length; j++) {
    var diff = Math.abs(v - allowedIntervals[j]);
    if (diff < nearestDiff) {
      nearest = allowedIntervals[j];
      nearestDiff = diff;
    }
  }
  return nearest;
}

function sdkContext() {
  // Property Inspector must use its own registration UUID as context.
  // Using the action context causes Stream Deck to reject messages as "wrong context".
  return uuid || context;
}

function normalizeHex(value, fallback) {
  if (typeof value !== "string") {
    return fallback;
  }
  var v = value.trim().toLowerCase();
  if (!/^#[0-9a-f]{6}$/.test(v)) {
    return fallback;
  }
  return v;
}

function readTileSettingsFromUI() {
  var bg = byId("tileBackground");
  var text = byId("tileTextColor");
  var label = byId("showLabel");
  if (!bg || !text || !label) {
    return {
      tileBackground: "#000000",
      tileTextColor: "#ffffff",
      showLabel: true
    };
  }
  return {
    tileBackground: normalizeHex(bg.value, "#000000"),
    tileTextColor: normalizeHex(text.value, "#ffffff"),
    showLabel: label.checked === true
  };
}

function tileSettingsSignature(settings) {
  return [
    settings.tileBackground,
    settings.tileTextColor,
    settings.showLabel ? "1" : "0"
  ].join("|");
}

function applyTileSettingsToUI(settings) {
  var bgEl = byId("tileBackground");
  var textEl = byId("tileTextColor");
  var showEl = byId("showLabel");
  if (!bgEl || !textEl || !showEl) {
    return;
  }

  var bg = normalizeHex(settings.tileBackground, "#000000");
  var text = normalizeHex(settings.tileTextColor, "#ffffff");
  var show = settings.showLabel !== undefined ? settings.showLabel === true : true;

  if (bgEl.value !== bg) {
    bgEl.value = bg;
  }
  if (textEl.value !== text) {
    textEl.value = text;
  }
  if (showEl.checked !== show) {
    showEl.checked = show;
  }

  appearanceSignature = tileSettingsSignature(readTileSettingsFromUI());
}

function sendJson(payload) {
  if (!websocket || websocket.readyState !== 1) {
    return false;
  }
  websocket.send(JSON.stringify(payload));
  return true;
}

function requestSettingsStatus(fullRefresh) {
  var ctx = sdkContext();
  if (!ctx) {
    return;
  }
  var payload = fullRefresh ? { settingsConnected: true } : { requestSettingsStatus: true };
  sendJson({
    action: action,
    event: "sendToPlugin",
    context: ctx,
    payload: payload
  });
}

function connectElgatoStreamDeckSocket(inPort, inUUID, inRegisterEvent, inInfo, inActionInfo) {
  uuid = inUUID;
  actionInfo = parseJSONOrEmpty(inActionInfo);
  if (actionInfo["action"]) {
    action = actionInfo["action"];
  }
  context = actionInfo["context"];
  if (!context && actionInfo["payload"]) {
    context = actionInfo["payload"]["context"];
  }

  // Fallback to query string values if PI host uses URL params.
  try {
    var qs = new URLSearchParams(window.location.search);
    if (!context) {
      context = qs.get("context");
    }
    if (!action) {
      action = qs.get("action") || "com.moeilijk.lhm.settings";
    }
  } catch (_err) {
    // Ignore URL parsing errors in constrained PI runtimes.
  }
  if (!context) {
    context = uuid;
  }
  inInfo = parseJSONOrEmpty(inInfo);
  bindUIHandlers();
  websocket = new WebSocket("ws://" + ((typeof location !== "undefined" && location.hostname) ? location.hostname : "127.0.0.1") + ":" + inPort);

  websocket.onopen = function () {
    // Register with Stream Deck
    sendJson({
      event: inRegisterEvent,
      uuid: inUUID,
    });

    // Request global settings
    sendJson({
      event: "getGlobalSettings",
      context: inUUID,
    });

    // Request action settings (for tile appearance).
    var ctx = sdkContext();
    if (ctx) {
      sendJson({
        event: "getSettings",
        context: ctx,
      });
    }

    // Notify plugin that settings PI is connected
    requestSettingsStatus(true);
    if (statusPollTimer !== null) {
      window.clearInterval(statusPollTimer);
    }
    statusPollTimer = window.setInterval(function() {
      requestSettingsStatus(false);
    }, 1000);

    // Keep saving resilient even if some WebView color events are missed.
    if (appearancePollTimer !== null) {
      window.clearInterval(appearancePollTimer);
    }
    appearancePollTimer = window.setInterval(function() {
      var current = readTileSettingsFromUI();
      if (tileSettingsSignature(current) !== appearanceSignature) {
        saveTileSettings("poll");
      }
    }, 300);
  };

  websocket.onmessage = function (evt) {
    var jsonObj = parseJSONOrEmpty(evt.data);
    var event = jsonObj["event"];

    // Handle global settings received (poll interval + source profiles + global thresholds)
    if (event === "didReceiveGlobalSettings") {
      var settings = {};
      if (jsonObj.payload && jsonObj.payload.settings) {
        settings = jsonObj.payload.settings;
      }
      var interval = normalizeInterval(settings.pollInterval || 1000);
      var pollEl = byId("pollInterval");
      var rateEl = byId("currentRate");
      if (pollEl) {
        pollEl.value = interval;
      }
      if (rateEl) {
        rateEl.textContent = interval + "ms";
      }
      // Source profiles
      if (Array.isArray(settings.sourceProfiles)) {
        sourceProfiles = settings.sourceProfiles;
        defaultProfileId = settings.defaultSourceProfileId || "";
        if (!selectedProfileId) {
          selectedProfileId = defaultProfileId;
        }
        rebuildProfileDropdowns();
        applySelectedProfileToUI();
      }
      if (Array.isArray(settings.globalThresholds)) {
        globalThresholds = settings.globalThresholds;
        renderGlobalThresholds(globalThresholds);
      }
    }

    // Handle action settings received (tile appearance)
    if (event === "didReceiveSettings") {
      var settings = {};
      if (jsonObj.payload && jsonObj.payload.settings) {
        settings = jsonObj.payload.settings;
      }
      applyTileSettingsToUI(settings);
    }

    // Handle status updates from plugin
    if (event === "sendToPropertyInspector") {
      var payload = jsonObj.payload || {};
      if (Array.isArray(payload.globalThresholds)) {
        globalThresholds = payload.globalThresholds;
        renderGlobalThresholds(globalThresholds);
      }
      if (payload.connectionStatus !== undefined) {
        var statusEl = byId("connectionStatus");
        if (statusEl) {
          statusEl.textContent = payload.connectionStatus;
          statusEl.style.color = payload.connectionStatus === "Connected" ? "#4a4" : "#a44";
        }
      }
      if (payload.currentRate !== undefined) {
        var currentRateEl = byId("currentRate");
        if (currentRateEl) {
          currentRateEl.textContent = payload.currentRate + "ms";
        }
      }
      if (Array.isArray(payload.sourceProfiles)) {
        sourceProfiles = payload.sourceProfiles;
        defaultProfileId = payload.defaultSourceProfileId || "";
        if (payload.selectedSourceProfileId) {
          selectedProfileId = payload.selectedSourceProfileId;
        }
        rebuildProfileDropdowns();
        applySelectedProfileToUI();
      }
      if (payload.sparkDashUnits !== undefined) {
        sparkDashUnits = Array.isArray(payload.sparkDashUnits) ? payload.sparkDashUnits : [];
        var statusEl = byId("sparkDashUnitStatus");
        if (statusEl) {
          statusEl.textContent = payload.sparkDashUnitsError
            ? payload.sparkDashUnitsError
            : (sparkDashUnits.length ? sparkDashUnits.length + " units" : "No units");
        }
        var current = currentProfile();
        rebuildSparkDashUnitDropdown(current ? (current.sparkId || "") : "");
      }
    }
  };

  websocket.onclose = function () {
    if (appearancePollTimer !== null) {
      window.clearInterval(appearancePollTimer);
      appearancePollTimer = null;
    }
    if (statusPollTimer !== null) {
      window.clearInterval(statusPollTimer);
      statusPollTimer = null;
    }
  };
}

// Save tile appearance settings
function saveTileSettings(reason) {
  if (!websocket || websocket.readyState !== 1) {
    return;
  }
  var ctx = sdkContext();
  if (!ctx) {
    return;
  }

  var settings = readTileSettingsFromUI();
  var sig = tileSettingsSignature(settings);
  if (reason !== "force" && sig === appearanceSignature) {
    return;
  }
  appearanceSignature = sig;

  // Persist action settings.
  sendJson({
    event: "setSettings",
    context: ctx,
    payload: settings
  });

  // Trigger immediate redraw path.
  sendJson({
    action: action,
    event: "sendToPlugin",
    context: ctx,
    payload: {
      updateTileAppearance: settings
    }
  });
}

function scheduleTileSettingsSave() {
  window.setTimeout(function() {
    saveTileSettings("ui");
  }, 0);
}

// Expose handlers for inline PI control attributes.
window.saveTileSettings = saveTileSettings;
window.scheduleTileSettingsSave = scheduleTileSettingsSave;
window.addSourceProfile = addSourceProfile;
window.deleteSourceProfile = deleteSourceProfile;
window.refreshSparkDashUnits = refreshSparkDashUnits;
window.saveSourceProfile = saveSourceProfile;

function rebuildProfileDropdowns() {
  var sel = byId("sourceProfileSelect");
  var def = byId("defaultProfileSelect");
  if (!sel || !def) return;
  sel.innerHTML = "";
  def.innerHTML = "";
  for (var i = 0; i < sourceProfiles.length; i++) {
    var p = sourceProfiles[i];
    var o1 = document.createElement("option");
    o1.value = p.id;
    o1.textContent = p.name || p.id;
    if (p.id === selectedProfileId) o1.selected = true;
    sel.appendChild(o1);
    var o2 = document.createElement("option");
    o2.value = p.id;
    o2.textContent = p.name || p.id;
    if (p.id === defaultProfileId) o2.selected = true;
    def.appendChild(o2);
  }
  var deleteBtn = byId("deleteProfileBtn");
  if (deleteBtn) {
    deleteBtn.disabled = sourceProfiles.length <= 1 || selectedProfileId === defaultProfileId;
  }
}

function isActivelyEditing(el) {
  return !!el && document.activeElement === el;
}

function applyInputValue(el, value) {
  if (!el || isActivelyEditing(el)) {
    return;
  }
  var next = value === undefined || value === null ? "" : String(value);
  if (el.value !== next) {
    el.value = next;
  }
}

function currentProfile() {
  for (var i = 0; i < sourceProfiles.length; i++) {
    if (sourceProfiles[i].id === selectedProfileId) {
      return sourceProfiles[i];
    }
  }
  return null;
}

function profileKindValue(profile) {
  if (!profile) return "";
  return profile.kind === "sparkdash" ? "sparkdash" : "";
}

function defaultPortForKind(kind) {
  return kind === "sparkdash" ? 5555 : 8085;
}

function normalizeSparkDashEndpoint(host, port) {
  host = (host || "").trim();
  if (!host) host = "127.0.0.1";
  if (typeof port !== "number" || isNaN(port) || port < 1 || port > 65535) {
    port = 5555;
  }
  if (/^https?:\/\//i.test(host)) {
    try {
      var u = new URL(host);
      host = u.hostname || host;
      if (u.port) {
        port = parseInt(u.port, 10);
      } else if (u.protocol === "https:" && port === 5555) {
        port = 443;
      }
    } catch (_err) {
      var https = /^https:\/\//i.test(host);
      host = host.replace(/^https?:\/\//i, "").replace(/\/.*$/, "");
      if (https && port === 5555) port = 443;
    }
  }
  return { host: host, port: port };
}

function updateKindUI() {
  var kindEl = byId("profileKind");
  var kind = kindEl ? kindEl.value : "";
  var row = byId("sparkDashUnitRow");
  if (row) {
    row.style.display = kind === "sparkdash" ? "" : "none";
  }
  var hint = byId("sparkDashPortHint");
  if (hint) {
    hint.style.display = kind === "sparkdash" ? "" : "none";
  }
  var heading = byId("connectionHeading");
  if (heading) {
    heading.textContent = kind === "sparkdash" ? "SparkDash Connection" : "LHM Connection";
  }
  var portEl = byId("lhmPort");
  if (portEl && !isActivelyEditing(portEl)) {
    portEl.placeholder = String(defaultPortForKind(kind));
  }
}

function rebuildSparkDashUnitDropdown(selectedId) {
  var sel = byId("sparkDashUnit");
  if (!sel) return;
  sel.innerHTML = "";
  var placeholder = document.createElement("option");
  placeholder.value = "";
  placeholder.textContent = sparkDashUnits.length ? "Select unit…" : "Refresh to load units";
  if (!selectedId) placeholder.selected = true;
  sel.appendChild(placeholder);
  var seen = false;
  for (var i = 0; i < sparkDashUnits.length; i++) {
    var u = sparkDashUnits[i];
    var opt = document.createElement("option");
    opt.value = u.id;
    opt.textContent = u.name ? (u.name + " (" + u.id + ")") : u.id;
    if (u.id === selectedId) {
      opt.selected = true;
      seen = true;
    }
    sel.appendChild(opt);
  }
  if (selectedId && !seen) {
    var missing = document.createElement("option");
    missing.value = selectedId;
    missing.textContent = selectedId + " (not in list)";
    missing.selected = true;
    sel.appendChild(missing);
  }
}

function requestSparkDashUnits() {
  var hostEl = byId("lhmHost");
  var portEl = byId("lhmPort");
  var host = hostEl ? hostEl.value.trim() : "";
  var port = portEl ? parseInt(portEl.value, 10) : 5555;
  var ep = normalizeSparkDashEndpoint(host, port);
  host = ep.host;
  port = ep.port;
  sparkDashUnitsHostPort = host + ":" + port;
  var statusEl = byId("sparkDashUnitStatus");
  if (statusEl) statusEl.textContent = "Loading…";
  sendJson({
    action: action,
    event: "sendToPlugin",
    context: sdkContext(),
    payload: { listSparkDashUnits: { host: host, port: port } }
  });
}

function refreshSparkDashUnits() {
  sparkDashUnitsHostPort = "";
  requestSparkDashUnits();
}

function applySelectedProfileToUI() {
  for (var i = 0; i < sourceProfiles.length; i++) {
    if (sourceProfiles[i].id === selectedProfileId) {
      var p = sourceProfiles[i];
      var kind = profileKindValue(p);
      var nameEl = byId("profileName");
      var hostEl = byId("lhmHost");
      var portEl = byId("lhmPort");
      var kindEl = byId("profileKind");
      var unitEl = byId("sparkDashUnit");
      var iconEl = byId("profileIcon");
      var accentEl = byId("profileAccent");
      applyInputValue(nameEl, p.name || "");
      applyInputValue(hostEl, p.host || "127.0.0.1");
      applyInputValue(portEl, p.port || defaultPortForKind(kind));
      applyInputValue(kindEl, kind);
      applyInputValue(iconEl, p.icon || "server");
      applyInputValue(accentEl, p.accent || defaultAccentFor(p.icon || "server"));
      updateKindUI();
      rebuildSparkDashUnitDropdown(p.sparkId || "");
      if (kind === "sparkdash" && hostEl && portEl && !isActivelyEditing(hostEl) && !isActivelyEditing(portEl)) {
        var key = (hostEl.value || "") + ":" + (portEl.value || "");
        if (key !== sparkDashUnitsHostPort) {
          requestSparkDashUnits();
        }
      }
      if (unitEl && p.sparkId && !isActivelyEditing(unitEl)) {
        unitEl.value = p.sparkId;
      }
      return;
    }
  }
}

function addSourceProfile() {
  sendJson({
    action: action,
    event: "sendToPlugin",
    context: sdkContext(),
    payload: { addSourceProfile: true }
  });
}

function deleteSourceProfile() {
  if (!selectedProfileId || selectedProfileId === defaultProfileId) return;
  sendJson({
    action: action,
    event: "sendToPlugin",
    context: sdkContext(),
    payload: { deleteSourceProfile: selectedProfileId }
  });
}

function saveSourceProfile() {
  if (!selectedProfileId) return;
  var nameEl = byId("profileName");
  var hostEl = byId("lhmHost");
  var portEl = byId("lhmPort");
  var kindEl = byId("profileKind");
  var unitEl = byId("sparkDashUnit");
  var iconEl = byId("profileIcon");
  var accentEl = byId("profileAccent");
  var kind = kindEl ? kindEl.value : "";
  var name = nameEl ? nameEl.value.trim() : "";
  var host = hostEl ? hostEl.value.trim() : "127.0.0.1";
  var port = portEl ? parseInt(portEl.value, 10) : defaultPortForKind(kind);
  var sparkId = unitEl ? unitEl.value : "";
  var icon = iconEl ? iconEl.value : "server";
  var accent = accentEl ? accentEl.value : "";
  if (!name) name = "Source";
  if (!host) host = "127.0.0.1";
  if (isNaN(port) || port < 1 || port > 65535) port = defaultPortForKind(kind);
  if (kind === "sparkdash") {
    var ep = normalizeSparkDashEndpoint(host, port);
    host = ep.host;
    port = ep.port;
    if (hostEl && hostEl.value !== host) hostEl.value = host;
    if (portEl && String(portEl.value) !== String(port)) portEl.value = String(port);
  } else {
    kind = "";
    sparkId = "";
  }
  sendJson({
    action: action,
    event: "sendToPlugin",
    context: sdkContext(),
    payload: { setSourceProfile: { id: selectedProfileId, name: name, host: host, port: port, kind: kind, sparkId: sparkId, icon: icon, accent: accent } }
  });
  if (kind === "sparkdash") {
    var key = host + ":" + port;
    if (key !== sparkDashUnitsHostPort) {
      requestSparkDashUnits();
    }
  }
}

function bindUIHandlers() {
  if (uiBound) {
    return;
  }

  var pollEl = byId("pollInterval");
  var bgEl = byId("tileBackground");
  var textEl = byId("tileTextColor");
  var showEl = byId("showLabel");
  if (!pollEl || !bgEl || !textEl || !showEl) {
    return;
  }

  var profileSelEl = byId("sourceProfileSelect");
  if (profileSelEl) {
    profileSelEl.addEventListener("change", function(e) {
      selectedProfileId = e.target.value;
      applySelectedProfileToUI();
      rebuildProfileDropdowns();
      sendJson({
        action: action,
        event: "sendToPlugin",
        context: sdkContext(),
        payload: { setSelectedSourceProfile: selectedProfileId }
      });
    });
  }

  var defaultSelEl = byId("defaultProfileSelect");
  if (defaultSelEl) {
    defaultSelEl.addEventListener("change", function(e) {
      defaultProfileId = e.target.value;
      sendJson({
        action: action,
        event: "sendToPlugin",
        context: sdkContext(),
        payload: { setDefaultSourceProfile: defaultProfileId }
      });
      rebuildProfileDropdowns();
    });
  }

  var profileIconEl = byId("profileIcon");
  if (profileIconEl) {
    profileIconEl.addEventListener("change", function () {
      // Switching icon retargets the accent to that brand's colour unless the
      // user has already chosen one that differs from the previous default.
      var accentEl = byId("profileAccent");
      if (accentEl) {
        var prev = currentProfileAccent();
        if (!prev || isDefaultAccent(prev)) {
          accentEl.value = defaultAccentFor(profileIconEl.value);
        }
      }
      saveSourceProfile();
    });
  }
  var profileAccentEl = byId("profileAccent");
  if (profileAccentEl) {
    profileAccentEl.addEventListener("change", saveSourceProfile);
  }
  var profileNameEl = byId("profileName");
  if (profileNameEl) {
    profileNameEl.addEventListener("change", saveSourceProfile);
  }

  var hostEl = byId("lhmHost");
  var portEl = byId("lhmPort");
  if (hostEl) {
    hostEl.addEventListener("change", saveSourceProfile);
  }
  if (portEl) {
    portEl.addEventListener("change", saveSourceProfile);
  }

  var kindEl = byId("profileKind");
  if (kindEl) {
    kindEl.addEventListener("change", function () {
      var kind = kindEl.value;
      var portInput = byId("lhmPort");
      if (portInput) {
        var current = parseInt(portInput.value, 10);
        var otherDefault = kind === "sparkdash" ? 8085 : 5555;
        if (!portInput.value || current === otherDefault) {
          portInput.value = String(defaultPortForKind(kind));
        }
      }
      updateKindUI();
      if (kind === "sparkdash") {
        sparkDashUnitsHostPort = "";
        requestSparkDashUnits();
      }
      saveSourceProfile();
    });
  }
  var unitEl = byId("sparkDashUnit");
  if (unitEl) {
    unitEl.addEventListener("change", function () {
      var nameEl = byId("profileName");
      var name = nameEl ? nameEl.value.trim() : "";
      if (nameEl && (!name || name === "New Source" || name === "New Profile")) {
        var opt = unitEl.options[unitEl.selectedIndex];
        if (opt && opt.value) {
          var label = opt.textContent || opt.value;
          var cut = label.lastIndexOf(" (");
          if (cut > 0) label = label.slice(0, cut);
          nameEl.value = label;
        }
      }
      saveSourceProfile();
    });
  }

  pollEl.addEventListener("change", function(e) {
    if (!websocket || websocket.readyState !== 1) {
      return;
    }
    var interval = normalizeInterval(e.target.value);
    e.target.value = interval;

    // Route through the plugin rather than calling setGlobalSettings here:
    // Stream Deck replaces the stored object wholesale, so writing just this
    // field would drop every source profile. The plugin owns the full state.
    sendJson({
      action: action,
      event: "sendToPlugin",
      context: sdkContext(),
      payload: { setPollInterval: interval }
    });

    var rateEl = byId("currentRate");
    if (rateEl) {
      rateEl.textContent = interval + "ms";
    }

    sendJson({
      action: action,
      event: "sendToPlugin",
      context: sdkContext(),
      payload: {
        setPollInterval: interval
      }
    });
  });

  bgEl.addEventListener("change", scheduleTileSettingsSave);
  bgEl.addEventListener("input", scheduleTileSettingsSave);
  textEl.addEventListener("change", scheduleTileSettingsSave);
  textEl.addEventListener("input", scheduleTileSettingsSave);
  showEl.addEventListener("change", scheduleTileSettingsSave);
  showEl.addEventListener("input", scheduleTileSettingsSave);
  showEl.addEventListener("click", scheduleTileSettingsSave);

  bindGlobalThresholdControls();
  appearanceSignature = tileSettingsSignature(readTileSettingsFromUI());
  uiBound = true;
}

if (document.readyState === "loading") {
  document.addEventListener("DOMContentLoaded", bindUIHandlers);
} else {
  bindUIHandlers();
}

// --- Global threshold library ---

function sendGlobalThresholdUpdate(id, field, value, checked) {
  var upd = { id: id, field: field, value: String(value) };
  if (typeof checked === "boolean") upd.checked = checked;
  sendJson({
    action: action,
    event: "sendToPlugin",
    context: sdkContext(),
    payload: { updateGlobalThreshold: upd }
  });
}

function renderGlobalThresholds(thresholds) {
  var container = byId("globalThresholdsContainer");
  if (!container) return;
  var list = thresholds || [];

  var existingItems = container.querySelectorAll(".threshold-item");
  var existingIds = Array.prototype.map.call(existingItems, function(el) { return el.dataset.thresholdId; });
  var incomingIds = list.map(function(t) { return t.id; });

  if (JSON.stringify(existingIds) !== JSON.stringify(incomingIds)) {
    container.innerHTML = "";
    list.forEach(function(t) { container.appendChild(createGlobalThresholdElement(t)); });
    return;
  }

  var active = document.activeElement;
  list.forEach(function(t) {
    var item = container.querySelector('.threshold-item[data-threshold-id="' + t.id + '"]');
    if (!item || (active && item.contains(active))) return;
    applyInputValue(item.querySelector(".threshold-name"), t.name || "");
    applyInputValue(item.querySelector(".threshold-text"), t.text || "");
    applyInputValue(item.querySelector(".threshold-value"), t.value != null ? t.value : "");
    applyInputValue(item.querySelector(".threshold-hysteresis"), t.hysteresis != null ? t.hysteresis : "");
    applyInputValue(item.querySelector(".threshold-dwell"), t.dwellMs != null ? t.dwellMs : "");
    applyInputValue(item.querySelector(".threshold-cooldown"), t.cooldownMs != null ? t.cooldownMs : "");
  });
}

function createGlobalThresholdElement(threshold) {
  var template = document.querySelector("#globalThresholdTemplate");
  if (!template) return document.createDocumentFragment();
  var clone = template.content.cloneNode(true);
  var wrapper = clone.querySelector(".threshold-item");
  wrapper.dataset.thresholdId = threshold.id;

  var nameInput = clone.querySelector(".threshold-name");
  nameInput.value = threshold.name || "";

  var textInput = clone.querySelector(".threshold-text");
  textInput.value = threshold.text || "";

  var readingTypeSelect = clone.querySelector(".threshold-reading-type");
  if (readingTypeSelect) readingTypeSelect.value = threshold.readingType || "";

  var operatorSelect = clone.querySelector(".threshold-operator");
  operatorSelect.value = threshold.operator || ">=";

  var valueInput = clone.querySelector(".threshold-value");
  valueInput.value = threshold.value != null ? threshold.value : "";

  var hysteresisInput = clone.querySelector(".threshold-hysteresis");
  hysteresisInput.value = threshold.hysteresis != null ? threshold.hysteresis : "";

  var dwellInput = clone.querySelector(".threshold-dwell");
  dwellInput.value = threshold.dwellMs != null ? threshold.dwellMs : "";

  var cooldownInput = clone.querySelector(".threshold-cooldown");
  cooldownInput.value = threshold.cooldownMs != null ? threshold.cooldownMs : "";

  var bgInput = clone.querySelector(".threshold-bg");
  bgInput.value = threshold.backgroundColor || "#333300";
  var fgInput = clone.querySelector(".threshold-fg");
  fgInput.value = threshold.foregroundColor || "#999900";
  var hlInput = clone.querySelector(".threshold-hl");
  hlInput.value = threshold.highlightColor || "#ffff00";
  var vtInput = clone.querySelector(".threshold-vt");
  vtInput.value = threshold.valueTextColor || "#ffff00";
  var tcInput = clone.querySelector(".threshold-tc");
  if (tcInput) tcInput.value = threshold.textColor || "#ffffff";

  var toggleBtn = clone.querySelector(".threshold-toggle");
  var settingsDiv = clone.querySelector(".threshold-settings");
  var stickyBtn = clone.querySelector(".threshold-sticky-toggle");
  var advancedToggleBtn = clone.querySelector(".threshold-advanced-toggle");
  var advancedPanel = clone.querySelector(".threshold-advanced-panel");

  var isEnabled = threshold.enabled !== false;
  var isSticky = threshold.sticky === true;
  var isAdvancedOpen = globalThresholdAdvancedOpen[threshold.id] === true;
  var thresholdId = threshold.id;

  function updateToggleState() {
    toggleBtn.textContent = isEnabled ? "on" : "off";
    toggleBtn.style.background = isEnabled ? "#4a4" : "#a44";
    settingsDiv.style.display = isEnabled ? "block" : "none";
  }
  function updateStickyState() {
    if (!stickyBtn) return;
    stickyBtn.textContent = isSticky ? "on" : "off";
    stickyBtn.style.background = isSticky ? "#4a4" : "#a44";
    stickyBtn.style.color = "#fff";
  }
  function updateAdvancedState() {
    if (!advancedToggleBtn || !advancedPanel) return;
    advancedToggleBtn.textContent = isAdvancedOpen ? "Advanced ▼" : "Advanced ▶";
    advancedPanel.style.display = isAdvancedOpen ? "block" : "none";
  }
  updateToggleState();
  updateStickyState();
  updateAdvancedState();

  toggleBtn.addEventListener("click", function() {
    isEnabled = !isEnabled;
    updateToggleState();
    sendGlobalThresholdUpdate(thresholdId, "thresholdEnabled", isEnabled ? "true" : "false", isEnabled);
  });

  var nameTimeout;
  nameInput.addEventListener("input", function(e) {
    clearTimeout(nameTimeout);
    nameTimeout = setTimeout(function() {
      sendGlobalThresholdUpdate(thresholdId, "thresholdName", e.target.value);
    }, 300);
  });

  var textTimeout;
  textInput.addEventListener("input", function(e) {
    clearTimeout(textTimeout);
    textTimeout = setTimeout(function() {
      sendGlobalThresholdUpdate(thresholdId, "thresholdText", e.target.value);
    }, 300);
  });

  if (readingTypeSelect) {
    readingTypeSelect.addEventListener("change", function(e) {
      sendGlobalThresholdUpdate(thresholdId, "thresholdReadingType", e.target.value);
    });
  }

  operatorSelect.addEventListener("change", function(e) {
    sendGlobalThresholdUpdate(thresholdId, "thresholdOperator", e.target.value);
  });

  var valueTimeout;
  valueInput.addEventListener("input", function(e) {
    clearTimeout(valueTimeout);
    valueTimeout = setTimeout(function() {
      sendGlobalThresholdUpdate(thresholdId, "thresholdValue", e.target.value);
    }, 300);
  });

  var hysteresisTimeout;
  hysteresisInput.addEventListener("input", function(e) {
    clearTimeout(hysteresisTimeout);
    hysteresisTimeout = setTimeout(function() {
      sendGlobalThresholdUpdate(thresholdId, "thresholdHysteresis", e.target.value);
    }, 300);
  });

  var dwellTimeout;
  dwellInput.addEventListener("input", function(e) {
    clearTimeout(dwellTimeout);
    dwellTimeout = setTimeout(function() {
      sendGlobalThresholdUpdate(thresholdId, "thresholdDwellMs", e.target.value);
    }, 300);
  });

  var cooldownTimeout;
  cooldownInput.addEventListener("input", function(e) {
    clearTimeout(cooldownTimeout);
    cooldownTimeout = setTimeout(function() {
      sendGlobalThresholdUpdate(thresholdId, "thresholdCooldownMs", e.target.value);
    }, 300);
  });

  if (stickyBtn) {
    stickyBtn.addEventListener("click", function() {
      isSticky = !isSticky;
      updateStickyState();
      sendGlobalThresholdUpdate(thresholdId, "thresholdSticky", isSticky ? "true" : "false", isSticky);
    });
  }

  if (advancedToggleBtn) {
    advancedToggleBtn.addEventListener("click", function() {
      isAdvancedOpen = !isAdvancedOpen;
      globalThresholdAdvancedOpen[thresholdId] = isAdvancedOpen;
      updateAdvancedState();
    });
  }

  bgInput.addEventListener("change", function(e) { sendGlobalThresholdUpdate(thresholdId, "thresholdBackgroundColor", e.target.value); });
  fgInput.addEventListener("change", function(e) { sendGlobalThresholdUpdate(thresholdId, "thresholdForegroundColor", e.target.value); });
  hlInput.addEventListener("change", function(e) { sendGlobalThresholdUpdate(thresholdId, "thresholdHighlightColor", e.target.value); });
  vtInput.addEventListener("change", function(e) { sendGlobalThresholdUpdate(thresholdId, "thresholdValueTextColor", e.target.value); });
  if (tcInput) {
    tcInput.addEventListener("change", function(e) { sendGlobalThresholdUpdate(thresholdId, "thresholdTextColor", e.target.value); });
  }

  var removeBtn = clone.querySelector(".threshold-remove");
  removeBtn.addEventListener("click", function() {
    delete globalThresholdAdvancedOpen[thresholdId];
    sendJson({
      action: action,
      event: "sendToPlugin",
      context: sdkContext(),
      payload: { deleteGlobalThreshold: thresholdId }
    });
    wrapper.remove();
  });

  return clone;
}

function bindGlobalThresholdControls() {
  var addBtn = byId("addGlobalThresholdBtn");
  if (addBtn && !addBtn.dataset.bound) {
    addBtn.dataset.bound = "1";
    addBtn.addEventListener("click", function(e) {
      e.preventDefault();
      e.stopPropagation();
      var nameEl = byId("newGlobalThresholdName");
      var name = nameEl ? nameEl.value.trim() : "";
      sendJson({
        action: action,
        event: "sendToPlugin",
        context: sdkContext(),
        payload: { addGlobalThreshold: name }
      });
      if (nameEl) nameEl.value = "";
    });
  }
}

// Accent defaults mirror defaultAccents in labtile.go.
var ACCENT_DEFAULTS = {
  apple: "#e6e8eb",
  nvidia: "#76b900",
  linux: "#e9952c",
  server: "#6e9eff",
  chip: "#6e9eff",
  none: "#6e9eff"
};

function defaultAccentFor(icon) {
  return ACCENT_DEFAULTS[icon] || ACCENT_DEFAULTS.server;
}

function isDefaultAccent(hex) {
  var v = String(hex || "").toLowerCase();
  for (var k in ACCENT_DEFAULTS) {
    if (ACCENT_DEFAULTS[k] === v) return true;
  }
  return false;
}

function currentProfileAccent() {
  for (var i = 0; i < sourceProfiles.length; i++) {
    if (sourceProfiles[i].id === selectedProfileId) return sourceProfiles[i].accent || "";
  }
  return "";
}
