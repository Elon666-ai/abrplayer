const CONFIG = {
  DEBUG: true,
  SCRIPT_VERSION: 'v1.0.2',
  ABR_SWITCH_COOLDOWN_MS: 50000,
  MONITOR_INTERVAL_MS: 2000,
  UPGRADE_PROBE_INTERVAL_MS: 12000,
  BANDWIDTH_LOG_INTERVAL_MS: 60000,
  ABR_PROBE_LOG_INTERVAL_MS: 60000,
  AUDIO_DEBUG_LOG_INTERVAL_MS: 5000,
  ZERO_BANDWIDTH_PROBE_INTERVAL_MS: 60000,
  UPGRADE_CONFIRM_COUNT: 1,
  DOWNGRADE_CONFIRM_COUNT: 3,
  UPGRADE_PROBATION_MS: 15000,
  UPGRADE_BACKOFF_MS: 90000,
  HEVC_BACKOFF_MS: 300000,
  NETWORK_ERROR_GRACE_MS: 60000,
  BITRATE_DOWN_FACTOR: 0.6,
  STALL_THRESHOLD_MS: 5000,
  // Bandwidth probing downloads static same-origin assets only. It is not tied
  // to H.264/HEVC codec choice; codec handling is done by playback errors.
  BANDWIDTH_TEST_URLS: [
    'NIOES8001.jpeg'
  ],
  DEFAULT_FALLBACK_STREAM: '3drush-fwh_audio'
};

const UPGRADE_THRESHOLDS_KBPS = {
  bottom: 300,
  economic: 600,
  standard: 1500
};

const DOWNGRADE_THRESHOLDS_KBPS = {
  bottom: 100,
  economic: 200,
  standard: 500,
  high: 1200
};

const QUALITY_ORDER = ['bottom', 'economic', 'standard', 'high'];

console.log(`[ABR] script loaded: ${CONFIG.SCRIPT_VERSION}`);

let currentSiteName = '3drush-fwv';
let STREAM_LADDER = buildStreamLadder(currentSiteName);

function buildStreamLadder(siteName) {
    const ladder = [
        { level: 0, key: 'bottom',   name: `${siteName}_audio`,    bitrate: 128,  codec: 'opus', online: true },
        { level: 1, key: 'economic', name: `${siteName}_economic`, bitrate: 400,  codec: 'h264', online: true },
        { level: 2, key: 'standard_hevc', name: `${siteName}_standard_hevc`, bitrate: 600, codec: 'hevc', online: supportsHEVC() },
        { level: 3, key: 'standard', name: `${siteName}_standard`, bitrate: 1000, codec: 'h264', online: true },
        { level: 4, key: 'high',     name: siteName,               bitrate: 2000, codec: 'h264', online: true }
    ];
    return ladder.filter(stream => stream.online);
}

function setSiteName(siteName) {
    siteName = (siteName || '').trim();
    if (!siteName) return;
    currentSiteName = siteName;
    STREAM_LADDER = buildStreamLadder(siteName);
    CONFIG.DEFAULT_FALLBACK_STREAM = `${siteName}_audio`;
}

function setStreamLadder(siteName, streamNames) {
    siteName = (siteName || '').trim();
    if (!siteName) return;
    currentSiteName = siteName;
    STREAM_LADDER = buildStreamLadder(siteName).map(profile => {
        let name = streamNames && streamNames[profile.key];
        if (profile.key === 'standard_hevc') {
            name = streamNames && (streamNames.standardHevc || streamNames.standard_hevc);
        }
        return Object.assign({}, profile, { name: (name || profile.name).trim() });
    });
    const bottom = STREAM_LADDER.find(s => s.key === 'bottom');
    CONFIG.DEFAULT_FALLBACK_STREAM = bottom ? bottom.name : `${siteName}_audio`;
}

// 鍏ㄥ眬鍙橀噺
let tcplayer = null;
let autoSelectActive = false;
let currentWebrtcUrl = null;
let monitorTimer = null;
let playTimer = null;
let startPlayTime = 0;
let startPlayTimeLast = -1;
let playerMuted = getCookie('phil_player_muted') === '1';
let lastAudioDebugLogTime = 0;

// ABR 鐘舵€佹満
const ABRState = {
  currentStream: null,
  lastSwitchTime: 0,
  currentReceiveKbps: 0,
  probedBandwidthKbps: 0,
  inCooldown: false,
  // 鐘舵€侀攣涓庤鏁板櫒
  isSwitching: false,
  consecutiveLowCount: 0,
  consecutiveProbeLowCount: 0,
  consecutiveHighCount: 0,
  lastProbeTime: 0,
  lastBandwidthLogTime: 0,
  lastAbrProbeLogTime: 0,
  lastNetworkIssueLogTime: 0,
  lastNetworkIssueTime: 0,
  zeroBandwidthBackoffUntil: 0,
  probeInFlight: false,
  lastStallTime: 0,
  lastErrorTime: 0,
  lastUpgradeTime: 0,
  lastUpgradeFromStream: null,
  upgradeBackoffUntil: 0,
  hevcBackoffUntil: 0
};

