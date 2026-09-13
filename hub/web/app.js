const I18N = {
  pl: {
    dash: "MAPA", chat: "CZAT", invite: "KARTA", join: "CONT", net: "DIAG",
    identity: "GRACZ", name: "IMIE", fp: "ODCISK", short: "KOD",
    uptime: "CZAS", cap: "POWER", hubs: "STAGE",
    inviteHelp: "POKAZ TEN KOD NA LAN. TO NIE JEST SEED Z CHMURY.",
    noInvite: "BRAK KARTY — TO NIE HUB.",
    copy: "KOPIUJ", copyShort: "KOD",
    joinHelp: "WKLEJ BLOB, KOD 10 ALBO IPv6. SEED NA KONCU.",
    dial: "START", send: "A", press: "NACISNIJ START",
    toPh: "DO KOGO", msgPh: "WIADOMOSC",
    joinPh: "starmesh1:… ALBO IPv6",
    probe: "SONDA", svc: "SELECT", become: "HUB", stop: "STOP",
    noHub: "BRAK HUBA — KOLEJKA",
    peersEmpty: "NIKT NA MAPIE. WPISZ NICK ALBO CZEKAJ.",
    unknownPeer: "NIE MA TAKIEGO. WEZ Z LISTY ALBO CZEKAJ.",
    copied: "OK",
    ipv6Real: "IPv6 GLOBAL",
    ipv6Lab: "LAB --DEV / BRAK IPv6",
    ipv4Pub: "IPv4 PUBLIC",
    ipv4None: "IPv4 OFF",
    udpOk: "UDP :4433 OK",
    udpNo: "UDP :4433 FAIL",
    cgnat: "CGNAT",
    e2e: "BOX ON — HUB WIDZI SZYFROGRAM",
    e2eOff: "BOX OFF",
  },
  en: {
    dash: "MAP", chat: "CHAT", invite: "CARD", join: "CONT", net: "DIAG",
    identity: "1P", name: "NAME", fp: "PRINT", short: "CODE",
    uptime: "TIME", cap: "POWER", hubs: "STAGE",
    inviteHelp: "SHOW THIS CODE ON THE LAN. NOT THE CLOUD SEED.",
    noInvite: "NO CARD — NOT A HUB.",
    copy: "COPY", copyShort: "CODE",
    joinHelp: "PASTE BLOB, 10-CHAR CODE, OR IPv6. SEED LAST.",
    dial: "START", send: "A", press: "PRESS START",
    toPh: "TO WHOM", msgPh: "MESSAGE",
    joinPh: "starmesh1:… OR IPv6",
    probe: "PROBE", svc: "SELECT", become: "HUB", stop: "STOP",
    noHub: "NO HUB — QUEUE",
    peersEmpty: "NOBODY ON THE MAP. TYPE A NICK OR WAIT.",
    unknownPeer: "UNKNOWN. PICK FROM THE LIST OR WAIT.",
    copied: "OK",
    ipv6Real: "IPv6 GLOBAL",
    ipv6Lab: "LAB --DEV / NO IPv6",
    ipv4Pub: "IPv4 PUBLIC",
    ipv4None: "IPv4 OFF",
    udpOk: "UDP :4433 OK",
    udpNo: "UDP :4433 FAIL",
    cgnat: "CGNAT",
    e2e: "BOX ON — HUB SEES CIPHERTEXT",
    e2eOff: "BOX OFF",
  },
};

const token = new URLSearchParams(location.search).get("token") || "";
const headers = () => {
  const h = { "Content-Type": "application/json" };
  if (token) h["X-Starmesh-Token"] = token;
  return h;
};

let lang = localStorage.getItem("starmesh_lang") || "pl";
let state = {};

function t(k) { return (I18N[lang] && I18N[lang][k]) || I18N.en[k] || k; }

function applyI18n() {
  document.documentElement.lang = lang;
  document.querySelectorAll("[data-i18n]").forEach((el) => {
    el.textContent = t(el.getAttribute("data-i18n"));
  });
  document.querySelectorAll("[data-i18n-ph]").forEach((el) => {
    el.placeholder = t(el.getAttribute("data-i18n-ph"));
  });
  document.getElementById("langBtn").textContent = lang === "pl" ? "EN" : "PL";
}

