/**
 * main.js
 * UI bindings for site-based ABR playback.
 */

window.VConsole && new window.VConsole();

let pageSiteName = currentSiteName;
let pageViewName = 'fwh';
let pageStreamNames = null;

const SITE_OPTIONS = [
  { value: 'studio_3drush' },
  { value: 'studio_gsp2w' }
];

const VIEW_OPTIONS = [
  { value: 'fwh' },
  { value: 'fwv' }
];

const DEFAULT_VIEW_BY_SITE = {
  studio_3drush: 'fwh',
  studio_gsp2w: 'fwv'
};

const SITE_BY_STREAM = {
  '3drush-fwh': 'studio_3drush',
  '3drush-fwv': 'studio_3drush',
  'gsp2w-fwv': 'studio_gsp2w',
  'gsp2w-fwh': 'studio_gsp2w',
};

const QUALITY_LEVELS = [
  { value: 'auto', label: 'auto' },
  { value: 'high', label: '1080P' },
  { value: 'standard', label: '720P' },
  { value: 'economic', label: '360P' },
  { value: 'bottom', label: 'audio' }
];

function normalizeSiteName(siteName) {
  const value = (siteName || '').trim();
  if (SITE_OPTIONS.some(site => site.value === value)) return value;
  return SITE_BY_STREAM[value] || SITE_OPTIONS[0].value;
}

function normalizeViewName(viewName) {
  const value = (viewName || '').trim().toLowerCase();
  if (VIEW_OPTIONS.some(view => view.value === value)) return value;
  return VIEW_OPTIONS[0].value;
}

function defaultViewForSite(siteName) {
  return DEFAULT_VIEW_BY_SITE[normalizeSiteName(siteName)] || VIEW_OPTIONS[0].value;
}

function viewNameFromStream(streamName) {
  const value = (streamName || '').trim().toLowerCase();
  if (value.includes('fwv')) return 'fwv';
  if (value.includes('fwh')) return 'fwh';
  return VIEW_OPTIONS[0].value;
}

function streamBaseNameForSiteView(siteName, viewName) {
  const siteBase = normalizeSiteName(siteName).replace(/^studio_/, '');
  return `${siteBase}-${normalizeViewName(viewName)}`;
}

function legacyStreamName(streamName) {
  const value = (streamName || '').trim();
  const match = value.match(/^(3drush|gsp2w)_(fwh|fwv)(.*)$/);
  if (!match) return value;
  return `${match[1]}-${match[2]}${match[3]}`;
}

function streamNamesFromSelection(siteName, viewName) {
  const site = streamBaseNameForSiteView(siteName, viewName);
  return {
    high: site,
    standard: `${site}_standard`,
    standardHevc: `${site}_standard_hevc`,
    economic: `${site}_economic`,
    bottom: `${site}_audio`
  };
}

function streamNamesForPlayback(streamNames) {
  if (!window.__useLegacyStreamNames) return streamNames;
  return Object.keys(streamNames || {}).reduce((out, key) => {
    out[key] = legacyStreamName(streamNames[key]);
    return out;
  }, {});
}

function normalizeQualityChoice(choice) {
  const value = (choice || '').trim().toLowerCase();
  if (value === '1080p' || value === '1080') return 'high';
  if (value === '720p' || value === '720') return 'standard';
  if (value === '360p' || value === '360') return 'economic';
  if (value === 'audio') return 'bottom';
  return QUALITY_LEVELS.some(item => item.value === value) ? value : 'auto';
}

function buildSelectableStreams() {
  return SITE_OPTIONS.map(site => ({ name: site.value, label: site.value }));
}

function buildSelectableViews() {
  return VIEW_OPTIONS.map(view => ({ name: view.value, label: view.value }));
}