/* ------------------------
   ABR 杈呭姪鍑芥暟
   ------------------------ */
function getStreamProfile(streamName) {
  if (!streamName) return STREAM_LADDER[0]; 
  return STREAM_LADDER.find(s => s.name === streamName);
}

function getCurrentStreamIndex() {
    return STREAM_LADDER.findIndex(s => s.name === ABRState.currentStream);
}

function getQualityKey(profileOrKey) {
    const key = typeof profileOrKey === 'string' ? profileOrKey : profileOrKey && profileOrKey.key;
    return key === 'standard_hevc' ? 'standard' : key;
}

function getQualityRank(profileOrKey) {
    return QUALITY_ORDER.indexOf(getQualityKey(profileOrKey));
}

function getPreferredStreamForQuality(qualityKey, options = {}) {
    const preferHevc = options.preferHevc !== false;
    if (qualityKey === 'standard') {
        if (preferHevc) {
            const hevc = STREAM_LADDER.find(s => s.key === 'standard_hevc' && s.online);
            if (supportsHEVC() && hevc && !isHevcBackoffActive()) return hevc.name;
        }
        const h264 = STREAM_LADDER.find(s => s.key === 'standard' && s.online);
        if (h264) return h264.name;
    }

    const stream = STREAM_LADDER.find(s => getQualityKey(s) === qualityKey && s.online);
    return stream ? stream.name : null;
}

function getPreviousQualityStreamName(profileOrIndex) {
    const profile = typeof profileOrIndex === 'number' ? STREAM_LADDER[profileOrIndex] : profileOrIndex;
    const rank = getQualityRank(profile);
    for (let i = rank - 1; i >= 0; i--) {
        const stream = getPreferredStreamForQuality(QUALITY_ORDER[i]);
        if (stream) return stream;
    }
    return CONFIG.DEFAULT_FALLBACK_STREAM;
}

function getNextQualityStreamName(profileOrIndex, options = {}) {
    const profile = typeof profileOrIndex === 'number' ? STREAM_LADDER[profileOrIndex] : profileOrIndex;
    const rank = getQualityRank(profile);
    for (let i = rank + 1; i < QUALITY_ORDER.length; i++) {
        const stream = getPreferredStreamForQuality(QUALITY_ORDER[i], options);
        if (stream) return stream;
    }
    return null;
}

function getPreferredStandardProfile(options = {}) {
    const preferHevc = options.preferHevc !== false;
    if (preferHevc && supportsHEVC() && !isHevcBackoffActive()) {
        const hevc = STREAM_LADDER.find(s => s.key === 'standard_hevc');
        if (hevc) return hevc;
    }
    return STREAM_LADDER.find(s => s.key === 'standard');
}

function preferHevcForStandardStream(streamName) {
    const profile = getStreamProfile(streamName);
    if (!profile || profile.key !== 'standard' || !supportsHEVC() || isHevcBackoffActive()) return streamName;

    const hevc = STREAM_LADDER.find(s => s.key === 'standard_hevc' && s.online);
    return hevc ? hevc.name : streamName;
}

function isHevcBackoffActive() {
    const active = Date.now() < ABRState.hevcBackoffUntil;
    if (active) {
        logAbrProbeProgress(`[ABR] HEVC standard is in backoff until ${new Date(ABRState.hevcBackoffUntil).toISOString()}`);
    }
    return active;
}

function markNetworkIssue(reason) {
    ABRState.lastNetworkIssueTime = Date.now();
    const now = Date.now();
    if (now - ABRState.lastNetworkIssueLogTime >= CONFIG.ABR_PROBE_LOG_INTERVAL_MS) {
        ABRState.lastNetworkIssueLogTime = now;
        console.warn(`[ABR] Network issue suspected: ${reason}`);
    }
}

function isNetworkSuspectForHevcFailure() {
    const now = Date.now();
    if (typeof navigator !== 'undefined' && navigator.onLine === false) return true;
    if (ABRState.lastNetworkIssueTime && now - ABRState.lastNetworkIssueTime < CONFIG.NETWORK_ERROR_GRACE_MS) return true;
    if (ABRState.lastStallTime && now - ABRState.lastStallTime < CONFIG.NETWORK_ERROR_GRACE_MS) return true;
    if (ABRState.probedBandwidthKbps > 0 && ABRState.probedBandwidthKbps < UPGRADE_THRESHOLDS_KBPS.standard) return true;
    return false;
}

