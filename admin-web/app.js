(function () {
  const API_LIST = "/api/stats/play/list";
  const API_TREND = "/api/stats/play/trend";
  const API_STREAMS = "/list/fields/streams";
  const API_PLAY_DOMAIN = "/api/settings/play-domain";
  const API_ENV_DOMAINS = "/api/settings/env-domains";
  const DEFAULT_PLAYER_BASE_URL = "http://127.0.0.1:8080/";
  const PAGE_SIZE = 200;
  const ENV_ORDER = ["local", "dev", "test", "stag", "prod"];

  const SITE_OPTIONS = [
    { value: "studio_3drush", label: "studio_3drush" },
    { value: "studio_gsp2w", label: "studio_gsp2w" },
  ];
  const VIEW_BY_SITE = {
    studio_3drush: ["fwh", "fwv"],
    studio_gsp2w: ["fwv", "fwh"],
  };
  const DEFAULT_VIEW_BY_SITE = {
    studio_3drush: "fwh",
    studio_gsp2w: "fwv",
  };
  const PAGE_COPY = {
    stat: ["StatAPI", "播放埋点、趋势和播放域名配置。"],
    player: ["AbrPlayer", "播放器链接、流梯和 ABR 策略。"],
    pusher: ["Multipusher", "本地推流配置说明，不提供远程控制。"],
    health: ["Health", "服务健康检查入口。"],
  };

  const LADDER = [
    { level: "high", suffix: "", codec: "H.264", bitrate: "2000 kbps", resolution: "1080x1920 / 1920x1080", rule: "bandwidth >= 1500 kbps" },
    { level: "standard", suffix: "_standard", codec: "H.264", bitrate: "1000 kbps", resolution: "720x1280 / 1280x720", rule: "standard HEVC failback" },
    { level: "standard_hevc", suffix: "_standard_hevc", codec: "HEVC", bitrate: "600 kbps", resolution: "720x1280 / 1280x720", rule: "preferred when HEVC is supported" },
    { level: "economic", suffix: "_economic", codec: "H.264", bitrate: "400 kbps", resolution: "360x640 / 640x360", rule: "bandwidth < 500 kbps" },
    { level: "bottom", suffix: "_audio", codec: "AAC", bitrate: "128 kbps", resolution: "audio only", rule: "bandwidth < 200 kbps or zero probe" },
  ];

  const PUSHER_PROFILES = [
    { level: "bottom", template: "{siteName}_audio", encoding: "AAC 128k, audio only", status: "兜底流，播放器冻结最后一帧后只保留音频。" },
    { level: "economic", template: "{siteName}_economic", encoding: "H.264, 400k max 600k, 360x640 portrait / 640x360 landscape, AAC 128k", status: "低带宽视频档。" },
    { level: "standard_hevc", template: "{siteName}_standard_hevc", encoding: "HEVC, 600k max 1000k, 720x1280 portrait / 1280x720 landscape, AAC 128k", status: "standard 质量等级的 HEVC 子流。" },
    { level: "standard", template: "{siteName}_standard", encoding: "H.264, 1000k max 1500k, 720x1280 portrait / 1280x720 landscape, AAC 128k", status: "standard 质量等级的 H.264 回退流。" },
    { level: "high", template: "{siteName}", encoding: "H.264, 2000k max 3000k, 1080x1920 portrait / 1920x1080 landscape, AAC 128k", status: "最高画质流。" },
  ];

  const state = {
    page: 1,
    total: 0,
    pageSize: PAGE_SIZE,
    listItems: [],
    trendItems: [],
    envDomains: [],
  };

  const $ = (id) => document.getElementById(id);
  const el = {
    pageTitle: $("pageTitle"),
    pageSubtitle: $("pageSubtitle"),
    navItems: Array.from(document.querySelectorAll(".nav-item")),
    views: Array.from(document.querySelectorAll(".view")),
    startTime: $("startTime"),
    endTime: $("endTime"),
    streamName: $("streamName"),
    isSucceed: $("isSucceed"),
    cdnName: $("cdnName"),
    agentType: $("agentType"),
    minLoadTime: $("minLoadTime"),
    maxLoadTime: $("maxLoadTime"),
    playDomain: $("playDomain"),
    playDomainStatus: $("playDomainStatus"),
    savePlayDomain: $("savePlayDomain"),
    envDomainRows: $("envDomainRows"),
    envDomainStatus: $("envDomainStatus"),
    refresh: $("refresh"),
    reset: $("reset"),
    metricTotal: $("metricTotal"),
    metricSuccessRate: $("metricSuccessRate"),
    metricAvgLoad: $("metricAvgLoad"),
    metricLagRate: $("metricLagRate"),
    trendStatus: $("trendStatus"),
    trendChart: $("trendChart"),
    resultRows: $("resultRows"),
    prevPage: $("prevPage"),
    nextPage: $("nextPage"),
    pageInfo: $("pageInfo"),
    playerSite: $("playerSite"),
    playerEnv: $("playerEnv"),
    playerView: $("playerView"),
    playerQuality: $("playerQuality"),
    playerBaseUrl: $("playerBaseUrl"),
    playerUrl: $("playerUrl"),
    copyPlayerUrl: $("copyPlayerUrl"),
    openPlayerUrl: $("openPlayerUrl"),
    playerLadderRows: $("playerLadderRows"),
    pusherProfileRows: $("pusherProfileRows"),
    healthCards: $("healthCards"),
    toast: $("toast"),
  };

  function pad(num) {
    return String(num).padStart(2, "0");
  }

  function toLocalInputValue(date) {
    return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`;
  }

  function toApiDateTime(inputValue) {
    return inputValue ? inputValue.replace("T", " ") + ":00" : "";
  }

  function formatDate(value) {
    if (!value) return "-";
    const date = new Date(value);
    return Number.isNaN(date.getTime()) ? String(value) : date.toLocaleString();
  }

  function number(value, digits) {
    const parsed = Number(value);
    if (!Number.isFinite(parsed)) return "--";
    return parsed.toLocaleString(undefined, { maximumFractionDigits: digits == null ? 2 : digits });
  }

  function percent(value) {
    const parsed = Number(value);
    return Number.isFinite(parsed) ? parsed.toFixed(2) + "%" : "--";
  }

  function escapeHtml(value) {
    return String(value == null ? "" : value)
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;")
      .replace(/'/g, "&#039;");
  }

  function showToast(message) {
    el.toast.textContent = message;
    el.toast.hidden = false;
    window.clearTimeout(showToast.timer);
    showToast.timer = window.setTimeout(() => {
      el.toast.hidden = true;
    }, 3200);
  }

  function headers() {
    return { "Content-Type": "application/json" };
  }

  async function requestJSON(url, options) {
    const resp = await fetch(url, options);
    const payload = await resp.json().catch(() => null);
    if (!resp.ok) {
      throw new Error((payload && payload.msg) || "HTTP " + resp.status);
    }
    if (payload && typeof payload.code === "number" && payload.code >= 400) {
      throw new Error(payload.msg || "request failed");
    }
    return payload;
  }

  function playerAPIURL(path) {
    const base = el.playerBaseUrl && el.playerBaseUrl.value ? el.playerBaseUrl.value : DEFAULT_PLAYER_BASE_URL;
    return new URL(path, base).toString();
  }

  function buildPayload() {
    const payload = {
      startTime: toApiDateTime(el.startTime.value),
      endTime: toApiDateTime(el.endTime.value),
      streamName: el.streamName.value,
      cdnName: el.cdnName.value.trim(),
      agentType: el.agentType.value.trim(),
      minLoadTime: Number(el.minLoadTime.value || 0),
      maxLoadTime: Number(el.maxLoadTime.value || 0),
      page: state.page,
      pageSize: state.pageSize,
    };
    if (el.isSucceed.value !== "") payload.isSucceed = el.isSucceed.value === "true";
    return payload;
  }

  async function loadStreams() {
    const payload = await requestJSON(API_STREAMS);
    const data = payload && payload.data ? payload.data : {};
    const options = ['<option value="">全部流</option>'];
    Object.keys(data).sort().forEach((fieldName) => {
      const streams = Array.isArray(data[fieldName]) ? data[fieldName] : [];
      streams.forEach((stream) => {
        options.push(`<option value="${escapeHtml(stream)}">${escapeHtml(fieldName + " / " + stream)}</option>`);
      });
    });
    el.streamName.innerHTML = options.join("");
  }

  async function loadPlayDomain() {
    const payload = await requestJSON(API_PLAY_DOMAIN, { method: "GET", headers: headers() });
    const domain = payload && payload.data ? payload.data.domain : "";
    el.playDomain.value = domain || "";
    el.playDomainStatus.textContent = domain ? "当前：" + domain : "未配置";
  }

  async function savePlayDomain() {
    const payload = await requestJSON(API_PLAY_DOMAIN, {
      method: "PUT",
      headers: headers(),
      body: JSON.stringify({ domain: el.playDomain.value.trim() }),
    });
    const domain = payload && payload.data ? payload.data.domain : "";
    el.playDomain.value = domain || "";
    el.playDomainStatus.textContent = domain ? "当前：" + domain : "未配置";
    showToast("播放域名已保存");
  }

  async function loadEnvDomains() {
    const payload = await requestJSON(API_ENV_DOMAINS, { method: "GET", headers: headers() });
    state.envDomains = payload && payload.data && Array.isArray(payload.data.items) ? payload.data.items : [];
    populateEnvSelector();
    renderEnvDomains();
  }

  function renderEnvDomains() {
    if (!el.envDomainRows) return;
    if (!state.envDomains.length) {
      el.envDomainRows.innerHTML = '<tr><td colspan="4" class="empty">No environment domains</td></tr>';
      if (el.envDomainStatus) el.envDomainStatus.textContent = "0 environments";
      return;
    }
    el.envDomainRows.innerHTML = state.envDomains.map((item) => [
      '<tr data-env="', escapeHtml(item.env), '">',
      "<td><strong>", escapeHtml(String(item.env || "").toUpperCase()), "</strong></td>",
      "<td>", escapeHtml(item.label || ""), "</td>",
      '<td><input class="env-domain-input" data-env-input="', escapeHtml(item.env), '" value="', escapeHtml(item.domain || ""), '" placeholder="https://example.com"></td>',
      '<td><button class="primary" data-save-env="', escapeHtml(item.env), '">Save</button></td>',
      "</tr>",
    ].join("")).join("");
    if (el.envDomainStatus) el.envDomainStatus.textContent = state.envDomains.length + " environments";
  }

  function populateEnvSelector() {
    if (!el.playerEnv) return;
    const selected = localStorage.getItem("adminPlayerEnv") || "local";
    const items = state.envDomains.length ? state.envDomains : ENV_ORDER.map((env) => ({ env, label: env }));
    el.playerEnv.innerHTML = items.map((item) => {
      const label = item.label ? `${item.env.toUpperCase()} (${item.label})` : item.env.toUpperCase();
      return `<option value="${escapeHtml(item.env)}">${escapeHtml(label)}</option>`;
    }).join("");
    el.playerEnv.value = items.some((item) => item.env === selected) ? selected : "local";
    applySelectedEnvDomain(false);
  }

  function selectedEnvDomain() {
    if (!el.playerEnv) return null;
    return state.envDomains.find((item) => item.env === el.playerEnv.value) || null;
  }

  function applySelectedEnvDomain(save) {
    const item = selectedEnvDomain();
    if (item && item.domain) {
      el.playerBaseUrl.value = item.domain;
      if (save) {
        localStorage.setItem("adminPlayerBaseUrl", item.domain);
      }
    }
    renderPlayer();
  }

  async function saveEnvDomain(env) {
    const input = document.querySelector(`[data-env-input="${env}"]`);
    const payload = await requestJSON(API_ENV_DOMAINS, {
      method: "PUT",
      headers: headers(),
      body: JSON.stringify({ env, domain: input ? input.value.trim() : "" }),
    });
    const updated = payload && payload.data ? payload.data : null;
    if (updated && updated.env) {
      state.envDomains = state.envDomains.map((item) => item.env === updated.env ? updated : item);
      populateEnvSelector();
      renderEnvDomains();
      if (el.playerEnv && el.playerEnv.value === updated.env) {
        applySelectedEnvDomain(true);
      }
    }
    showToast("Environment domain saved");
  }

  async function refreshAll() {
    await Promise.all([loadList(), loadTrend()]);
  }

  async function loadList() {
    const resp = await requestJSON(API_LIST, { method: "POST", headers: headers(), body: JSON.stringify(buildPayload()) });
    const data = resp.data || {};
    state.listItems = Array.isArray(data.items) ? data.items : [];
    state.total = Number(data.total || 0);
    state.page = Number(data.page || state.page);
    state.pageSize = Number(data.pageSize || PAGE_SIZE);
    renderTable();
    renderListMetrics();
    renderPager();
  }

  async function loadTrend() {
    const payload = buildPayload();
    el.trendStatus.textContent = "加载中";
    const resp = await requestJSON(API_TREND, {
      method: "POST",
      headers: headers(),
      body: JSON.stringify({
        startTime: payload.startTime,
        endTime: payload.endTime,
        cdnName: payload.cdnName,
        streamName: payload.streamName,
      }),
    });
    state.trendItems = Array.isArray(resp.data) ? resp.data : [];
    el.trendStatus.textContent = state.trendItems.length + " 个时间点";
    renderChart();
    renderTrendMetrics();
  }

  function renderListMetrics() {
    const items = state.listItems;
    const success = items.filter((item) => item.isSucceed).length;
    const totalLoad = items.reduce((sum, item) => sum + Number(item.waitMiliTime || 0), 0);
    el.metricTotal.textContent = number(state.total, 0);
    el.metricSuccessRate.textContent = items.length ? percent((success / items.length) * 100) : "--";
    el.metricAvgLoad.textContent = items.length ? number(totalLoad / items.length, 0) + " ms" : "--";
  }

  function renderTrendMetrics() {
    const latest = state.trendItems[state.trendItems.length - 1];
    el.metricLagRate.textContent = latest && latest.avgLagRate != null ? percent(latest.avgLagRate) : "--";
  }

  function renderTable() {
    if (!state.listItems.length) {
      el.resultRows.innerHTML = '<tr><td colspan="9" class="empty">暂无数据</td></tr>';
      return;
    }
    el.resultRows.innerHTML = state.listItems.map((item) => {
      const status = item.isSucceed ? '<span class="status-ok">成功</span>' : '<span class="status-fail">失败</span>';
      return [
        "<tr>",
        "<td>", escapeHtml(formatDate(item.createdAt)), "</td>",
        "<td>", escapeHtml(item.streamName || "-"), "</td>",
        "<td>", escapeHtml(item.cdnName || "-"), "</td>",
        "<td>", escapeHtml(item.gameUserId || "-"), "</td>",
        "<td>", status, "</td>",
        "<td>", escapeHtml(number(item.waitMiliTime, 0)), "</td>",
        "<td>", escapeHtml(number(item.retryCount, 0)), "</td>",
        "<td>", escapeHtml(item.reason || "-"), "</td>",
        "<td>", escapeHtml(item.userAgent || "-"), "</td>",
        "</tr>",
      ].join("");
    }).join("");
  }

  function renderPager() {
    const pageCount = Math.max(1, Math.ceil(state.total / state.pageSize));
    el.pageInfo.textContent = `第 ${state.page} / ${pageCount} 页`;
    el.prevPage.disabled = state.page <= 1;
    el.nextPage.disabled = state.page >= pageCount;
  }

  function renderChart() {
    const canvas = el.trendChart;
    if (!canvas) return;
    const ctx = canvas.getContext("2d");
    const rect = canvas.getBoundingClientRect();
    const dpr = window.devicePixelRatio || 1;
    canvas.width = Math.max(600, Math.floor(rect.width * dpr));
    canvas.height = Math.max(260, Math.floor(rect.height * dpr));
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    const width = rect.width;
    const height = rect.height;
    ctx.clearRect(0, 0, width, height);
    drawGrid(ctx, width, height);
    if (!state.trendItems.length) {
      ctx.fillStyle = "#667085";
      ctx.font = "14px Segoe UI, Microsoft YaHei, sans-serif";
      ctx.fillText("暂无趋势数据", 24, 42);
      return;
    }
    [
      { color: "#2563eb", key: "requestStreamTotal" },
      { color: "#0f9f6e", key: "avgStreamSuccessRate" },
      { color: "#b76e00", key: "avgLoadTime" },
      { color: "#d92d20", key: "avgLagRate" },
    ].forEach((line) => drawLine(ctx, state.trendItems.map((item) => Number(item[line.key] || 0)), line.color, width, height));
    ctx.fillStyle = "#667085";
    ctx.font = "12px Segoe UI, Microsoft YaHei, sans-serif";
    ctx.fillText(formatDate(state.trendItems[0].statHour || state.trendItems[0].createdAt), 24, height - 12);
    ctx.textAlign = "right";
    const last = state.trendItems[state.trendItems.length - 1];
    ctx.fillText(formatDate(last.statHour || last.createdAt), width - 24, height - 12);
    ctx.textAlign = "left";
  }

  function drawGrid(ctx, width, height) {
    const left = 24;
    const right = width - 24;
    const top = 18;
    const bottom = height - 34;
    ctx.strokeStyle = "#e4e7ec";
    ctx.lineWidth = 1;
    for (let i = 0; i <= 4; i += 1) {
      const y = top + ((bottom - top) * i) / 4;
      ctx.beginPath();
      ctx.moveTo(left, y);
      ctx.lineTo(right, y);
      ctx.stroke();
    }
  }

  function drawLine(ctx, values, color, width, height) {
    const left = 24;
    const right = width - 24;
    const top = 18;
    const bottom = height - 34;
    const max = Math.max(1, ...values);
    ctx.strokeStyle = color;
    ctx.lineWidth = 2;
    ctx.beginPath();
    values.forEach((value, index) => {
      const x = values.length === 1 ? left : left + ((right - left) * index) / (values.length - 1);
      const y = bottom - ((bottom - top) * value) / max;
      if (index === 0) ctx.moveTo(x, y);
      else ctx.lineTo(x, y);
    });
    ctx.stroke();
  }

  function defaultViewForSite(site) {
    return DEFAULT_VIEW_BY_SITE[site] || "fwh";
  }

  function streamBaseName() {
    return el.playerSite.value.replace(/^studio_/, "") + "-" + el.playerView.value;
  }

  function populatePlayerSelectors() {
    el.playerSite.innerHTML = SITE_OPTIONS.map((site) => `<option value="${site.value}">${site.label}</option>`).join("");
    el.playerSite.value = localStorage.getItem("adminPlayerSite") || SITE_OPTIONS[0].value;
    if (el.playerEnv) {
      el.playerEnv.innerHTML = ENV_ORDER.map((env) => `<option value="${env}">${env.toUpperCase()}</option>`).join("");
      el.playerEnv.value = localStorage.getItem("adminPlayerEnv") || "local";
    }
    populateViewSelector(localStorage.getItem("adminPlayerView") || defaultViewForSite(el.playerSite.value));
    el.playerQuality.value = localStorage.getItem("adminPlayerQuality") || "auto";
    el.playerBaseUrl.value = localStorage.getItem("adminPlayerBaseUrl") || DEFAULT_PLAYER_BASE_URL;
    renderPlayer();
  }

  function populateViewSelector(selected) {
    const views = VIEW_BY_SITE[el.playerSite.value] || ["fwh", "fwv"];
    const value = views.includes(selected) ? selected : defaultViewForSite(el.playerSite.value);
    el.playerView.innerHTML = views.map((view) => `<option value="${view}">${view}</option>`).join("");
    el.playerView.value = value;
  }

  function buildPlayerUrl() {
    const base = new URL(el.playerBaseUrl.value || DEFAULT_PLAYER_BASE_URL, window.location.href);
    base.searchParams.set("site", el.playerSite.value);
    base.searchParams.set("view", el.playerView.value);
    base.searchParams.set("quality", el.playerQuality.value);
    return base.toString();
  }

  function renderPlayer() {
    const baseName = streamBaseName();
    el.playerUrl.value = buildPlayerUrl();
    el.playerLadderRows.innerHTML = LADDER.map((row) => [
      "<tr>",
      "<td>", escapeHtml(row.level), "</td>",
      "<td>", escapeHtml(baseName + row.suffix), "</td>",
      "<td>", escapeHtml(row.codec), "</td>",
      "<td>", escapeHtml(row.bitrate), "</td>",
      "<td>", escapeHtml(row.resolution), "</td>",
      "<td>", escapeHtml(row.rule), "</td>",
      "</tr>",
    ].join("")).join("");
  }

  function renderPusher() {
    el.pusherProfileRows.innerHTML = PUSHER_PROFILES.map((row) => [
      "<tr>",
      "<td>", escapeHtml(row.level), "</td>",
      "<td>", escapeHtml(row.template), "</td>",
      "<td>", escapeHtml(row.encoding), "</td>",
      "<td>", escapeHtml(row.status), "</td>",
      "</tr>",
    ].join("")).join("");
  }

  function renderHealthCards() {
    const items = [
      { name: "Backend /healthz", url: "/healthz" },
      { name: "Backend /api/stat/health", url: "/api/stat/health" },
      { name: "AbrPlayer configured /healthz", url: new URL("/healthz", el.playerBaseUrl.value || window.location.origin + "/").toString() },
    ];
    el.healthCards.innerHTML = items.map((item, index) => `
      <div class="health-card">
        <span>${escapeHtml(item.name)}</span>
        <code>${escapeHtml(item.url)}</code>
        <button data-health-index="${index}">Check</button>
        <strong id="healthResult${index}">not checked</strong>
      </div>
    `).join("");
    el.healthCards.querySelectorAll("button[data-health-index]").forEach((button) => {
      button.addEventListener("click", () => run(async () => checkHealth(items[Number(button.dataset.healthIndex)], Number(button.dataset.healthIndex))));
    });
  }

  async function checkHealth(item, index) {
    const target = $("healthResult" + index);
    target.textContent = "checking";
    try {
      const resp = await fetch(item.url, { cache: "no-store" });
      target.textContent = resp.ok ? "ok " + resp.status : "fail " + resp.status;
      target.className = resp.ok ? "health-ok" : "health-fail";
    } catch (err) {
      target.textContent = "fail: " + (err.message || String(err));
      target.className = "health-fail";
    }
  }

  function switchView(view) {
    el.navItems.forEach((item) => item.classList.toggle("active", item.dataset.view === view));
    el.views.forEach((section) => section.classList.toggle("active", section.id === "view-" + view));
    const copy = PAGE_COPY[view] || PAGE_COPY.stat;
    el.pageTitle.textContent = copy[0];
    el.pageSubtitle.textContent = copy[1];
    if (view === "stat") renderChart();
    if (view === "player") run(loadEnvDomains);
    if (view === "health") renderHealthCards();
  }

  function initDefaults() {
    const end = new Date();
    const start = new Date(end.getTime() - 7 * 24 * 60 * 60 * 1000);
    el.startTime.value = toLocalInputValue(start);
    el.endTime.value = toLocalInputValue(end);
    populatePlayerSelectors();
    renderPusher();
  }

  function bindEvents() {
    el.navItems.forEach((button) => button.addEventListener("click", () => switchView(button.dataset.view)));
    el.savePlayDomain.addEventListener("click", () => run(savePlayDomain));
    if (el.envDomainRows) {
      el.envDomainRows.addEventListener("click", (event) => {
        const button = event.target.closest("button[data-save-env]");
        if (!button) return;
        run(() => saveEnvDomain(button.dataset.saveEnv));
      });
    }
    el.refresh.addEventListener("click", () => {
      state.page = 1;
      run(refreshAll);
    });
    el.reset.addEventListener("click", () => {
      el.streamName.value = "";
      el.isSucceed.value = "";
      el.cdnName.value = "";
      el.agentType.value = "";
      el.minLoadTime.value = "";
      el.maxLoadTime.value = "";
      initDefaults();
      state.page = 1;
      run(refreshAll);
    });
    el.prevPage.addEventListener("click", () => {
      if (state.page <= 1) return;
      state.page -= 1;
      run(loadList);
    });
    el.nextPage.addEventListener("click", () => {
      const pageCount = Math.max(1, Math.ceil(state.total / state.pageSize));
      if (state.page >= pageCount) return;
      state.page += 1;
      run(loadList);
    });
    el.playerSite.addEventListener("change", () => {
      populateViewSelector(defaultViewForSite(el.playerSite.value));
      persistPlayerState();
      renderPlayer();
    });
    if (el.playerEnv) {
      el.playerEnv.addEventListener("change", () => {
        persistPlayerState();
        applySelectedEnvDomain(true);
      });
    }
    [el.playerView, el.playerQuality, el.playerBaseUrl].forEach((node) => {
      node.addEventListener("change", () => {
        persistPlayerState();
        renderPlayer();
      });
      node.addEventListener("input", () => {
        persistPlayerState();
        renderPlayer();
      });
    });
    el.copyPlayerUrl.addEventListener("click", async () => {
      await navigator.clipboard.writeText(el.playerUrl.value);
      showToast("播放器链接已复制");
    });
    el.openPlayerUrl.addEventListener("click", () => window.open(el.playerUrl.value, "_blank", "noopener"));
    window.addEventListener("resize", renderChart);
  }

  function persistPlayerState() {
    localStorage.setItem("adminPlayerSite", el.playerSite.value);
    if (el.playerEnv) localStorage.setItem("adminPlayerEnv", el.playerEnv.value);
    localStorage.setItem("adminPlayerView", el.playerView.value);
    localStorage.setItem("adminPlayerQuality", el.playerQuality.value);
    localStorage.setItem("adminPlayerBaseUrl", el.playerBaseUrl.value);
  }

  async function run(task) {
    try {
      await task();
    } catch (err) {
      showToast(err.message || String(err));
    }
  }

  initDefaults();
  bindEvents();
  run(async () => {
    await loadStreams();
    await loadPlayDomain();
    await loadEnvDomains();
    await refreshAll();
  });
})();
