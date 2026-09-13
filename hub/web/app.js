const I18N = {
  pl: {
    dash: "Pulpit", chat: "Czat", invite: "QR", join: "Dołącz", net: "Sieć",
    identity: "Tożsamość", name: "Nazwa", fp: "Odcisk", short: "Kod 10",
    uptime: "Czas pracy", cap: "Zdolność", hubs: "Huby i sesje",
    inviteHelp: "Pokaż ten QR telefonowi w LAN. To nie jest adres seeda w chmurze.",
    noInvite: "Brak zaproszenia — ten proces nie jest hubem.",
    copy: "Kopiuj blob", copyShort: "Kopiuj kod",
    joinHelp: "Wklej zaproszenie, kod 10-znakowy albo IPv6. Daemon wybiera hub — seed na końcu.",
    dial: "Wybierz / dodaj", send: "Wyślij",
    toPh: "Do (nazwa lub odcisk)", msgPh: "Wiadomość (E2E)",
    joinPh: "starmesh1:… albo IPv6",
    probe: "Sonda", svc: "Usługa", become: "Zostań hubem", stop: "Przestań być hubem",
    noHub: "Brak huba — w kolejce",
    peersEmpty: "Brak peerów. Poczekaj na obecność albo dołącz drugiego spoke.",
    unknownPeer: "Nieznany peer. Wybierz kogoś z listy albo poczekaj na presence.",
    copied: "Skopiowano",
    ipv6Real: "Globalne IPv6",
    ipv6Lab: "Lab --dev (brak globalnego IPv6)",
    ipv4Pub: "Publiczne IPv4",
    ipv4None: "IPv4 nieogłaszane",
    udpOk: "UDP :4433 OK",
    udpNo: "UDP :4433 zajęty/błąd",
    cgnat: "CGNAT",
  },
  en: {
    dash: "Dashboard", chat: "Chat", invite: "QR", join: "Join", net: "Net",
    identity: "Identity", name: "Name", fp: "Fingerprint", short: "Short",
    uptime: "Uptime", cap: "Capability", hubs: "Hubs & sessions",
    inviteHelp: "Show this QR to a phone on the LAN. This is not the cloud seed host.",
    noInvite: "No invite — this process is not a hub.",
    copy: "Copy blob", copyShort: "Copy short",
    joinHelp: "Paste a starmesh1: blob, 10-char code, or IPv6. The daemon dials; seed is last.",
    dial: "Dial / add", send: "Send",
    toPh: "To (name or fingerprint)", msgPh: "Message (E2E)",
    joinPh: "starmesh1:… or IPv6",
    probe: "Probe", svc: "Service", become: "Become hub", stop: "Stop being hub",
    noHub: "No hub — queued",
    peersEmpty: "No peers yet. Wait for presence or join a second spoke.",
    unknownPeer: "Unknown peer. Pick someone from the list or wait for presence.",
    copied: "Copied",
    ipv6Real: "Global IPv6",
    ipv6Lab: "Lab --dev (no global IPv6)",
    ipv4Pub: "Public IPv4",
    ipv4None: "IPv4 not advertised",
    udpOk: "UDP :4433 OK",
    udpNo: "UDP :4433 busy/error",
    cgnat: "CGNAT",
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
  const h = Math.floor(s / 3600), m = Math.floor((s % 3600) / 60), sec = s % 60;
  if (h) return `${h}h ${m}m`;
  if (m) return `${m}m ${sec}s`;
  return `${sec}s`;
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
  el.className = "banner " + (String(text).toLowerCase().includes("no hub") ? "wait" : "ok");
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
  document.getElementById("badges").innerHTML = badges.map(([c, l]) => `<span class="badge ${c}">${l}</span>`).join("");
  document.getElementById("capHint").textContent = p.reason || "";
  const hubs = s.hubs || [];
  document.getElementById("hubList").innerHTML = hubs.length
    ? hubs.map((h) => `<div><strong>${h.name || h.fingerprint || "?"}</strong>
        ${h.self ? " · this" : ""}<div class="meta">${[h.role, h.ipv6 && ("IPv6 " + h.ipv6), h.ipv4 ? ("IPv4 " + h.ipv4) : "IPv4 —", h.rtt_ms && (h.rtt_ms + " ms")].filter(Boolean).join(" · ")}</div></div>`).join("")
    : `<p class="hint">${t("noHub")}</p>`;
}

function renderChat(s) {
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
    return `<div class="${cls}">${m.mine ? "" : `<strong>${m.from}:</strong> `}${esc(m.text)}</div>`;
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
    document.getElementById("banner").className = "banner wait";
  }
}

document.querySelectorAll(".tabs button").forEach((b) => {
  b.onclick = () => {
    document.querySelectorAll(".tabs button").forEach((x) => x.classList.toggle("on", x === b));
    document.querySelectorAll(".page").forEach((p) => p.classList.toggle("active", p.id === "page-" + b.dataset.page));
  };
});

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