function populateStreamSelect(siteName, viewName) {
  pageSiteName = normalizeSiteName(siteName);
  pageViewName = normalizeViewName(viewName);
  pageStreamNames = streamNamesFromSelection(pageSiteName, pageViewName);
  setStreamLadder(pageSiteName, streamNamesForPlayback(pageStreamNames));

  const select = document.getElementById('stream-select');
  if (!select) return;

  select.innerHTML = '';
  for (const site of buildSelectableStreams()) {
    select.add(new Option(site.label, site.name, false, site.name === pageSiteName));
  }
  select.value = pageSiteName;

  if (typeof M !== 'undefined') {
    M.FormSelect.init(select);
  }

  populateViewSelect();
  populateQualitySelect();
}

function populateViewSelect() {
  const select = document.getElementById('view-select');
  if (!select) return;

  select.innerHTML = '';
  for (const view of buildSelectableViews()) {
    select.add(new Option(view.label, view.name, false, view.name === pageViewName));
  }
  select.value = pageViewName;

  if (typeof M !== 'undefined') {
    M.FormSelect.init(select);
  }
}

function populateQualitySelect() {
  const select = document.getElementById('quality-select');
  if (!select) return;

  const current = normalizeQualityChoice(getCookie('phil_quality_choice') || select.value || 'auto');
  select.innerHTML = '';
  for (const item of QUALITY_LEVELS) {
    select.add(new Option(item.label, item.value, false, item.value === current));
  }
  select.value = current;

  if (typeof M !== 'undefined') {
    M.FormSelect.init(select);
  }
}

function selectedQualityLevel() {
  return normalizeQualityChoice($('#quality-select').val() || getCookie('phil_quality_choice'));
}

function streamForQualityLevel(quality) {
  quality = normalizeQualityChoice(quality);
  if (quality === 'auto') return null;
  return getPreferredStreamForQuality(quality) || CONFIG.DEFAULT_FALLBACK_STREAM;
}

function streamNameForPlayback(streamName) {
  const value = (streamName || '').trim();
  if (!value) return '';
  return window.__useLegacyStreamNames ? legacyStreamName(value) : value;
}

function streamNameFromWebrtcUrl(url) {
  const value = (url || '').trim();
  const match = value.match(/\/live\/([^?]+)/);
  return match ? match[1] : '';
}

function publicPlayerUrlForWebrtc(url) {
  const value = (url || '').trim();
  if (!value) return '';
  const publicOrigin = 'https://abrplayer.example.com';
  return `${publicOrigin}/?webrtc=${encodeURIComponent(value)}`;
}

function showPublicPlayerHint(url, reason) {
  const handoffUrl = publicPlayerUrlForWebrtc(url);
  if (!handoffUrl) return;

  let hint = document.getElementById('public-player-hint');
  if (!hint) {
    hint = document.createElement('div');
    hint.id = 'public-player-hint';
    hint.className = 'helper-text';
    const field = document.querySelector('.webrtc-field');
    if (field) field.appendChild(hint);
  }

  const reasonText = reason ? ` (${reason})` : '';
  hint.innerHTML = `Local playback failed${reasonText}. <a href="${handoffUrl}" target="_blank" rel="noopener">Open public player</a>`;
  console.warn(`[Player] Local playback failed${reasonText}. Public player URL: ${handoffUrl}`);
}

async function startDirectWebrtcUrl(url) {
  url = (url || '').trim();
  if (!url) return false;

  const streamName = streamNameFromWebrtcUrl(url);
  if (streamName) {
    ABRState.currentStream = streamName;
    syncStreamSelectValue(streamName);
    syncViewSelectValue(viewNameFromStream(streamName));
    if (typeof updateCanvasProfile === 'function') updateCanvasProfile(url);
  }

  autoSelectActive = false;
  currentWebrtcUrl = url;
  setCookie('phil_stream_url', url, 1);
  document.getElementById('webrtc').value = url;
  console.log(`[Init] direct webrtc playback, stream=${streamName || 'custom'}`);
  attachPlayer({ source: url });
  return true;
}

function syncStreamSelectValue(streamName) {
  const select = document.getElementById('stream-select');
  if (!select) return;
  const siteName = normalizeSiteName(streamName || pageSiteName);
  if ([...select.options].some(option => option.value === siteName)) {
    select.value = siteName;
    if (typeof M !== 'undefined') M.FormSelect.init(select);
  }
}