function fallbackHevcToH264(reason) {
    const profile = getStreamProfile(ABRState.currentStream);
    if (!profile || profile.key !== 'standard_hevc') return false;

    const h264Standard = STREAM_LADDER.find(s => s.key === 'standard');
    if (!h264Standard) return false;

    if (isNetworkSuspectForHevcFailure()) {
        console.warn(`[ABR] HEVC error treated as network issue; no HEVC backoff. Reason: ${reason}`);
        return false;
    }

    ABRState.hevcBackoffUntil = Date.now() + CONFIG.HEVC_BACKOFF_MS;
    console.warn(`[ABR] HEVC fallback to H264 standard. Reason: ${reason}. ${ABRState.currentStream} -> ${h264Standard.name}`);
    ABRState.isSwitching = true;
    safeSwitchToStream(h264Standard.name, 'hevc_fallback');
    return true;
}

function downgradeHevcForNetworkIssue(reason) {
    const profile = getStreamProfile(ABRState.currentStream);
    if (!profile || profile.key !== 'standard_hevc' || !isNetworkSuspectForHevcFailure()) return false;

    const currentIndex = getCurrentStreamIndex();
    const targetStream = getPreviousQualityStreamName(currentIndex);
    console.warn(`[ABR] HEVC error treated as network issue; no HEVC backoff. Reason: ${reason}. ${ABRState.currentStream} -> ${targetStream}`);
    ABRState.isSwitching = true;
    safeSwitchToStream(targetStream, 'downgrade');
    return true;
}

function fallbackUnderscoreToLegacy(reason) {
    const current = ABRState.currentStream || '';
    if (!current.includes('_') || typeof legacyStreamName !== 'function') return false;

    const legacy = legacyStreamName(current);
    if (!legacy || legacy === current) return false;

    if (typeof applyLegacyStreamNameMode === 'function') {
        applyLegacyStreamNameMode();
    } else {
        window.__useLegacyStreamNames = true;
    }
    console.warn(`[ABR] Legacy stream-name fallback. Reason: ${reason}. ${current} -> ${legacy}`);
    ABRState.isSwitching = true;
    safeSwitchToStream(legacy, 'legacy_fallback');
    return true;
}

function checkCooldown() {
    const now = Date.now();
    if (now - ABRState.lastSwitchTime < CONFIG.ABR_SWITCH_COOLDOWN_MS) {
        return true;
    }
    return false;
}

/**
 * 鏍规嵁甯﹀涓婇檺锛屽鎵炬渶鍚堥€傜殑娴?(浠庨珮寰€浣庢壘)
 */
function selectBestStreamBelow(kbps) {
    return selectBestStreamByBitrate(kbps);
}

function selectBestStreamByBitrate(kbps) {
    let key = 'high';
    if (kbps < DOWNGRADE_THRESHOLDS_KBPS.economic) key = 'bottom';
    else if (kbps < DOWNGRADE_THRESHOLDS_KBPS.standard) key = 'economic';
    else if (kbps < DOWNGRADE_THRESHOLDS_KBPS.high) key = 'standard';

    return getPreferredStreamForQuality(key) || CONFIG.DEFAULT_FALLBACK_STREAM;
}

function selectDowngradeStreamForCurrent(kbps) {
    const currentIndex = getCurrentStreamIndex();
    if (currentIndex <= 0) return null;

    const currentProfile = STREAM_LADDER[currentIndex];
    const threshold = DOWNGRADE_THRESHOLDS_KBPS[getQualityKey(currentProfile)];
    const shouldDowngrade = threshold && kbps < threshold;

    if (!shouldDowngrade) return null;
    return getPreviousQualityStreamName(currentProfile);
}

function shouldRollbackUpgrade() {
    return ABRState.lastUpgradeFromStream
        && Date.now() - ABRState.lastUpgradeTime <= CONFIG.UPGRADE_PROBATION_MS;
}

function clearUpgradeProbeState() {
    ABRState.consecutiveHighCount = 0;
    ABRState.consecutiveProbeLowCount = 0;
    ABRState.probedBandwidthKbps = 0;
}

function resetAbrCounters() {
    ABRState.consecutiveLowCount = 0;
    ABRState.consecutiveProbeLowCount = 0;
    ABRState.consecutiveHighCount = 0;
}

function isAudioOnlyStream(streamName, source) {
    const profile = getStreamProfile(streamName);
    if (profile && profile.key === 'bottom') return true;
    const value = `${streamName || ''} ${source || ''}`.toLowerCase();
    return value.includes('_audio') || value.includes('audio');
}