function fmtUp(s) {
  s = Number(s) || 0;
  const m = Math.floor(s / 60);
  if (m > 999) return String(m);
  return String(m).padStart(3, "0");
}

async function api(path, opt) {
  const r = await fetch(path, { ...opt, headers: { ...headers(), ...(opt && opt.headers) } });
  const text = await r.text();
  let j = {};
  try { j = text ? JSON.parse(text) : {}; } catch { j = { error: text }; }
  if (!r.ok && !j.error) j.error = r.statusText;
  return j;
}

function renderBanner(s) {
  const el = document.getElementById("banner");
  const text = s.banner || t("noHub");
  el.textContent = text;
  const waiting = /no hub|brak hub|queued|kolejk/i.test(String(text));
  el.className = "ticker " + (waiting ? "wait" : "ok");
}

function renderDash(s) {
  document.getElementById("roleLine").textContent = s.role || "—";
  document.getElementById("dName").textContent = s.name || "—";
  document.getElementById("dFp").textContent = s.fingerprint || "—";
  document.getElementById("dShort").textContent = s.short || "—";
  document.getElementById("dUp").textContent = fmtUp(s.uptime_s);
  const p = s.probe || {};
  const badges = [];
  if (p.has_ipv6) badges.push(["good", t("ipv6Real")]);
  else badges.push(["warn", t("ipv6Lab")]);
  if (p.claim_ipv4) badges.push(["good", t("ipv4Pub")]);
  else if (p.lab_ipv4) badges.push(["warn", "LAN " + p.lab_ipv4]);
  else badges.push(["", t("ipv4None")]);
  if (p.can_bind_udp) badges.push(["good", t("udpOk")]);
  else badges.push(["warn", t("udpNo")]);
  if (p.behind_cgnat) badges.push(["warn", t("cgnat")]);
  if (p.dev) badges.push(["warn", "--dev"]);
  if (s.chat_e2e !== false) badges.push(["good", "BOX"]);
  document.getElementById("badges").innerHTML = badges.map(([c, l]) => `<span class="badge ${c}">${l}</span>`).join("");
  const hint = document.getElementById("capHint");
  if (hint) hint.textContent = p.reason || "";
  const hubs = s.hubs || [];
  document.getElementById("hubList").innerHTML = hubs.length
    ? hubs.map((h) => `<div><strong>${h.name || h.fingerprint || "?"}</strong>
        ${h.self ? " · this" : ""}<div class="meta">${[h.role, h.ipv6 && ("IPv6 " + h.ipv6), h.ipv4 ? ("IPv4 " + h.ipv4) : "IPv4 —", h.rtt_ms && (h.rtt_ms + " ms")].filter(Boolean).join(" · ")}</div></div>`).join("")
    : `<p class="hint">${t("noHub")}</p>`;
}

function renderChat(s) {
  const led = document.getElementById("e2eLed");
  if (led) {
    led.textContent = s.chat_e2e === false ? t("e2eOff") : t("e2e");
    led.classList.toggle("off", s.chat_e2e === false);
  }
  const peers = s.peers || [];
  const chips = document.getElementById("peerChips");
  chips.innerHTML = peers.length
    ? peers.map((p) => `<button type="button" data-to="${p.name || p.fingerprint}">${p.name || p.fingerprint}</button>`).join("")
    : `<span class="hint">${t("peersEmpty")}</span>`;
  chips.querySelectorAll("button").forEach((b) => {
    b.onclick = () => { document.getElementById("chatTo").value = b.dataset.to; };
  });
  const log = document.getElementById("chatLog");
  const lines = s.messages || [];
  log.innerHTML = lines.map((m) => {
    const cls = m.mine ? "bubble mine" : (m.from === "system" ? "bubble sys" : "bubble");
    const who = m.mine ? "1P" : (m.from || "?");
    const lock = m.e2e !== false ? `<i class="lock" title="NaCl box"></i>` : "";
    return `<div class="${cls}">${lock}<b>${esc(who)}</b> ${esc(m.text)}</div>`;
  }).join("");
  log.scrollTop = log.scrollHeight;
}