function syncViewSelectValue(viewName) {
  const select = document.getElementById('view-select');
  if (!select) return;
  const selectedView = normalizeViewName(viewName || pageViewName);
  if ([...select.options].some(option => option.value === selectedView)) {
    select.value = selectedView;
    if (typeof M !== 'undefined') M.FormSelect.init(select);
  }
}

async function startSelectedQuality() {
  const quality = selectedQualityLevel();
  setCookie('phil_quality_choice', quality, 1);
  if (quality === 'auto') {
    await startSiteAbr(pageSiteName);
    return;
  }

  await startManualStream(streamForQualityLevel(quality));
}

function selectSite(siteName) {
  pageSiteName = normalizeSiteName(siteName);
  pageViewName = defaultViewForSite(pageSiteName);
  pageStreamNames = streamNamesFromSelection(pageSiteName, pageViewName);
  setCookie('phil_site_choice', pageSiteName, 1);
  setCookie('phil_view_choice', pageViewName, 1);
  setStreamLadder(pageSiteName, streamNamesForPlayback(pageStreamNames));
  syncStreamSelectValue(pageSiteName);
  syncViewSelectValue(pageViewName);
}

function selectView(viewName) {
  pageViewName = normalizeViewName(viewName);
  pageStreamNames = streamNamesFromSelection(pageSiteName, pageViewName);
  setCookie('phil_view_choice', pageViewName, 1);
  setStreamLadder(pageSiteName, streamNamesForPlayback(pageStreamNames));
  syncViewSelectValue(pageViewName);
}

function applyLegacyStreamNameMode() {
  window.__useLegacyStreamNames = true;
  pageStreamNames = streamNamesFromSelection(pageSiteName, pageViewName);
  setStreamLadder(pageSiteName, streamNamesForPlayback(pageStreamNames));
}

async function startSiteAbr(siteName) {
  console.log('[Init] Checking capabilities...');
  await detectHevcCapability();

  if (siteName && normalizeSiteName(siteName) !== pageSiteName) {
    selectSite(siteName);
  }
  setStreamLadder(pageSiteName, streamNamesForPlayback(pageStreamNames));
  autoSelectActive = true;
  setCookie('phil_quality_choice', 'auto', 1);
  ABRState.lastSwitchTime = Date.now();

  let pick = getCookie('phil_stream_choice');
  if (!pick || !getStreamProfile(pick)) {
      const standard = getPreferredStandardProfile();
      pick = standard ? standard.name : currentSiteName;
  }
  pick = preferHevcForStandardStream(pick);
  if (pick === CONFIG.DEFAULT_FALLBACK_STREAM) {
      const standard = getPreferredStandardProfile();
      pick = standard ? standard.name : currentSiteName;
  }
  pick = preferHevcForStandardStream(pick);

  ABRState.currentStream = pick;
  syncStreamSelectValue(pick);
  syncViewSelectValue(viewNameFromStream(pick));
  console.log(`[Init] selected site=${pageSiteName}, view=${pageViewName}, quality=auto, stream=${pick}`);
  const url = await getWebrtcUrl(pick);

  if (url) {
      currentWebrtcUrl = url;
      document.getElementById('webrtc').value = url;
      attachPlayer({ source: url });
  } else {
      console.error('Auto start failed: No URL');
  }

  startMonitorLoop();
}

async function startManualStream(streamName) {
  streamName = streamNameForPlayback(streamName);
  if (!streamName) return;

  console.log('[Init] Checking capabilities...');
  await detectHevcCapability();

  setStreamLadder(pageSiteName, streamNamesForPlayback(pageStreamNames));
  autoSelectActive = false;
  ABRState.currentStream = streamName;
  syncStreamSelectValue(streamName);
  syncViewSelectValue(viewNameFromStream(streamName));
  const profile = getStreamProfile(streamName);
  if (profile) setCookie('phil_quality_choice', getQualityKey(profile), 1);
  console.log(`[Init] selected site=${pageSiteName}, view=${pageViewName}, quality=${profile ? getQualityKey(profile) : 'unknown'}, stream=${streamName}`);

  const url = await getWebrtcUrl(streamName);
  if (url) {
      currentWebrtcUrl = url;
      document.getElementById('webrtc').value = url;
      attachPlayer({ source: url });
  } else {
      console.error('Manual start failed: No URL');
  }
}