function describePlaybackStream(width, height) {
    const profile = getStreamProfile(ABRState.currentStream) || {};
    const codec = (profile.codec || (isAudioOnlyStream(ABRState.currentStream, currentWebrtcUrl) ? 'opus' : 'unknown')).toUpperCase();
    const quality = getQualityKey(profile) || 'unknown';
    const level = getQualityRank(profile);
    const resolution = width > 0 && height > 0 ? `${width}x${height}` : 'audio-only';
    return {
        stream: ABRState.currentStream || 'unknown',
        quality,
        level: level >= 0 ? level : 'unknown',
        codec,
        resolution
    };
}

function setVideoPlaybackVisible(visible) {
    const el = document.getElementById('player-container-id');
    if (!el) return;
    el.style.display = visible ? '' : 'none';
}

function isFatalPlayerError(errCode) {
    return [14, 1001, 1002, -2001, -2004, -2005].includes(Number(errCode));
}

function cleanupPlayerDom() {
    const container = document.getElementById('local-video');
    if (!container) return;

    container.querySelectorAll(
        '#player-container-id, .video-js, .vjs-error-display, .vjs-modal-dialog, video, #snapshot-canvas'
    ).forEach(el => {
        if (!el.classList || !el.classList.contains('stat-info')) {
            el.remove();
        }
    });
}

function clearPlayerErrorState() {
    try {
        if (tcplayer && typeof tcplayer.error === 'function') {
            tcplayer.error(null);
        }
    } catch (e) {}

    document.querySelectorAll('.vjs-error, .vjs-error-display, .vjs-modal-dialog')
        .forEach(el => {
            if (el.classList) el.classList.remove('vjs-error');
            if (el.classList && (el.classList.contains('vjs-error-display') || el.classList.contains('vjs-modal-dialog'))) {
                el.remove();
            }
        });
}

function maybeDowngradeByBandwidth(kbps, source) {
    const currentIndex = getCurrentStreamIndex();
    if (currentIndex <= 0) {
        ABRState.consecutiveProbeLowCount = 0;
        return false;
    }

    const targetStream = selectDowngradeStreamForCurrent(kbps);
    const targetIndex = STREAM_LADDER.findIndex(s => s.name === targetStream);
    if (targetIndex >= 0 && targetIndex < currentIndex) {
        ABRState.consecutiveProbeLowCount++;
        ABRState.consecutiveHighCount = 0;
        markNetworkIssue(`${source} low: ${kbps} kbps`);
        logAbrProbeProgress(`[ABR] Downgrade probe low x${ABRState.consecutiveProbeLowCount}/${CONFIG.DOWNGRADE_CONFIRM_COUNT}: ${kbps} kbps via ${source} -> ${targetStream}`);
        if (ABRState.consecutiveProbeLowCount >= CONFIG.DOWNGRADE_CONFIRM_COUNT) {
            performDowngrade(`${source} low: ${kbps} kbps -> ${targetStream}`, false, kbps);
            ABRState.consecutiveProbeLowCount = 0;
        }
        return true;
    }

    ABRState.consecutiveProbeLowCount = 0;
    return false;
}

async function probeAvailableBandwidth() {
    const urls = resolveBandwidthTestUrls();

    for (const baseUrl of urls) {
        const separator = baseUrl.includes('?') ? '&' : '?';
        const probeUrl = `${baseUrl}${separator}_probe=${Date.now()}`;
        const rawKbps = await probeBandwidth(probeUrl);
        const kbps = rawKbps === null ? null : parseFloat(rawKbps);
        if (kbps > 0) {
            ABRState.probedBandwidthKbps = kbps;
            ABRState.lastProbeTime = Date.now();
            ABRState.zeroBandwidthBackoffUntil = 0;
            logBandwidthProbeResult(kbps, baseUrl);
            return kbps;
        }
        console.warn(`[BW] probe source unavailable, trying fallback if configured: ${baseUrl}`);
    }

    ABRState.lastProbeTime = Date.now();
    markNetworkIssue('bandwidth probe unavailable');
    console.warn('[BW] All bandwidth probe sources unavailable; keep current stream.');
    return null;
}

function getPlayerVolume() {
    return playerMuted ? 0 : 1;
}

function updateMuteButton() {
    const icon = document.querySelector('#mutePlay i');
    if (!icon) return;
    icon.textContent = playerMuted ? 'volume_off' : 'volume_up';
}