function renderInvite(s) {
  const blob = s.invite || "";
  document.getElementById("invShort").textContent = s.short || "—";
  document.getElementById("invBlob").value = blob;
  const img = document.getElementById("qrImg");
  const empty = document.getElementById("qrEmpty");
  if (blob) {
    img.src = "/v1/qr.svg" + (token ? ("?token=" + encodeURIComponent(token)) : "");
    img.hidden = false;
    empty.hidden = true;
  } else {
    img.hidden = true;
    empty.hidden = false;
  }
}

function renderNet(s) {
  const p = s.probe || {};
  const rows = [
    ["IPv6", (p.global_ipv6 || []).join(", ") || "fe80 / brak"],
    ["UDP", p.can_bind_udp ? "OK :" + (p.bind_port || 4433) : (p.bind_error || "nie")],
    ["IPv4", p.public_ipv4 || p.lab_ipv4 || "—"],
    ["CGNAT", p.behind_cgnat ? "tak" : "nie"],
    ["Hub?", p.can_be_hub ? "tak" : "nie"],
  ];
  document.getElementById("probeDL").innerHTML = rows.map(([k, v]) => `<div><dt>${k}</dt><dd>${esc(String(v))}</dd></div>`).join("");
  const ra = (p.ra && p.ra.advice) || "";
  document.getElementById("raAdvice").textContent = ra;
}

function esc(s) {
  return String(s).replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
}

async function refresh() {
  try {
    state = await api("/v1/status");
    renderBanner(state);
    renderDash(state);
    renderChat(state);
    renderInvite(state);
    renderNet(state);
  } catch (e) {
    document.getElementById("banner").textContent = "API offline";
    document.getElementById("banner").className = "ticker wait";
  }
}

function showPage(id) {
  document.querySelectorAll(".dock button").forEach((x) => x.classList.toggle("on", x.dataset.page === id));
  document.querySelectorAll(".page").forEach((p) => p.classList.toggle("active", p.id === "page-" + id));
}

document.querySelectorAll(".dock button").forEach((b) => {
  b.onclick = () => showPage(b.dataset.page);
});
const startPage = new URLSearchParams(location.search).get("page");
if (startPage) showPage(startPage);

document.getElementById("langBtn").onclick = () => {
  lang = lang === "pl" ? "en" : "pl";
  localStorage.setItem("starmesh_lang", lang);
  applyI18n();
  refresh();
};

document.getElementById("chatForm").onsubmit = async (e) => {
  e.preventDefault();
  const to = document.getElementById("chatTo").value.trim();
  const text = document.getElementById("chatText").value.trim();
  const err = document.getElementById("chatErr");
  err.hidden = true;
  if (!text) return;
  const j = await api("/v1/send", { method: "POST", body: JSON.stringify({ to, text }) });
  if (j.error) {
    err.hidden = false;
    err.textContent = j.unknown_peer ? t("unknownPeer") : j.error;
  } else {
    document.getElementById("chatText").value = "";
  }
  await refresh();
};

document.getElementById("joinForm").onsubmit = async (e) => {
  e.preventDefault();
  const raw = document.getElementById("joinRaw").value.trim();
  const msg = document.getElementById("joinMsg");
  if (!raw) return;
  const j = await api("/v1/invite", { method: "POST", body: JSON.stringify({ invite: raw }) });
  msg.textContent = j.error || "OK";
  await refresh();
};

document.getElementById("copyBlob").onclick = async () => {
  await navigator.clipboard.writeText(document.getElementById("invBlob").value);
};
document.getElementById("copyShort").onclick = async () => {
  await navigator.clipboard.writeText(document.getElementById("invShort").textContent);
};

document.getElementById("btnBecome").onclick = async () => {
  const j = await api("/v1/become-hub", { method: "POST" });
  document.getElementById("svcMsg").textContent = j.message || j.error || "OK";
  await refresh();
};
document.getElementById("btnStop").onclick = async () => {
  const j = await api("/v1/stop-hub", { method: "POST" });
  document.getElementById("svcMsg").textContent = j.error || "OK";
};

applyI18n();
refresh();
setInterval(refresh, 2000);