$('#quality-select').on('change', debounce(async function() {
  const quality = selectedQualityLevel();
  setCookie('phil_quality_choice', quality, 1);
  if (tcplayer || currentWebrtcUrl) {
    destroyPlayer();
    await startSelectedQuality();
  }
}, 400));

$('#stream-select').on('change', debounce(async function() {
  selectSite($('#stream-select').val());
  if (tcplayer || currentWebrtcUrl) {
    destroyPlayer();
    await startSelectedQuality();
  }
}, 400));

$('#view-select').on('change', debounce(async function() {
  selectView($('#view-select').val());
  if (tcplayer || currentWebrtcUrl) {
    destroyPlayer();
    await startSelectedQuality();
  }
}, 400));

$('#startPlay').on('click', async function() {
  destroyPlayer();
  const typedUrl = ($('#webrtc').val() || '').trim();
  if (typedUrl) {
    await startDirectWebrtcUrl(typedUrl);
    return;
  }
  await startSelectedQuality();
});

$('#stopPlay').on('click', function() {
  destroyPlayer();
  autoSelectActive = false;
  if (monitorTimer) { clearInterval(monitorTimer); monitorTimer = null; }
  startPlayTime = 0;

  $('.stat-info').hide();
  $('ul.stat').empty();
});

function onPlayStats(data) {
  const statSections = [
    { title: 'Video', data: data && data.video },
    { title: 'Audio', data: data && data.audio }
  ];
  let ulHtml = '';

  statSections.forEach(function(section) {
    ulHtml += `<li class="stat-section-title"><div class="title">${section.title}</div></li>`;
    if (!section.data || Object.keys(section.data).length === 0) {
      ulHtml += `<li><div class="label">status</div><div class="text">No data</div></li>`;
      return;
    }

    Object.keys(section.data).forEach(function(key) {
      const val = formatStatValue(key, section.data[key]);
      ulHtml += `<li><div class="label">${escapeStatText(key)}</div><div class="text">${escapeStatText(val)}</div></li>`;
    });
  });
  $('ul.stat').html(ulHtml);
}

function formatStatValue(key, value) {
  if (value === null || value === undefined || value === '') return '-';
  if (key === 'bitrate' && Number.isFinite(Number(value))) {
    return `${(Number(value) / 1000).toFixed(0)} kbps`;
  }
  if (typeof value === 'object') return JSON.stringify(value);
  return String(value);
}

function escapeStatText(value) {
  return String(value)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

$('#showStat').on('click', function() { $('.stat-info').toggle(); });
$('#mutePlay').on('click', function() {
  if (typeof togglePlayerMuted === 'function') togglePlayerMuted();
});
$('.close-icon').on('click', function() { $('.stat-info').hide(); });
$('#enterFullScreen').on('click', function() { try { if (tcplayer) tcplayer.requestFullscreen(true); } catch(e){} });

document.addEventListener('DOMContentLoaded', function() {
    const params = new URLSearchParams(window.location.search);
    const site = params.get('site') || getCookie('phil_site_choice') || currentSiteName;
    const view = params.get('view') || defaultViewForSite(site);
    const quality = params.get('quality');
    const directWebrtcUrl = params.get('webrtc') || params.get('url');
    if (quality) setCookie('phil_quality_choice', normalizeQualityChoice(quality), 1);
    populateStreamSelect(site, view);
    if (typeof updateMuteButton === 'function') updateMuteButton();
    if (directWebrtcUrl) {
      startDirectWebrtcUrl(directWebrtcUrl);
    }
});
window.addEventListener('beforeunload', function(){ destroyPlayer(); });