function applyPlayerAudioSettings() {
    if (!tcplayer) return;
    try {
        if (typeof tcplayer.muted === 'function') {
            tcplayer.muted(playerMuted);
        }
    } catch (e) {}
    try {
        if (typeof tcplayer.volume === 'function') {
            tcplayer.volume(getPlayerVolume());
        }
    } catch (e) {}
    updateMuteButton();
}

function togglePlayerMuted() {
    playerMuted = !playerMuted;
    setCookie('phil_player_muted', playerMuted ? '1' : '0', 30);
    applyPlayerAudioSettings();
    console.log(`[PlayerAudio] user ${playerMuted ? 'muted' : 'unmuted'} player, volume=${getPlayerVolume()}`);
}

function compactAudioStats(audio) {
    if (!audio || Object.keys(audio).length === 0) return 'none';
    const parts = [];
    ['bitrate', 'bytesReceived', 'packetsReceived', 'packetsLost', 'jitter', 'sampleRate', 'audioLevel'].forEach(key => {
        if (audio[key] !== undefined && audio[key] !== null && audio[key] !== '') {
            parts.push(`${key}=${audio[key]}`);
        }
    });
    return parts.length ? parts.join(', ') : JSON.stringify(audio);
}

function logAudioStats(data) {
    const now = Date.now();
    if (now - lastAudioDebugLogTime < CONFIG.AUDIO_DEBUG_LOG_INTERVAL_MS) return;
    lastAudioDebugLogTime = now;
    const audio = data && data.audio;
    const hasAudioStats = !!(audio && Object.keys(audio).length > 0);
    const bitrate = audio && Number.isFinite(Number(audio.bitrate)) ? Math.round(Number(audio.bitrate) / 1000) : 0;
    if (!hasAudioStats || bitrate <= 0) {
        console.warn(`[PlayerAudio] no active audio stats from Tencent player. stream=${ABRState.currentStream}, muted=${playerMuted}, volume=${getPlayerVolume()}, stats=${compactAudioStats(audio)}`);
        return;
    }
    console.log(`[PlayerAudio] stream=${ABRState.currentStream}, muted=${playerMuted}, volume=${getPlayerVolume()}, bitrate=${bitrate} kbps, stats=${compactAudioStats(audio)}`);
}

function logBandwidthProbeResult(kbps, baseUrl) {
    const now = Date.now();
    if (now - ABRState.lastBandwidthLogTime < CONFIG.BANDWIDTH_LOG_INTERVAL_MS) return;
    ABRState.lastBandwidthLogTime = now;
    console.log(`[BW] Available bandwidth probe result: ${kbps} kbps`);
}

function logAbrProbeProgress(message) {
    const now = Date.now();
    if (now - ABRState.lastAbrProbeLogTime < CONFIG.ABR_PROBE_LOG_INTERVAL_MS) return;
    ABRState.lastAbrProbeLogTime = now;
    console.log(message);
}

function resolveBandwidthTestUrls() {
    const configured = Array.isArray(CONFIG.BANDWIDTH_TEST_URLS)
        ? CONFIG.BANDWIDTH_TEST_URLS
        : [CONFIG.BANDWIDTH_TEST_URL].filter(Boolean);
    const urls = [];
    const add = value => {
        if (!value) return;
        try {
            const resolved = new URL(value, window.location.href).href;
            if (!urls.includes(resolved)) urls.push(resolved);
        } catch (e) {}
    };

    configured.forEach(add);
    return urls;
}

function playbackHealthyForUpgrade() {
    const now = Date.now();
    if (now < ABRState.upgradeBackoffUntil) return false;
    if (ABRState.lastStallTime && now - ABRState.lastStallTime < CONFIG.UPGRADE_PROBATION_MS) return false;
    if (ABRState.lastErrorTime && now - ABRState.lastErrorTime < CONFIG.UPGRADE_PROBATION_MS) return false;
    return true;
}

async function maybeProbeAndUpgrade() {
    if (!autoSelectActive || ABRState.isSwitching || ABRState.probeInFlight) return;
    if (!playbackHealthyForUpgrade()) {
        ABRState.consecutiveHighCount = 0;
        return;
    }

    const currentIndex = getCurrentStreamIndex();
    if (currentIndex < 0) {
        ABRState.consecutiveHighCount = 0;
        return;
    }

    const now = Date.now();
    if (now < ABRState.zeroBandwidthBackoffUntil) {
        console.log(`[BW] Skip probe after 0 kbps result until ${new Date(ABRState.zeroBandwidthBackoffUntil).toISOString()}`);
        return;
    }
    if (now - ABRState.lastProbeTime < CONFIG.UPGRADE_PROBE_INTERVAL_MS) return;

    let kbps = null;
    ABRState.probeInFlight = true;
    try {
        kbps = await probeAvailableBandwidth();
    } finally {
        ABRState.probeInFlight = false;
    }
    if (kbps === null || kbps <= 0) return;

    if (checkCooldown()) {
        resetAbrCounters();
        return;
    }

    if (maybeDowngradeByBandwidth(kbps, 'bandwidth probe')) {
        return;
    }

    const currentProfile = STREAM_LADDER[currentIndex];
    if (getQualityRank(currentProfile) >= QUALITY_ORDER.length - 1) {
        ABRState.consecutiveHighCount = 0;
        return;
    }

    const threshold = UPGRADE_THRESHOLDS_KBPS[getQualityKey(currentProfile)];
    if (!threshold) return;

    const nextStream = getNextQualityStreamName(currentProfile);
    if (!nextStream) return;

    if (kbps >= threshold) {
        ABRState.consecutiveHighCount++;
        ABRState.consecutiveLowCount = 0;
        ABRState.consecutiveProbeLowCount = 0;
        logAbrProbeProgress(`[ABR] Upgrade probe good x${ABRState.consecutiveHighCount}/${CONFIG.UPGRADE_CONFIRM_COUNT}: ${kbps} >= ${threshold} kbps`);
        if (ABRState.consecutiveHighCount >= CONFIG.UPGRADE_CONFIRM_COUNT) {
            performUpgrade(nextStream, `probe bandwidth stable: ${kbps} >= ${threshold} kbps`);
        }
    } else {
        logAbrProbeProgress(`[ABR] Upgrade probe not enough: ${kbps} < ${threshold} kbps`);
        ABRState.consecutiveHighCount = 0;
    }
}

/* ------------------------
   ABR 鏍稿績鍒囨崲閫昏緫
   ------------------------ */

function performDowngrade(reason, force = false, measuredKbps = 0) {
    if (!autoSelectActive) return;

    let targetStream = CONFIG.DEFAULT_FALLBACK_STREAM;

    if (force) {
        console.warn(`[ABR] urgent DOWNGRADE triggered! ${reason}. ${ABRState.currentStream} -> ${targetStream}`);
        ABRState.isSwitching = true;
        safeSwitchToStream(targetStream, 'downgrade');
        return;
    }

    if (ABRState.isSwitching) {
        console.warn(`[ABR] Ignored Downgrade (${reason}) because switching is in progress.`);
        return;
    }

    if (checkCooldown()) {
        console.log(`[ABR] Downgrade ignored due to cooldown. Reason: ${reason}`);
        return;
    }

    const currentIndex = getCurrentStreamIndex();
    if (currentIndex <= 0) {
        ABRState.consecutiveLowCount = 0;
        return;
    }

    if (shouldRollbackUpgrade()) {
        targetStream = ABRState.lastUpgradeFromStream;
        ABRState.upgradeBackoffUntil = Date.now() + CONFIG.UPGRADE_BACKOFF_MS;
        console.warn(`[ABR] rollback upgrade during probation. Backoff until ${new Date(ABRState.upgradeBackoffUntil).toISOString()}`);
    } else if (measuredKbps > 0) {
        targetStream = selectDowngradeStreamForCurrent(measuredKbps) || getPreviousQualityStreamName(currentIndex);
    } else {
        targetStream = getPreviousQualityStreamName(currentIndex);
    }

    const targetIndex = STREAM_LADDER.findIndex(s => s.name === targetStream);
    const currentRank = getQualityRank(STREAM_LADDER[currentIndex]);
    const targetRank = getQualityRank(STREAM_LADDER[targetIndex]);
    if (targetStream !== ABRState.currentStream && targetIndex >= 0 && targetRank >= 0 && targetRank < currentRank) {
        console.warn(`[ABR] DOWNGRADE triggered! Reason: ${reason}. ${ABRState.currentStream} -> ${targetStream}`);
        ABRState.isSwitching = true;
        safeSwitchToStream(targetStream, 'downgrade');
    }
}

function performUpgrade(targetStream, reason) {
    if (!autoSelectActive) return;
    if (ABRState.isSwitching) return;
    if (checkCooldown()) {
        console.log(`[ABR] Upgrade ignored due to cooldown. Reason: ${reason}`);
        return;
    }
    if (!playbackHealthyForUpgrade()) {
        console.log(`[ABR] Upgrade ignored because playback is not healthy. Reason: ${reason}`);
        return;
    }

    const currentIndex = getCurrentStreamIndex();
    const currentRank = getQualityRank(STREAM_LADDER[currentIndex]);
    if (currentRank >= QUALITY_ORDER.length - 1) return;

    const targetIndex = STREAM_LADDER.findIndex(s => s.name === targetStream);
    const targetRank = getQualityRank(STREAM_LADDER[targetIndex]);
    if (targetStream && targetIndex >= 0 && targetRank === currentRank + 1) {
        console.log(`[ABR] UPGRADE triggered! Reason: ${reason}. ${ABRState.currentStream} -> ${targetStream}`);
        ABRState.isSwitching = true;
        ABRState.lastUpgradeFromStream = ABRState.currentStream;
        ABRState.lastUpgradeTime = Date.now();
        safeSwitchToStream(targetStream, 'upgrade');
    }
}

/* ------------------------
   Player Lifecycle & Events
   ------------------------ */

const playerHandlers = {
  debug: null,
  webrtcstats: null,
  events: new Map()
};

function destroyPlayer() {
  if (monitorTimer) { clearInterval(monitorTimer); monitorTimer = null; }
  try {
    if (tcplayer) {
      if (playerHandlers.debug) tcplayer.off('debug', playerHandlers.debug);
      if (playerHandlers.webrtcstats) tcplayer.off('webrtcstats', playerHandlers.webrtcstats);
      for (const [evt, h] of playerHandlers.events) {
        tcplayer.off(evt, h);
      }
    }
  } catch(e){}
  try { if (tcplayer && tcplayer.dispose) tcplayer.dispose(); } catch(e){}
  tcplayer = null;
  cleanupPlayerDom();
}

function attachPlayer(options) {
  destroyPlayer();
  applyCanvasLayout(currentCanvasProfile);
  createVideoElementIfMissing();

  const audioOnly = isAudioOnlyStream(ABRState.currentStream, options.source);
  setVideoPlaybackVisible(!audioOnly);
  
  const cfg = {
    autoplay: true,
    webrtcConfig: {
      connectTimeout: 5,
      connectRetryDelay: 1,
      connectRetryCount: 1,
      receiveVideo: !audioOnly,
      receiveAudio: true,
      fallback: false,
      showLog: false
    },
    language: 'zh-CN',
    reportable: false,
    sources: [options.source]
  };

  try { tcplayer = new TCPlayer('player-container-id', cfg); }
  catch (e) {
      console.error('TCPlayer init fail', e);
      ABRState.isSwitching = false;
      return;
  }
  applyPlayerAudioSettings();
  try {
      tcplayer.ready(function() {
          applyPlayerAudioSettings();
          const playResult = tcplayer.play && tcplayer.play();
          if (playResult && typeof playResult.catch === 'function') {
              playResult.catch(err => console.warn(`[PlayerAudio] play request rejected: ${err && err.message ? err.message : err}`));
          }
      });
  } catch (e) {}

  let stBuffTime = 0;
  playerHandlers.debug = async function(event) {
    try {
      const d = event && event.data;
      if (!d) return;
      if (d.code === 1009) {
          stBuffTime = Date.now();
      } 
      else if (d.code === 1010) { // 缂撳啿缁撴潫
          let buffTime = Date.now() - stBuffTime;
          if (buffTime > CONFIG.STALL_THRESHOLD_MS) {
              console.warn(`[Stall] Heavy lag detected: ${buffTime}ms`);
              ABRState.lastStallTime = Date.now();
              markNetworkIssue(`stall ${buffTime}ms`);
              performDowngrade(`Stall > 5000ms (${buffTime}ms)`, false);
              ApiPostPlayLag(currentWebrtcUrl, buffTime);
          }
      }
    } catch(e){}
  };
  tcplayer.on('debug', playerHandlers.debug);

  playerHandlers.webrtcstats = function(event){
    try {
      const data = event.data;
      const vBitrate = (data.video && data.video.bitrate) ? parseInt(data.video.bitrate / 1000) : 0;
      ABRState.currentReceiveKbps = vBitrate; 
      logAudioStats(data);
      
      // 璋冪敤 main.js 涓殑 UI 鏇存柊
      if (typeof onPlayStats === 'function') onPlayStats(data);
    } catch(e){}
  };
  tcplayer.on('webrtcstats', playerHandlers.webrtcstats);

  // 3. 閫氱敤浜嬩欢
  const commonEvents = ['loadstart','error','playing','play','pause','ended'];
  commonEvents.forEach(function(evt){
    const handler = async function(event){
        if (evt === 'play') {
          startPlayTime = Date.now();
        } else if (evt === 'playing') {
          if (!audioOnly) {
            updateCanvasProfile(tcplayer.videoWidth(), tcplayer.videoHeight());
          }
          const diffMs = Date.now() - startPlayTime;
          const playback = describePlaybackStream(tcplayer.videoWidth(), tcplayer.videoHeight());
          console.log(`[Player] playing stream=${playback.stream}, quality=${playback.quality}, level=${playback.level}, codec=${playback.codec}, resolution=${playback.resolution}, loadTime=${diffMs} ms`);
          applyPlayerAudioSettings();
          if (audioOnly) {
            setVideoPlaybackVisible(false);
            console.log('[Player] audio-only stream active, video playback stopped.');
          } else {
            setVideoPlaybackVisible(true);
            unfreezeLastFrame();
          }

          let diffFromLastPlay = -1;
          if (startPlayTimeLast !== -1){
              diffFromLastPlay = startPlayTime - startPlayTimeLast;
          }
          if (diffMs >= 50) {
              ApiPostStartPlay(currentWebrtcUrl, true, "ok", diffMs, diffFromLastPlay, supportsHEVC());
          }
          startPlayTimeLast = new Date();
          
          if (playTimer) clearInterval(playTimer);
          playTimer = setInterval(() => {
              ApiPostEndPlay(currentWebrtcUrl, 6000);
          }, 6000);

          ABRState.isSwitching = false;
          resetAbrCounters();

        } else if (evt === 'error') {
            const errCode = event && event.data && event.data.code;
            const audioOnlyError = isAudioOnlyStream(ABRState.currentStream, currentWebrtcUrl);
            const logFn = audioOnlyError ? console.warn : console.error;
            logFn(`[Player Error] CODE:${errCode}`);
            ABRState.lastErrorTime = Date.now();
            if (errCode === -2002 && typeof showPublicPlayerHint === 'function') {
                showPublicPlayerHint(currentWebrtcUrl, 'TCPlayer -2002 SERVER_ERR');
            }
            
            if (isFatalPlayerError(errCode)) {
                if (audioOnlyError) {
                    console.warn(`[ABR] Ignored player error ${errCode} on audio-only fallback stream.`);
                    clearPlayerErrorState();
                    ABRState.isSwitching = false;
                    return;
                }
                cleanupPlayerDom();
                if (errCode === -2004 && fallbackUnderscoreToLegacy(`Player Error Code: ${errCode}`)) {
                    return;
                }
                if (downgradeHevcForNetworkIssue(`Player Error Code: ${errCode}`)) {
                    return;
                }
                if (fallbackHevcToH264(`Player Error Code: ${errCode}`)) {
                    return;
                }
                // console.error(`[Health] Marking ${ABRState.currentStream} as DEAD (Offline).`);
                // ABRState.isSwitching = false;

                // const currentProfile = getStreamProfile(ABRState.currentStream);
                // if (currentProfile) {
                //     currentProfile.online = false;
                // }
                performDowngrade(`Player Error Code: ${errCode}`, true);
            }
      }
    };
    playerHandlers.events.set(evt, handler);
    tcplayer.on(evt, handler);
  });

  if (!monitorTimer) { 
    startMonitorLoop();
  }
}

/* ------------------------
   ABR 鐩戞帶寰幆
   ------------------------ */
function startMonitorLoop() {
    if (monitorTimer) clearInterval(monitorTimer);

    resetAbrCounters();
    
    monitorTimer = setInterval(async () => {
        if (!tcplayer || !autoSelectActive || !ABRState.currentStream) return;

        if (ABRState.isSwitching) return;

        if (checkCooldown()) {
            resetAbrCounters();
            return;
        }

        const currentIndex = getCurrentStreamIndex();
        if (currentIndex < 0) return;

        await maybeProbeAndUpgrade();
    }, CONFIG.MONITOR_INTERVAL_MS);
}

async function safeSwitchToStream(streamKey, switchType = 'manual') {
  const url = await getWebrtcUrl(streamKey);
  if (!url) {
      console.warn(`[Switch] Failed to get URL for ${streamKey}`);
      ABRState.isSwitching = false;
      return;
  }

  const targetAudioOnly = isAudioOnlyStream(streamKey, url);
  if (targetAudioOnly) {
      freezeLastFrame();
      setVideoPlaybackVisible(false);
  } else {
      freezeLastFrame();
      setVideoPlaybackVisible(true);
  }

  console.log(`[Switch] Executing switch to: ${streamKey}`);
  ABRState.currentStream = streamKey;
  currentWebrtcUrl = url;
  ABRState.lastSwitchTime = Date.now();
  if (switchType !== 'upgrade') {
      ABRState.lastUpgradeFromStream = null;
      ABRState.lastUpgradeTime = 0;
  }
  clearUpgradeProbeState();
  setCookie('phil_stream_choice', streamKey, 1);
  setCookie('phil_stream_url', url, 1);

  attachPlayer({ source: url });
  if (!targetAudioOnly) {
      unfreezeLastFrame();
  }
}
