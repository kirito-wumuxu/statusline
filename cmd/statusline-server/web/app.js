// ==========================================================================
// StatusLine Web Dashboard - Core Application Engine
// Giant Concentric Rings Quota Pool, Workspace Project Grouping, Master-Detail Debug
// ==========================================================================

// Global State
let activeMainTab = 'tab-quota'; // 'tab-quota' | 'tab-sessions' | 'tab-tokens' | 'tab-debug'
let selectedNodeId = '';
let isAliasesOpen = false;
let currentAliasTab = 'quota'; // 'quota' | 'model' | 'account'
let addingNewAlias = false;
let newAliasPrefill = { key: '', name: '', short: '', sort: 0 };
let editingAliasKey = null;
let activeAliases = { quotas: [], models: [], accounts: [] };
let knownNodes = [];
let lastRawQuotas = [];
const expandedSessionIds = new Set();
const rawReportsCache = new Map();
let currentDebugId = null;
let lastRenderCache = { daily: '', quotas: '', sessions: '', overview: '' };
let refreshTimer = null;

function setText(id, text) {
  const el = document.getElementById(id);
  if (el) el.textContent = text;
}

function setHtml(id, html) {
  const el = document.getElementById(id);
  if (el) el.innerHTML = html;
}

// 联动顶部连接指示器与文字状态
function setConnectionState(isConnected, statusText) {
  const indicator = document.getElementById('live-indicator');
  const statusEl = document.getElementById('connection-status');
  if (indicator) {
    if (isConnected) {
      indicator.classList.remove('error');
      indicator.title = '实时连接中';
    } else {
      indicator.classList.add('error');
      indicator.title = '连接异常或中断';
    }
  }
  if (statusEl) {
    statusEl.textContent = statusText;
    statusEl.style.color = isConnected ? '#34d399' : '#f43f5e';
  }
}

// Automatically compute base URL for API calls so it works both at root (/)
// and under reverse proxy subpaths (e.g. /statusline/ or /statusline)
function getApiUrl(path) {
  const cleanPath = path.replace(/^\/+/, '');
  let basePath = window.location.pathname;
  if (basePath.endsWith('.html') || basePath.endsWith('.htm')) {
    basePath = basePath.substring(0, basePath.lastIndexOf('/') + 1);
  } else if (!basePath.endsWith('/')) {
    basePath += '/';
  }
  return basePath + cleanPath;
}

function getAuthHeaders() {
  const headers = { 'Content-Type': 'application/json' };
  const key = localStorage.getItem('statusline_api_key') || new URLSearchParams(window.location.search).get('api_key');
  if (key) {
    headers['X-API-Key'] = key;
    headers['Authorization'] = 'Bearer ' + key;
  }
  return headers;
}

function updateApiKeyBtnText() {
  const hasKey = !!localStorage.getItem('statusline_api_key');
  const btnHeader = document.getElementById('btn-set-api-key');
  if (btnHeader) {
    btnHeader.innerHTML = hasKey ? '🔑 密钥已存' : '🔑 密钥';
    btnHeader.className = hasKey
      ? 'px-3 py-1.5 rounded-lg bg-amber-950/60 text-amber-300 border border-amber-800/60 hover:bg-amber-900/60 transition'
      : 'px-3 py-1.5 rounded-lg bg-slate-800/80 hover:bg-slate-700 text-slate-300 border border-slate-700 transition';
  }
  const btnModal = document.getElementById('btn-set-api-key-modal');
  if (btnModal) {
    btnModal.innerHTML = hasKey ? '🔑 密钥已存' : '🔑 密钥';
    btnModal.className = hasKey
      ? 'btn-action text-amber-300 border-amber-800/80 bg-amber-950/40 hover:bg-amber-900/60'
      : 'btn-action text-slate-300 border-slate-700 hover:bg-slate-800';
  }
}

window.promptApiKey = function() {
  const currentKey = localStorage.getItem('statusline_api_key') || '';
  const newKey = prompt('请输入 statusline-server 服务端 (server.yaml) 中配置的 API Key 认证密钥:\n(如未开启或需清除请直接清空保存)', currentKey);
  if (newKey !== null) {
    const trimmed = newKey.trim();
    if (trimmed) {
      localStorage.setItem('statusline_api_key', trimmed);
    } else {
      localStorage.removeItem('statusline_api_key');
    }
    updateApiKeyBtnText();
    loadAliases();
    refreshDashboard(true);
  }
};

async function requestWithAuth(url, options = {}) {
  const authHeaders = getAuthHeaders();
  if (options.body instanceof FormData) {
    delete authHeaders['Content-Type'];
  }
  options.headers = { ...authHeaders, ...(options.headers || {}) };
  if (options.body instanceof FormData && options.headers['Content-Type'] === 'application/json') {
    delete options.headers['Content-Type'];
  }
  let res = await fetch(url, options);
  if (res.status === 401) {
    const currentKey = localStorage.getItem('statusline_api_key') || '';
    const promptMsg = currentKey
      ? '当前已配置的 API Key 鉴权失败 (401 Unauthorized)。\n请输入服务端 (server.yaml) 正确配置的 API Key 认证密钥:'
      : '服务端已开启安全密钥认证 (401 Unauthorized)。\n请输入服务端 (server.yaml) 中配置的 API Key 认证密钥:';
    const entered = prompt(promptMsg, currentKey);
    if (entered !== null) {
      const trimmed = entered.trim();
      if (trimmed) {
        localStorage.setItem('statusline_api_key', trimmed);
      } else {
        localStorage.removeItem('statusline_api_key');
      }
      updateApiKeyBtnText();
      // 使用最新输入的密钥重试请求
      const retryAuthHeaders = getAuthHeaders();
      if (options.body instanceof FormData) {
        delete retryAuthHeaders['Content-Type'];
      }
      options.headers = { ...retryAuthHeaders, ...(options.headers || {}) };
      if (options.body instanceof FormData && options.headers['Content-Type'] === 'application/json') {
        delete options.headers['Content-Type'];
      }
      res = await fetch(url, options);
    }
  }
  return res;
}

function formatDurationCompact(seconds) {
  if (!seconds || seconds <= 0) return '0m';
  const days = Math.floor(seconds / 86400);
  const remSec = seconds % 86400;
  const hours = Math.floor(remSec / 3600);
  const minutes = Math.floor((remSec % 3600) / 60);
  if (days > 0) return `${days}d${hours}h`;
  if (hours > 0) return `${hours}h${minutes}m`;
  return `${minutes}m`;
}

// 方案 B：弧长计算 (返回 SVG stroke-dasharray="len circumference")
function arcAttrs(r, pct) {
  const c = 2 * Math.PI * r;
  const clamped = Math.min(100, Math.max(0, Number(pct) || 0));
  const len = (clamped / 100) * c;
  return `stroke-dasharray="${len.toFixed(2)} ${c.toFixed(2)}"`;
}

// 解析配额周期的总秒数 (5h: 18000s, 7d: 604800s, 24h: 86400s)
function getWindowTotalSeconds(item, fallbackWindow) {
  if (!item) {
    return fallbackWindow === '7d' ? 7 * 86400 : 5 * 3600;
  }
  const win = (item.window_label || '').toLowerCase();
  const key = (item.quota_key || '').toLowerCase();
  if (win.includes('5h') || key.includes('-5h') || key.includes('-short')) return 5 * 3600;
  if (win.includes('7d') || win.includes('week') || key.includes('-weekly')) return 7 * 86400;
  if (win.includes('24h') || win.includes('1d') || key.includes('-daily')) return 24 * 3600;
  if (win.includes('30d') || win.includes('month') || key.includes('-monthly')) return 30 * 86400;

  const matchH = win.match(/(\d+)h/);
  if (matchH) return parseInt(matchH[1], 10) * 3600;
  const matchD = win.match(/(\d+)d/);
  if (matchD) return parseInt(matchD[1], 10) * 86400;

  const resetSec = Number(item.reset_in_seconds || 0);
  return resetSec > 86400 ? 7 * 86400 : 5 * 3600;
}

// 计算当前周期内的剩余时间百分比 (剩余秒数 / 周期总秒数 * 100)
function calculateTimeRemainingPct(resetSec, totalWinSec) {
  if (!resetSec || resetSec <= 0 || !totalWinSec || totalWinSec <= 0) return 0;
  const pct = (resetSec / totalWinSec) * 100;
  return Math.round(Math.min(100, Math.max(0, pct)));
}

function splitQuotaKeySuffix(key) {
  const lower = (key || '').toLowerCase();
  const suffixes = [
    { suffix: '-5h', window: '5h' },
    { suffix: '-weekly', window: '7d' },
    { suffix: '-daily', window: '24h' },
    { suffix: '-monthly', window: '30d' },
    { suffix: '-short', window: '5h' }
  ];
  for (const s of suffixes) {
    if (lower.endsWith(s.suffix)) {
      return { prefix: key.slice(0, key.length - s.suffix.length), window: s.window };
    }
  }
  return { prefix: key, window: '' };
}

function matchQuotaAlias(rawKey) {
  const { prefix, window } = splitQuotaKeySuffix(rawKey);
  const prefixLower = prefix.toLowerCase();
  let bestMatchLen = -1;
  let bestItem = null;

  for (const item of (activeAliases.quotas || [])) {
    const kLower = (item.key || '').toLowerCase();
    if (prefixLower === kLower || prefixLower.startsWith(kLower) || prefixLower.includes(kLower)) {
      if (kLower.length > bestMatchLen) {
        bestMatchLen = kLower.length;
        bestItem = item;
      }
    }
  }

  if (bestItem) {
    return {
      name: bestItem.name,
      shortName: bestItem.short_name,
      window: window,
      prefix: prefix,
      matched: true
    };
  }

  // Fallback: capitalize prefix
  const capitalized = prefix.charAt(0).toUpperCase() + prefix.slice(1);
  const short = capitalized.length > 2 ? capitalized.slice(0, 2) : capitalized;
  return {
    name: capitalized,
    shortName: short,
    window: window,
    prefix: prefix,
    matched: false
  };
}

function matchModelAlias(modelId, displayName) {
  const idLower = (modelId || '').toLowerCase();
  const nameLower = (displayName || '').toLowerCase();

  for (const item of (activeAliases.models || [])) {
    const kLower = (item.key || '').toLowerCase();
    if (idLower.includes(kLower) || nameLower.includes(kLower)) {
      return { name: item.name, shortName: item.short_name, matched: true };
    }
  }
  return { name: displayName || modelId || 'Gemini', shortName: '', matched: false };
}

function matchAccountAlias(email) {
  const eLower = (email || '').toLowerCase();
  for (const item of (activeAliases.accounts || [])) {
    const kLower = (item.key || '').toLowerCase();
    if (eLower === kLower || eLower.startsWith(kLower)) {
      return {
        name: item.name,
        shortName: item.short_name,
        sortOrder: item.sort_order || 0,
        matched: true
      };
    }
  }
  return { name: email, shortName: '', sortOrder: 0, matched: false };
}

async function loadAliases() {
  try {
    const res = await fetch(getApiUrl('api/v1/config/aliases'), { headers: getAuthHeaders() });
    if (!res.ok) return;
    const data = await res.json();
    activeAliases = {
      quotas: data.quotas || [],
      models: data.models || [],
      accounts: (data.accounts || []).sort((a, b) => {
        const orderA = a.sort_order > 0 ? a.sort_order : 999999;
        const orderB = b.sort_order > 0 ? b.sort_order : 999999;
        if (orderA !== orderB) return orderA - orderB;
        return (a.key || '').localeCompare(b.key || '');
      }),
      priorities: (data.priorities || []).sort((a, b) => {
        const orderA = a.sort_order > 0 ? a.sort_order : 999999;
        const orderB = b.sort_order > 0 ? b.sort_order : 999999;
        return orderA - orderB;
      })
    };
  } catch (e) {
    console.error('[statusline] loadAliases error:', e);
  }
}

function formatTokens(n) {
  if (!n || n <= 0) return '0';
  const num = Number(n);
  if (isNaN(num) || num <= 0) return '0';
  if (num < 1000) return String(Math.round(num));

  const units = [
    { val: 1e12, suffix: 'T' },
    { val: 1e9,  suffix: 'B' },
    { val: 1e6,  suffix: 'M' },
    { val: 1e3,  suffix: 'k' }
  ];

  for (let i = 0; i < units.length; i++) {
    const u = units[i];
    if (num >= u.val * 0.9995) {
      let v = num / u.val;
      let formatted;
      if (v < 10) {
        formatted = v.toFixed(2);
      } else if (v < 100) {
        formatted = v.toFixed(1);
      } else {
        formatted = Math.round(v).toString();
      }
      formatted = formatted.replace(/\.?0+$/, '');
      if (parseFloat(formatted) >= 1000 && i > 0) {
        const nextU = units[i - 1];
        let nextV = num / nextU.val;
        let nextFormatted = nextV < 10 ? nextV.toFixed(2) : (nextV < 100 ? nextV.toFixed(1) : Math.round(nextV).toString());
        nextFormatted = nextFormatted.replace(/\.?0+$/, '');
        return nextFormatted + nextU.suffix;
      }
      return formatted + u.suffix;
    }
  }
  return String(Math.round(num));
}

function formatUSD(n) {
  if (!n || n <= 0) return '$0.00';
  if (n < 0.01) return '$' + n.toFixed(3);
  return '$' + n.toFixed(2);
}

function escapeHtml(str) {
  if (!str) return '';
  return String(str)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

function formatRelativeTime(tsSeconds) {
  if (!tsSeconds) return '--';
  const diffSec = Math.floor(Date.now() / 1000 - tsSeconds);
  if (diffSec < 5) return '刚刚';
  if (diffSec < 60) return `${diffSec}秒前`;
  const diffMin = Math.floor(diffSec / 60);
  if (diffMin < 60) return `${diffMin}分钟前`;
  const diffHour = Math.floor(diffMin / 60);
  if (diffHour < 24) return `${diffHour}小时前`;
  return new Date(tsSeconds * 1000).toLocaleDateString();
}

// JSON 语法高亮渲染器
function syntaxHighlightJSON(rawObj) {
  let jsonStr = typeof rawObj === 'string' ? rawObj : JSON.stringify(rawObj, null, 2);
  let safeJson = escapeHtml(jsonStr);
  return safeJson.replace(/("(\\u[a-zA-Z0-9]{4}|\\[^u]|[^\\"])*"(\s*:)?|\b(true|false|null)\b|-?\d+(?:\.\d*)?(?:[eE][+\-]?\d+)?)/g, function (match) {
    let cls = 'json-number';
    if (/^"/.test(match)) {
      if (/:$/.test(match)) {
        cls = 'json-key';
      } else {
        cls = 'json-string';
      }
    } else if (/true|false/.test(match)) {
      cls = 'json-boolean';
    } else if (/null/.test(match)) {
      cls = 'json-null';
    }
    return '<span class="' + cls + '">' + match + '</span>';
  });
}

// 顶层 Tab 切换
window.switchMainTab = function(tabId) {
  activeMainTab = tabId;
  const tabs = ['tab-quota', 'tab-sessions', 'tab-tokens', 'tab-debug'];
  tabs.forEach(id => {
    const el = document.getElementById(id);
    const btn = document.getElementById('btn-' + id);
    if (el) {
      if (id === tabId) {
        el.classList.remove('hidden');
      } else {
        el.classList.add('hidden');
      }
    }
    if (btn) {
      if (id === tabId) {
        btn.classList.add('active');
      } else {
        btn.classList.remove('active');
      }
    }
  });

  // 如果切到 debug 且尚未加载数据，立即加载
  if (tabId === 'tab-debug') {
    loadDebugRawReports();
  }
};

// 1. 初始化并更新节点下拉筛选器
async function loadNodes() {
  try {
    const res = await fetch(getApiUrl('api/v1/nodes'));
    if (!res.ok) return;
    const nodes = await res.json();
    knownNodes = nodes || [];

    const select = document.getElementById('node-filter-select');
    if (!select) return;

    const currentVal = select.value || selectedNodeId;
    let html = '<option value="">全部节点 (All Nodes)</option>';
    for (const n of knownNodes) {
      const hostLabel = n.hostname ? ` (${escapeHtml(n.hostname)})` : '';
      const osLabel = n.os_type ? ` [${escapeHtml(n.os_type)}]` : '';
      const isSelected = n.node_id === currentVal ? 'selected' : '';
      html += `<option value="${escapeHtml(n.node_id)}" ${isSelected}>${escapeHtml(n.node_id)}${hostLabel}${osLabel}</option>`;
    }
    select.innerHTML = html;
  } catch (e) {
    // 静默忽略
  }
}

// 2. 渲染账号模型配额池 (208px 巨型同心双环)
function renderAccountQuotas(groupedAccounts, forceRedraw = false) {
  const container = document.getElementById('quotas-container');
  if (!container) return;

  const cacheKey = JSON.stringify(groupedAccounts || []);
  if (!forceRedraw && lastRenderCache.quotas === cacheKey) {
    return;
  }
  lastRenderCache.quotas = cacheKey;
  lastRawQuotas = groupedAccounts || [];

  const rawList = groupedAccounts || [];
  const accounts = [...rawList].sort((a, b) => {
    const aliasA = matchAccountAlias(a.account_email || '');
    const aliasB = matchAccountAlias(b.account_email || '');
    const orderA = aliasA.sortOrder > 0 ? aliasA.sortOrder : 999999;
    const orderB = aliasB.sortOrder > 0 ? aliasB.sortOrder : 999999;
    if (orderA !== orderB) return orderA - orderB;
    const timeA = a.updated_at ? new Date(a.updated_at).getTime() : 0;
    const timeB = b.updated_at ? new Date(b.updated_at).getTime() : 0;
    return timeB - timeA;
  });
  setText('badge-quota-count', `${accounts.length} 账号`);
  setText('val-accounts', accounts.length);
  setText('val-accounts-sub', `${accounts.length} 个上报账号`);

  if (accounts.length === 0) {
    container.innerHTML = `<div class="text-slate-400 text-sm p-4 col-span-full text-center">暂未检测到配额信息，终端会话产生交互后将自动同步...</div>`;
    return;
  }

  let html = '';
  let unmappedFound = [];

  for (const acc of accounts) {
    const email = acc.account_email || '未知账号';
    const accAlias = matchAccountAlias(email);
    const displayEmail = accAlias.matched && accAlias.name ? `${accAlias.name} (${email})` : email;
    const nodeId = acc.node_id || acc.last_node_id || 'node_default';
    const updatedStr = acc.updated_at ? new Date(acc.updated_at).toLocaleTimeString() : '--:--:--';

    // 聚合该账号下的所有配额项 (按实体前缀分组)
    const rawItems = acc.buckets || acc.quotas || [];
    const entityMap = new Map();

    for (const q of rawItems) {
      const { prefix, window } = splitQuotaKeySuffix(q.quota_key);
      const alias = matchQuotaAlias(q.quota_key);
      const entityKey = (alias.prefix || prefix || 'unknown').toLowerCase();

      if (!alias.matched && !unmappedFound.includes(alias.prefix)) {
        unmappedFound.push(alias.prefix);
      }

      if (!entityMap.has(entityKey)) {
        entityMap.set(entityKey, {
          key: entityKey,
          displayName: alias.name || prefix.toUpperCase(),
          shortName: alias.shortName || prefix.slice(0, 2).toUpperCase(),
          matched: alias.matched,
          prefix: alias.prefix,
          shortItem: null,
          weeklyItem: null,
          allItems: []
        });
      }
      const ent = entityMap.get(entityKey);
      ent.allItems.push(q);

      const winLower = (window || '').toLowerCase();
      if (winLower === '5h' || winLower === '24h' || (!ent.shortItem && winLower !== '7d' && winLower !== '30d')) {
        ent.shortItem = q;
      }
      if (winLower === '7d' || winLower === '30d' || (ent.shortItem && ent.shortItem !== q && !ent.weeklyItem)) {
        ent.weeklyItem = q;
      }
    }

    const entities = Array.from(entityMap.values());
    const numEntities = entities.length;
    let spanClass = 'account-span-1';
    let insideColsClass = 'grid-cols-1';

    if (numEntities === 2) {
      spanClass = 'account-span-2';
      insideColsClass = 'grid-cols-1 sm:grid-cols-2';
    } else if (numEntities === 3) {
      spanClass = 'account-span-3';
      insideColsClass = 'grid-cols-1 sm:grid-cols-2 lg:grid-cols-3';
    } else if (numEntities >= 4) {
      spanClass = 'account-span-4';
      insideColsClass = 'grid-cols-1 sm:grid-cols-2 lg:grid-cols-4';
    }

    let entityBoxesHtml = '';
    for (const ent of entities) {
      const shortItem = ent.shortItem || (ent.allItems[0] || null);
      const weeklyItem = ent.weeklyItem || (ent.allItems.length > 1 ? ent.allItems[1] : null);

      const shortFraction = shortItem ? Number(shortItem.remaining_fraction || 0) : 1;
      const shortReset = shortItem ? Number(shortItem.reset_in_seconds || 0) : 0;
      const shortPct = Math.round(Math.min(100, Math.max(0, shortFraction * 100)));

      const weeklyFraction = weeklyItem ? Number(weeklyItem.remaining_fraction || 0) : 1;
      const weeklyReset = weeklyItem ? Number(weeklyItem.reset_in_seconds || 0) : 0;
      const weeklyPct = Math.round(Math.min(100, Math.max(0, weeklyFraction * 100)));

      // 周期总时长 (秒) 与剩余时间百分比 (余%)
      const shortTotalSec = getWindowTotalSeconds(shortItem, '5h');
      const weeklyTotalSec = getWindowTotalSeconds(weeklyItem, '7d');
      const shortTimePct = calculateTimeRemainingPct(shortReset, shortTotalSec);
      const weeklyTimePct = calculateTimeRemainingPct(weeklyReset, weeklyTotalSec);

      // 倒计时与窗口展示文案
      const shortTimeStr = shortReset > 0 ? (shortItem && shortItem.reset_countdown ? shortItem.reset_countdown : formatDurationCompact(shortReset)) : '已重置';
      const weeklyTimeStr = weeklyReset > 0 ? (weeklyItem && weeklyItem.reset_countdown ? weeklyItem.reset_countdown : formatDurationCompact(weeklyReset)) : '已重置';
      const shortWinStr = shortItem && shortItem.window_label ? shortItem.window_label : '5h';
      const weeklyWinStr = weeklyItem && weeklyItem.window_label ? weeklyItem.window_label : '7d';

      // 预警判定：消耗过快预警 (容量余量低于时间余量) 与 低配额预警 (<20%)
      const isShortFastBurn = shortPct < shortTimePct && (shortPct < 50 || shortTimePct > 20);
      const isWeeklyFastBurn = weeklyPct < weeklyTimePct && (weeklyPct < 50 || weeklyTimePct > 20);
      const isFastBurn = isShortFastBurn || isWeeklyFastBurn;
      const isShortLow = shortPct < 20;
      const isWeeklyLow = weeklyPct < 20;
      const isLowQuota = isShortLow || isWeeklyLow;
      const isCritical = shortPct < 10 || weeklyPct < 10;
      const isWarn = isLowQuota || isFastBurn;

      const isShortWarn = isShortLow || isShortFastBurn;
      const isWeeklyWarn = isWeeklyLow || isWeeklyFastBurn;

      // 实体与厂商标识
      const isClaude = ent.displayName.toLowerCase().includes('claude') || ent.key.toLowerCase().includes('3p');
      const providerName = isClaude ? 'Anthropic' : 'Google AI';

      // 色彩映射 (方案 B)
      let shortColor = '#29b6f6'; // Bright Sky
      if (isClaude) {
        shortColor = '#10b981'; // Emerald
      }
      if (shortPct < 10) {
        shortColor = '#f43f5e'; // Rose
      } else if (isShortWarn) {
        shortColor = '#f59e0b'; // Amber
      }

      let weeklyColor = '#0284c7'; // Deep Sky
      if (isClaude) {
        weeklyColor = '#059669'; // Emerald or purple
      }
      if (weeklyPct < 10) {
        weeklyColor = '#e11d48'; // Deep Rose
      } else if (isWeeklyWarn) {
        weeklyColor = '#d97706'; // Deep Amber
      }

      let badgeClass = 'badge-blue';
      if (isClaude) {
        badgeClass = isWarn ? 'badge-orange' : 'badge-emerald';
      } else if (isWarn) {
        badgeClass = 'badge-orange';
      }

      let tagHtml = `<span class="provider-tag">${escapeHtml(providerName)}</span>`;
      if (isCritical) {
        tagHtml = `<span class="warn-tag animate-pulse">● 配额极低告急</span>`;
      } else if (isFastBurn) {
        tagHtml = `<span class="warn-tag animate-pulse">● 消耗过快预警</span>`;
      } else if (isLowQuota) {
        tagHtml = `<span class="warn-tag animate-pulse">● 低配额预警</span>`;
      }

      // 方案 B：主容量环 + 内侧贴边「伴随细时间轨」
      // 5h 组：外主环 r=86 (宽9) + 内贴边时间细环 r=77 (宽2.5)
      // 7d 组：外主环 r=62 (宽9) + 内贴边时间细环 r=53 (宽2.5)
      const svgHtml = `
        <svg viewBox="0 0 200 200" width="100%" height="100%">
          <!-- 5h 外层主容量粗环 (R=86, 宽9) -->
          <circle cx="100" cy="100" r="86" fill="none" stroke="#182232" stroke-width="9"/>
          <circle class="ring-arc" cx="100" cy="100" r="86" fill="none" stroke="${shortColor}" stroke-width="9"
            stroke-linecap="round" transform="rotate(-90 100 100)" ${arcAttrs(86, shortPct)}/>
          <!-- 5h 伴随时间细环 (R=77, 宽2.5) -->
          <circle cx="100" cy="100" r="77" fill="none" stroke="rgba(255,255,255,0.06)" stroke-width="2.5"/>
          <circle class="ring-arc" cx="100" cy="100" r="77" fill="none" stroke="rgba(241,245,249,0.55)" stroke-width="2.5"
            stroke-linecap="round" transform="rotate(-90 100 100)" ${arcAttrs(77, shortTimePct)}/>

          <!-- 7d 内层主容量粗环 (R=62, 宽9) -->
          <circle cx="100" cy="100" r="62" fill="none" stroke="#182232" stroke-width="9"/>
          <circle class="ring-arc" cx="100" cy="100" r="62" fill="none" stroke="${weeklyColor}" stroke-width="9"
            stroke-linecap="round" transform="rotate(-90 100 100)" ${arcAttrs(62, weeklyPct)}/>
          <!-- 7d 伴随时间细环 (R=53, 宽2.5) -->
          <circle cx="100" cy="100" r="53" fill="none" stroke="rgba(255,255,255,0.06)" stroke-width="2.5"/>
          <circle class="ring-arc" cx="100" cy="100" r="53" fill="none" stroke="rgba(241,245,249,0.45)" stroke-width="2.5"
            stroke-linecap="round" transform="rotate(-90 100 100)" ${arcAttrs(53, weeklyTimePct)}/>
        </svg>
      `;

      entityBoxesHtml += `
        <div class="quota-unit-box ${isWarn ? 'warn' : ''}">
          <div class="flex justify-between items-center w-full">
            <span class="badge-tag ${badgeClass}">${escapeHtml(ent.displayName.toUpperCase())}</span>
            ${tagHtml}
          </div>

          <!-- 方案 B：双维度侧环 SVG -->
          <div class="quota-ring-svg-wrap">
            ${svgHtml}
            <div class="ring-center">
              <div class="center-label">${escapeHtml(shortWinStr)} 短期</div>
              <div class="center-val">${shortPct}%</div>
              <div class="center-sub">${escapeHtml(weeklyWinStr)}: ${weeklyPct}%</div>
            </div>
          </div>

          <div class="ring-divider"></div>

          <!-- 双维度时间详情药丸 -->
          <div class="quota-pills">
            <div class="quota-pill ${isShortWarn ? 'pill-warn' : ''}">
              <div class="pill-row">
                <div class="pill-left"><span class="pill-dot" style="background:${shortColor}"></span>短期 (${escapeHtml(shortWinStr)})</div>
                <div class="pill-right">${shortPct}% · ${shortTimeStr} <span class="muted">(余${shortTimePct}%)</span></div>
              </div>
            </div>
            <div class="quota-pill ${isWeeklyWarn ? 'pill-warn' : ''}">
              <div class="pill-row">
                <div class="pill-left"><span class="pill-dot" style="background:${weeklyColor}"></span>周度 (${escapeHtml(weeklyWinStr)})</div>
                <div class="pill-right">${weeklyPct}% · ${weeklyTimeStr} <span class="muted">(余${weeklyTimePct}%)</span></div>
              </div>
            </div>
          </div>
        </div>
      `;
    }

    const sortBadge = accAlias.sortOrder > 0 ? `<span class="px-2 py-0.5 rounded text-xs bg-amber-950 text-amber-400 border border-amber-800/60 mono font-bold shrink-0">#${accAlias.sortOrder}</span>` : '';

    html += `
      <div class="account-quota-card ${spanClass} shadow-lg">
        <div class="flex items-center justify-between border-b border-slate-800 pb-3 mb-4 gap-3 w-full">
          <div class="flex items-center gap-2 truncate max-w-[320px]">
            ${sortBadge}
            <span class="text-amber-400 font-bold text-base truncate" title="${escapeHtml(displayEmail)}">
              👤 ${escapeHtml(displayEmail)}
            </span>
          </div>
          <span class="text-xs text-slate-400 mono bg-slate-900/90 px-2.5 py-1 rounded shrink-0">${nodeId} · ${updatedStr}</span>
        </div>
        <div class="grid ${insideColsClass} gap-4 flex-1 items-stretch">
          ${entityBoxesHtml}
        </div>
      </div>
    `;
  }

  container.innerHTML = html;

  // 未映射实体提示
  const unmappedEl = document.getElementById('quota-unmapped-tip');
  if (unmappedEl) {
    if (unmappedFound.length > 0) {
      unmappedEl.innerHTML = `检测到新前缀: <b>${escapeHtml(unmappedFound.join(', '))}</b> <button class="btn-action btn-sm ml-2" onclick="quickAddAlias('quota', '${escapeHtml(unmappedFound[0])}', '${escapeHtml(unmappedFound[0])}', '${escapeHtml(unmappedFound[0].slice(0, 2))}')">+ 快速映射</button>`;
    } else {
      unmappedEl.innerHTML = '';
    }
  }
}

// 3. 渲染终端会话列表 (按项目工作区 Workspace 聚合分组)
function renderSessions(sessions, forceRedraw = false) {
  const container = document.getElementById('sessions-container');
  if (!container) return;

  const stateToken = Array.from(expandedSessionIds).sort().join(',');
  const cacheKey = JSON.stringify(sessions || []) + '|' + stateToken;
  if (!forceRedraw && lastRenderCache.sessions === cacheKey) {
    return;
  }
  lastRenderCache.sessions = cacheKey;

  const sessList = sessions || [];
  if (sessList.length === 0) {
    container.innerHTML = `<div class="text-slate-400 text-sm p-4 text-center">暂未接收到任何会话遥测数据，请在终端中启动会话...</div>`;
    setText('badge-sessions-count', '0 项目 · 0 会话');
    return;
  }

  // 按项目路径分组聚合
  const projectMap = new Map();
  for (const s of sessList) {
    let projPath = s.project_dir || s.cwd || '~';
    // 统一处理 Windows 反斜杠以供分组
    const normKey = projPath.replace(/\\/g, '/');
    if (!projectMap.has(normKey)) {
      projectMap.set(normKey, {
        rawPath: projPath,
        normPath: normKey,
        sessions: [],
        totalInput: 0,
        totalOutput: 0,
        totalCache: 0,
        nodes: new Set()
      });
    }
    const group = projectMap.get(normKey);
    group.sessions.push(s);
    if (s.tokens) {
      group.totalInput += s.tokens.total_input_tokens || 0;
      group.totalOutput += s.tokens.total_output_tokens || 0;
      group.totalCache += s.tokens.total_cache_read_tokens || 0;
    }
    if (s.node_id) {
      group.nodes.add(s.node_id);
    }
  }

  const projectGroups = Array.from(projectMap.values());
  setText('badge-sessions-count', `${projectGroups.length} 项目 · ${sessList.length} 会话`);
  setText('sessions-filter-tip', `${projectGroups.length} 个项目目录 · 共 ${sessList.length} 个会话`);

  const agyCount = sessList.filter(s => s.source !== 'claudecode' && s.product_type !== 'claude_code').length;
  const claudeCount = sessList.length - agyCount;
  setHtml('val-sources', `${projectGroups.length} 个项目<br>${agyCount} agy / ${claudeCount} claude`);

  // 分离单会话项目 (数量=1) 与多会话大项目 (数量>1)
  const singleProjects = projectGroups.filter(g => g.sessions.length === 1);
  const multiProjects = projectGroups.filter(g => g.sessions.length > 1);

  let html = '';
  let groupIndex = 1;

  // 辅助函数：渲染单个会话卡片
  function renderSessionCard(sess) {
    const isClaude = sess.source === 'claudecode' || sess.product_type === 'claude_code';
    const sourceClass = isClaude ? 'bg-amber-950 text-amber-300 border border-amber-800/60' : 'bg-teal-950 text-teal-300 border border-teal-800/60';
    const sourceName = isClaude ? 'ClaudeCode' : 'Antigravity';
    const isWorking = sess.agent_state === 'working';
    const stateClass = isWorking ? 'bg-amber-950/60 text-amber-400' : 'bg-emerald-950/60 text-emerald-400';
    const stateName = isWorking ? '⚡ 运行中' : '空闲';

    const nodeId = sess.node_id || 'node_default';
    const updatedRelative = formatRelativeTime(sess.timestamp);
    const updatedClock = sess.timestamp ? new Date(sess.timestamp * 1000).toLocaleTimeString() : '--:--';
    const updatedFull = sess.timestamp ? new Date(sess.timestamp * 1000).toLocaleString() : '--';

    const inTokens = formatTokens(sess.tokens ? sess.tokens.total_input_tokens : 0);
    const outTokens = formatTokens(sess.tokens ? sess.tokens.total_output_tokens : 0);
    const cacheTokens = formatTokens(sess.tokens ? (sess.tokens.total_cache_read_tokens || 0) : 0);
    const rawMId = sess.model ? (sess.model.id || '') : '';
    const rawMDisp = sess.model ? (sess.model.display_name || '') : '';
    const matchedM = matchModelAlias(rawMId, rawMDisp);
    const modelName = matchedM.name || rawMDisp || rawMId || 'Gemini';

    const title = sess.conversation_title || sess.session_name || '默认终端会话';
    const email = (sess.account && sess.account.email) ? sess.account.email : '';
    const accAlias = matchAccountAlias(email);
    const displayEmail = accAlias.matched && accAlias.name ? `${accAlias.name} (${email})` : (email || '本地匿名用户');

    const sId = sess.session_id || sess.id || '';
    const isExpanded = expandedSessionIds.has(sId);

    // 上下文占比
    let ctxPct = 0;
    let ctxCap = '1M';
    if (sess.context_window) {
      ctxPct = Math.round(sess.context_window.used_percentage || 0);
      const size = sess.context_window.context_window_size || 1048576;
      ctxCap = size >= 1000000 ? `${(size / 1000000).toFixed(0)}M` : `${Math.round(size / 1000)}k`;
    }

    return `
      <div data-node="${escapeHtml(nodeId)}" onclick="toggleSessionExpand('${escapeHtml(sId)}', event)" class="session-card sub-card-bg p-3.5 rounded-xl space-y-2 hover:border-sky-500/60 transition cursor-pointer">
        <div class="flex justify-between items-center">
          <div class="flex items-center gap-1.5">
            <span class="px-2 py-0.5 rounded ${sourceClass} text-[11px]">${sourceName}</span>
            <span class="px-2 py-0.5 rounded ${stateClass} text-[11px]">${stateName}</span>
          </div>
          <span class="text-[11px] text-indigo-300 bg-indigo-950/60 px-2 py-0.5 rounded mono">${escapeHtml(nodeId)}</span>
        </div>
        <div class="flex justify-between items-center">
          <span class="font-bold text-slate-200 text-xs truncate max-w-[200px]" title="${escapeHtml(displayEmail)}">👤 ${escapeHtml(displayEmail)}</span>
          <span class="text-[11px] text-slate-400">${updatedRelative} (${updatedClock})</span>
        </div>
        <div class="flex justify-between items-center text-[11px] pt-0.5">
          <span class="text-slate-300 truncate max-w-[160px]" title="${escapeHtml(title)}">${escapeHtml(title)}</span>
          <span class="text-sky-400 mono">[${escapeHtml(modelName)}]</span>
        </div>
        <div class="flex justify-between items-center pt-2 text-[11px] mono text-slate-400 border-t border-slate-800/80">
          <span>↑ <b class="text-emerald-400">${inTokens}</b> · ↓ <b class="text-rose-400">${outTokens}</b> · ⚡ <b class="text-amber-300">${cacheTokens}</b></span>
          <span>上下文 <b class="text-white">${ctxPct}% [${ctxCap}]</b></span>
        </div>
        ${isExpanded ? `
          <div class="mt-3 pt-2.5 border-t border-slate-800 text-[11px] text-slate-400 space-y-1 mono bg-slate-950/40 p-2.5 rounded-lg">
            <div>会话 ID: <span class="text-slate-300 select-all">${escapeHtml(sId)}</span></div>
            ${sess.conversation_id ? `<div>对话 ID: <span class="text-slate-300 select-all">${escapeHtml(sess.conversation_id)}</span></div>` : ''}
            <div>完整时间: <span class="text-slate-300">${escapeHtml(updatedFull)}</span></div>
            ${sess.git_branch ? `<div>Git 分支: <span class="text-indigo-300">${escapeHtml(sess.git_branch)}${sess.git_dirty ? '*' : ''}</span></div>` : ''}
            ${sess.account && sess.account.plan_tier ? `<div>账号计划: <span class="text-amber-400">${escapeHtml(sess.account.plan_tier)}</span></div>` : ''}
          </div>
        ` : ''}
      </div>
    `;
  }

  // 1) 渲染单会话项目 (在 3 列并排网格中)
  if (singleProjects.length > 0) {
    html += `<div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">`;
    for (const group of singleProjects) {
      const gId = `proj-${groupIndex++}`;
      const pathParts = group.normPath.split('/');
      const lastPart = pathParts.pop() || '';
      const prefixPath = pathParts.length > 0 ? pathParts.join('/') + '/' : '';

      html += `
        <div class="project-group group-bg rounded-xl overflow-hidden flex flex-col justify-between border border-sky-900/40">
          <div onclick="toggleGroup('${gId}')" class="bg-slate-900/90 px-4 py-3 border-b border-slate-800 flex items-center justify-between cursor-pointer hover:bg-slate-800/70 transition">
            <div class="flex items-center gap-2 min-w-0">
              <span class="text-amber-400 text-base">📂</span>
              <div class="truncate">
                <div class="text-xs font-bold text-sky-300 mono truncate" title="${escapeHtml(group.rawPath)}">
                  ${escapeHtml(prefixPath)}<span class="text-white underline decoration-sky-500/60">${escapeHtml(lastPart)}</span>
                </div>
                <div class="text-[11px] text-slate-400 mt-0.5 flex items-center gap-2 mono">
                  <span>1 个活跃会话</span> · <span>汇总: <b class="text-emerald-400">↑${formatTokens(group.totalInput)}</b> <b class="text-rose-400">↓${formatTokens(group.totalOutput)}</b> <b class="text-amber-300">⚡${formatTokens(group.totalCache)}</b></span>
                </div>
              </div>
            </div>
            <span id="icon-${gId}" class="text-slate-400 text-xs ml-2">▼</span>
          </div>

          <div id="${gId}" class="project-body p-3.5">
            ${renderSessionCard(group.sessions[0])}
          </div>
        </div>
      `;
    }
    html += `</div>`;
  }

  // 2) 渲染多会话大项目 (全屏独占大卡片，内部以 3 列网格排布)
  for (const group of multiProjects) {
    const gId = `proj-${groupIndex++}`;
    const pathParts = group.normPath.split('/');
    const lastPart = pathParts.pop() || '';
    const prefixPath = pathParts.length > 0 ? pathParts.join('/') + '/' : '';

    let cardsHtml = '';
    for (const sess of group.sessions) {
      cardsHtml += renderSessionCard(sess);
    }

    html += `
      <div class="project-group group-bg rounded-xl overflow-hidden border border-slate-800 mt-4">
        <div onclick="toggleGroup('${gId}')" class="bg-slate-900/90 px-5 py-3.5 border-b border-slate-800 flex flex-col sm:flex-row sm:items-center justify-between gap-2 cursor-pointer hover:bg-slate-800/70 transition">
          <div class="flex items-center gap-3 min-w-0">
            <span class="text-amber-400 text-lg">📂</span>
            <div class="truncate">
              <div class="text-sm font-bold text-sky-300 mono flex items-center gap-2 truncate" title="${escapeHtml(group.rawPath)}">
                <span class="truncate">${escapeHtml(prefixPath)}<span class="text-white underline decoration-sky-500/60">${escapeHtml(lastPart)}</span></span>
                <span class="px-2 py-0.5 text-xs rounded-full bg-emerald-950 text-emerald-400 border border-emerald-800/60 shrink-0">${group.sessions.length} 个并发会话</span>
              </div>
            </div>
          </div>
          <div class="flex items-center gap-5 text-xs mono text-slate-400 shrink-0">
            <span>涉及节点: <b class="text-slate-200">${group.nodes.size} 个</b></span>
            <span>项目总吞吐: <b class="text-emerald-400">↑ ${formatTokens(group.totalInput)}</b> <b class="text-rose-400">↓ ${formatTokens(group.totalOutput)}</b> <b class="text-amber-300">⚡ ${formatTokens(group.totalCache)}</b></span>
            <span id="icon-${gId}" class="text-slate-400 text-xs">▼</span>
          </div>
        </div>

        <div id="${gId}" class="project-body p-4 grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3.5">
          ${cardsHtml}
        </div>
      </div>
    `;
  }

  container.innerHTML = html;

  // 如果当前有已选中的节点，执行过滤
  if (selectedNodeId) {
    filterSessions();
  }
}

// 折叠/展开单个项目
window.toggleGroup = function(id) {
  const body = document.getElementById(id);
  const icon = document.getElementById('icon-' + id);
  if (!body) return;
  const isHidden = body.classList.contains('hidden');
  if (isHidden) {
    body.classList.remove('hidden');
    if (icon) icon.innerText = '▼';
  } else {
    body.classList.add('hidden');
    if (icon) icon.innerText = '▶';
  }
};

// 全部展开/折叠项目
window.toggleAllProjects = function(expand) {
  document.querySelectorAll('.project-body').forEach(body => {
    const id = body.id;
    const icon = document.getElementById('icon-' + id);
    if (expand) {
      body.classList.remove('hidden');
      if (icon) icon.innerText = '▼';
    } else {
      body.classList.add('hidden');
      if (icon) icon.innerText = '▶';
    }
  });
};

// 按物理节点过滤会话（自动隐藏没有匹配会话的项目组）
window.filterSessions = function() {
  const select = document.getElementById('node-filter-select');
  const selected = select ? select.value : '';
  selectedNodeId = selected;

  document.querySelectorAll('.project-group').forEach(group => {
    let visibleCount = 0;
    group.querySelectorAll('.session-card').forEach(card => {
      const node = card.getAttribute('data-node') || '';
      if (!selected || node === selected) {
        card.style.display = '';
        visibleCount++;
      } else {
        card.style.display = 'none';
      }
    });
    group.style.display = visibleCount > 0 ? '' : 'none';
  });
};

// 切换单条会话详情卡片
window.toggleSessionExpand = function(sessionId, event) {
  if (event) event.stopPropagation();
  if (expandedSessionIds.has(sessionId)) {
    expandedSessionIds.delete(sessionId);
  } else {
    expandedSessionIds.add(sessionId);
  }
  // 重新渲染当前缓存的会话
  if (lastRenderCache.rawSessions) {
    renderSessions(lastRenderCache.rawSessions, true);
  }
};

// 每日模型 Token 分析与堆叠图过滤状态
let dailyFilterState = {
  days: 14,
  startDate: null,
  endDate: null,
  selectedModels: new Set(),
  allModels: []
};
let rawDailyTokensData = [];

const MODEL_PALETTE = [
  '#38bdf8', // Sky Blue (Gemini Flash)
  '#10b981', // Emerald (Claude Sonnet)
  '#a855f7', // Purple (GPT-OSS)
  '#f59e0b', // Warm Amber (Argon)
  '#6366f1', // Indigo (Hydrogen)
  '#ec4899', // Pink
  '#06b6d4', // Cyan
  '#f97316', // Orange
  '#14b8a6', // Teal
  '#8b5cf6'  // Violet
];

const modelColorMap = new Map();
function getModelColor(modelName, index = 0) {
  const lower = (modelName || '').toLowerCase();
  if (lower.includes('flash') || (lower.includes('gemini') && !lower.includes('argon') && !lower.includes('hydrogen'))) {
    return '#38bdf8';
  }
  if (lower.includes('claude')) {
    return '#10b981';
  }
  if (lower.includes('argon')) {
    return '#f59e0b';
  }
  if (lower.includes('hydrogen')) {
    return '#6366f1';
  }
  if (lower.includes('gpt-oss') || lower.includes('oss')) {
    return '#a855f7';
  }
  if (lower.includes('deepseek')) {
    return '#06b6d4';
  }
  if (modelColorMap.has(modelName)) {
    return modelColorMap.get(modelName);
  }
  const color = MODEL_PALETTE[index % MODEL_PALETTE.length];
  modelColorMap.set(modelName, color);
  return color;
}

window.setTimeRange = function(days) {
  dailyFilterState.days = Number(days);
  dailyFilterState.startDate = null;
  dailyFilterState.endDate = null;

  const startInput = document.getElementById('date-filter-start');
  const endInput = document.getElementById('date-filter-end');
  if (startInput) startInput.value = '';
  if (endInput) endInput.value = '';

  document.querySelectorAll('#time-range-buttons .time-range-btn').forEach(btn => {
    const d = Number(btn.getAttribute('data-days'));
    btn.classList.toggle('active', d === Number(days));
  });

  renderDailyTokensView(rawDailyTokensData, true);
};

window.handleCustomDateChange = function() {
  const startVal = document.getElementById('date-filter-start')?.value || '';
  const endVal = document.getElementById('date-filter-end')?.value || '';
  if (!startVal && !endVal) return;

  dailyFilterState.startDate = startVal || null;
  dailyFilterState.endDate = endVal || null;

  document.querySelectorAll('#time-range-buttons .time-range-btn').forEach(btn => {
    btn.classList.remove('active');
  });

  renderDailyTokensView(rawDailyTokensData, true);
};

window.resetDateRangeFilter = function() {
  window.setTimeRange(14);
};

window.toggleModelFilter = function(modelId) {
  if (dailyFilterState.selectedModels.has(modelId)) {
    dailyFilterState.selectedModels.delete(modelId);
  } else {
    dailyFilterState.selectedModels.add(modelId);
  }
  renderDailyTokensView(rawDailyTokensData, true);
};

window.selectAllModels = function(selectAll) {
  if (selectAll) {
    dailyFilterState.selectedModels = new Set(dailyFilterState.allModels);
  } else {
    dailyFilterState.selectedModels = new Set();
  }
  renderDailyTokensView(rawDailyTokensData, true);
};

// 4. 渲染每日各模型 Token 综合视图 (叠加柱状图 + 过滤器 + 明细表)
function renderDailyTokensView(rows, forceRedraw = false) {
  rawDailyTokensData = rows || [];
  const items = rawDailyTokensData;

  // 1. 发现所有在历史数据中出现的模型
  const discoveredModels = new Set();
  for (const r of items) {
    if (r.models && Array.isArray(r.models)) {
      for (const m of r.models) {
        if (m.model_id) discoveredModels.add(m.model_id);
      }
    }
  }

  const modelList = Array.from(discoveredModels);
  dailyFilterState.allModels = modelList;

  // 初次加载时，默认全选所有模型
  if (dailyFilterState.selectedModels.size === 0 && modelList.length > 0) {
    dailyFilterState.selectedModels = new Set(modelList);
  }

  // 2. 按时间范围筛选
  let filteredItems = items;
  if (dailyFilterState.startDate || dailyFilterState.endDate) {
    filteredItems = items.filter(r => {
      if (dailyFilterState.startDate && r.date < dailyFilterState.startDate) return false;
      if (dailyFilterState.endDate && r.date > dailyFilterState.endDate) return false;
      return true;
    });
  } else {
    filteredItems = items.slice(0, dailyFilterState.days);
  }

  // 3. 计算时间范围内的各模型消耗总额 (用于展示在 Chips 标签上)
  const modelTotals = new Map();
  for (const r of filteredItems) {
    if (r.models && Array.isArray(r.models)) {
      for (const m of r.models) {
        const cur = modelTotals.get(m.model_id) || 0;
        modelTotals.set(m.model_id, cur + (m.total_tokens || 0));
      }
    }
  }

  // 4. 过滤并聚合每日数据
  const dateGroups = new Map();
  let rangeTotalTokens = 0;
  let peakTokens = 0;
  let peakDate = '--';

  for (const r of filteredItems) {
    const d = r.date || '未知日期';
    if (!dateGroups.has(d)) {
      dateGroups.set(d, {
        date: d,
        totalIn: 0,
        totalOut: 0,
        totalCache: 0,
        totalTokens: 0,
        models: []
      });
    }
    const grp = dateGroups.get(d);
    const dayModels = r.models || [];

    for (const m of dayModels) {
      if (dailyFilterState.selectedModels.has(m.model_id)) {
        grp.models.push(m);
        grp.totalIn += (m.input_tokens || 0);
        grp.totalOut += (m.output_tokens || 0);
        grp.totalCache += (m.cache_read_tokens || 0);
        grp.totalTokens += (m.total_tokens || 0);
      }
    }

    rangeTotalTokens += grp.totalTokens;
    if (grp.totalTokens > peakTokens) {
      peakTokens = grp.totalTokens;
      peakDate = d;
    }
  }

  // 5. 更新顶部指标卡片
  const dayCount = dateGroups.size;
  const avgTokens = dayCount > 0 ? Math.round(rangeTotalTokens / dayCount) : 0;
  setText('range-total-tokens', formatTokens(rangeTotalTokens));
  setText('range-avg-tokens', `${formatTokens(avgTokens)}/天`);
  setText('range-peak-tokens', peakTokens > 0 ? `${formatTokens(peakTokens)} (${peakDate.slice(5)})` : '--');

  // 6. 渲染模型筛选 Chips
  renderModelFilterChips(modelList, modelTotals);

  // 7. 渲染叠加柱状图
  renderStackedBarChart(dateGroups);

  // 8. 渲染每日明细表格
  renderDailyTokensTable(dateGroups);
}

function renderModelFilterChips(allModels, modelTotals) {
  const container = document.getElementById('model-filter-chips');
  if (!container) return;

  container.innerHTML = allModels.map(mId => {
    const isSelected = dailyFilterState.selectedModels.has(mId);
    const color = getModelColor(mId);
    const alias = matchModelAlias(mId, mId);
    const displayName = alias.matched && alias.name ? alias.name : mId;
    const total = modelTotals.get(mId) || 0;

    return `
      <button type="button" onclick="toggleModelFilter('${escapeHtml(mId)}')"
              class="model-chip ${isSelected ? 'active' : 'inactive'}"
              title="点击切换此模型 (当前区间消耗: ${formatTokens(total)})">
        <span class="w-2 h-2 rounded-full shrink-0" style="background:${color}"></span>
        <span class="truncate max-w-[150px]">${escapeHtml(displayName)}</span>
        <span class="mono text-[10px] opacity-75">${formatTokens(total)}</span>
      </button>
    `;
  }).join('');
}

function renderStackedBarChart(dateGroups) {
  const container = document.getElementById('stacked-bar-chart-container');
  const legendContainer = document.getElementById('chart-legend-summary');
  if (!container) return;

  const days = Array.from(dateGroups.values()).reverse(); // 顺序按时间先后从左向右排列
  if (days.length === 0) {
    container.innerHTML = `<div class="flex items-center justify-center h-full text-slate-500 text-xs">当前筛选范围内无有效数据</div>`;
    if (legendContainer) legendContainer.innerHTML = '';
    return;
  }

  let maxDailyTokens = 0;
  for (const d of days) {
    if (d.totalTokens > maxDailyTokens) {
      maxDailyTokens = d.totalTokens;
    }
  }
  if (maxDailyTokens <= 0) maxDailyTokens = 100000;

  const N = days.length;
  const minSlot = 56;
  const chartWidth = Math.max(720, N * minSlot + 90);
  const chartHeight = 280;
  const padLeft = 65;
  const padRight = 30;
  const padTop = 32;
  const padBottom = 40;
  const innerW = chartWidth - padLeft - padRight;
  const innerH = chartHeight - padTop - padBottom;
  const slotW = innerW / N;
  const barW = Math.min(42, Math.max(14, slotW * 0.62));

  let svgHtml = `<svg viewBox="0 0 ${chartWidth} ${chartHeight}" width="${chartWidth}" height="${chartHeight}" class="w-full h-full block select-none">`;

  // 绘制 5 条水平网格标线与 Y 轴刻度
  const ticks = [0, 0.25, 0.5, 0.75, 1.0];
  for (const t of ticks) {
    const yPos = padTop + innerH * (1 - t);
    const val = maxDailyTokens * t;
    svgHtml += `
      <line x1="${padLeft}" y1="${yPos.toFixed(1)}" x2="${(chartWidth - padRight).toFixed(1)}" y2="${yPos.toFixed(1)}" stroke="#1e293b" stroke-dasharray="3 3"/>
      <text x="${(padLeft - 8)}" y="${(yPos + 4).toFixed(1)}" text-anchor="end" class="mono text-[10px] fill-slate-500 font-normal">${formatTokens(val)}</text>
    `;
  }

  const todayStr = new Date().toISOString().slice(0, 10);

  // 绘制每根柱子与分层模型
  days.forEach((day, idx) => {
    const centerX = padLeft + idx * slotW + slotW / 2;
    const barX = centerX - barW / 2;
    const isToday = day.date === todayStr;
    const dateLabel = day.date.slice(5);

    // 当日整列半透明悬浮感应区
    svgHtml += `
      <rect x="${(padLeft + idx * slotW).toFixed(1)}" y="${padTop}" width="${slotW.toFixed(1)}" height="${innerH}"
            fill="transparent" class="day-hover-band" data-date="${day.date}" data-daytotal="${day.totalTokens}"/>
    `;

    // 堆叠各模型分块 (自底向上累加)
    let currentY = padTop + innerH;
    const modelsToStack = [...day.models].filter(m => (m.total_tokens || 0) > 0);

    modelsToStack.forEach((m, sIdx) => {
      const segH = Math.max(2, ((m.total_tokens || 0) / maxDailyTokens) * innerH);
      const segY = currentY - segH;
      const color = getModelColor(m.model_id);
      const isTop = (sIdx === modelsToStack.length - 1);
      const rx = isTop ? '3' : '1';

      const rawModel = m.model_id || '';
      const alias = matchModelAlias(rawModel, m.display_name || rawModel);
      const displayName = alias.matched && alias.name ? alias.name : (m.display_name || rawModel);

      svgHtml += `
        <rect x="${barX.toFixed(1)}" y="${segY.toFixed(1)}" width="${barW.toFixed(1)}" height="${segH.toFixed(1)}"
              fill="${color}" rx="${rx}" class="stacked-bar-segment"
              data-date="${day.date}"
              data-model="${escapeHtml(displayName)}"
              data-in="${m.input_tokens || 0}"
              data-out="${m.output_tokens || 0}"
              data-cache="${m.cache_read_tokens || 0}"
              data-total="${m.total_tokens || 0}"
              data-daytotal="${day.totalTokens}"
              data-color="${color}"/>
      `;
      currentY = segY;
    });

    // 柱顶当日总量文字
    if (day.totalTokens > 0) {
      svgHtml += `
        <text x="${centerX.toFixed(1)}" y="${(currentY - 6).toFixed(1)}" text-anchor="middle"
              class="mono text-[10px] font-bold ${isToday ? 'fill-sky-400' : 'fill-slate-300'}">${formatTokens(day.totalTokens)}</text>
      `;
    }

    // X 轴日期标注
    svgHtml += `
      <text x="${centerX.toFixed(1)}" y="${(padTop + innerH + 18).toFixed(1)}" text-anchor="middle"
            class="mono text-xs ${isToday ? 'fill-sky-400 font-bold' : 'fill-slate-400'}">${dateLabel}${isToday ? ' (今)' : ''}</text>
    `;
  });

  svgHtml += `</svg>`;
  container.innerHTML = svgHtml;

  // 渲染右上方动态图例
  if (legendContainer) {
    const activeModels = Array.from(dailyFilterState.selectedModels);
    legendContainer.innerHTML = activeModels.map(mId => {
      const color = getModelColor(mId);
      const alias = matchModelAlias(mId, mId);
      const name = alias.matched && alias.name ? alias.name : mId;
      return `
        <span class="flex items-center gap-1.5 text-slate-300">
          <i class="w-2 h-2 rounded-full inline-block" style="background:${color}"></i>
          <span>${escapeHtml(name)}</span>
        </span>
      `;
    }).join('');
  }

  // 绑定交互悬浮 Tooltip
  setupChartTooltip(container);
}

function setupChartTooltip(container) {
  const tooltip = document.getElementById('chart-tooltip');
  if (!tooltip || !container) return;

  container.onmousemove = function(e) {
    const target = e.target;
    if (target && target.classList.contains('stacked-bar-segment')) {
      const date = target.getAttribute('data-date');
      const model = target.getAttribute('data-model');
      const inTokens = Number(target.getAttribute('data-in') || 0);
      const outTokens = Number(target.getAttribute('data-out') || 0);
      const cacheTokens = Number(target.getAttribute('data-cache') || 0);
      const totalTokens = Number(target.getAttribute('data-total') || 0);
      const dayTotal = Number(target.getAttribute('data-daytotal') || 0);
      const color = target.getAttribute('data-color') || '#38bdf8';

      const pct = dayTotal > 0 ? ((totalTokens / dayTotal) * 100).toFixed(1) : '0.0';

      tooltip.innerHTML = `
        <div class="flex items-center justify-between border-b border-slate-700/80 pb-1.5 mb-2 gap-3">
          <span class="font-bold text-white">📅 ${date}</span>
          <span class="text-sky-400 mono font-bold">当日: ${formatTokens(dayTotal)}</span>
        </div>
        <div class="flex items-center gap-2 mb-2 font-semibold text-slate-200">
          <span class="w-2.5 h-2.5 rounded-full shrink-0" style="background:${color}"></span>
          <span class="truncate max-w-[200px]">${escapeHtml(model)}</span>
          <span class="text-xs text-sky-300 mono ml-auto font-bold">${pct}%</span>
        </div>
        <div class="space-y-1 text-[11px] mono text-slate-300 bg-slate-900/60 p-2 rounded-lg border border-slate-800">
          <div class="flex justify-between items-center text-white"><span>模型总计:</span><b>${formatTokens(totalTokens)}</b></div>
          <div class="flex justify-between items-center text-emerald-400"><span>输入 (↑):</span><span>${formatTokens(inTokens)}</span></div>
          <div class="flex justify-between items-center text-rose-400"><span>输出 (↓):</span><span>${formatTokens(outTokens)}</span></div>
          <div class="flex justify-between items-center text-amber-300"><span>缓存 (⚡):</span><span>${formatTokens(cacheTokens)}</span></div>
        </div>
      `;

      const rect = container.getBoundingClientRect();
      const x = e.clientX - rect.left;
      const y = e.clientY - rect.top;

      tooltip.style.left = `${Math.min(rect.width - 240, Math.max(10, x - 110))}px`;
      tooltip.style.top = `${Math.max(10, y - 135)}px`;
      tooltip.classList.remove('hidden');
    } else {
      tooltip.classList.add('hidden');
    }
  };

  container.onmouseleave = function() {
    tooltip.classList.add('hidden');
  };
}

function renderDailyTokensTable(dateGroups) {
  const tbody = document.getElementById('daily-tokens-tbody');
  const statsEl = document.getElementById('table-filter-stats');
  if (!tbody) return;

  const days = Array.from(dateGroups.values()); // 降序：最新日期在最前
  if (days.length === 0) {
    tbody.innerHTML = `<tr><td colspan="7" class="py-8 text-center text-slate-500">当前筛选范围内暂无模型 Token 消耗记录</td></tr>`;
    if (statsEl) statsEl.textContent = '显示 0 天 · 0 条模型记录';
    return;
  }

  const todayStr = new Date().toISOString().slice(0, 10);
  let html = '';
  let totalModelRows = 0;

  for (const grp of days) {
    const isToday = grp.date === todayStr;
    const dateLabel = isToday ? `📅 ${grp.date} (今日)` : `📅 ${grp.date}`;

    // 当日汇总行
    html += `
      <tr class="bg-sky-950/20 font-semibold text-slate-200">
        <td class="py-3 px-4 font-bold text-sky-400">${dateLabel}</td>
        <td class="py-3 px-4 text-slate-300">当天所选模型总计</td>
        <td class="py-3 px-4 text-emerald-400">${formatTokens(grp.totalIn)}</td>
        <td class="py-3 px-4 text-rose-400">${formatTokens(grp.totalOut)}</td>
        <td class="py-3 px-4 text-amber-300">${formatTokens(grp.totalCache)}</td>
        <td class="py-3 px-4 font-bold text-white">${formatTokens(grp.totalTokens)}</td>
        <td class="py-3 px-4">
          <div class="text-[11px] text-slate-400 mb-1">当天总占比 100%</div>
          <div class="bar-bg"><div class="bar-fill" style="width: 100%; background-color: var(--accent-sky);"></div></div>
        </td>
      </tr>
    `;

    // 各模型明细行
    for (const m of grp.models) {
      totalModelRows++;
      const rawModel = m.model_id || m.model || '';
      const alias = matchModelAlias(rawModel, m.display_name || rawModel);
      const modelDisplayName = alias.matched && alias.name ? `${alias.name}` : (m.display_name || rawModel || '默认模型');
      const isZero = (m.total_tokens || 0) === 0;
      const pct = (m.percentage != null && m.percentage > 0 && grp.totalTokens > 0)
        ? ((m.total_tokens / grp.totalTokens) * 100).toFixed(1)
        : (grp.totalTokens > 0 ? ((m.total_tokens / grp.totalTokens) * 100).toFixed(1) : '0.0');
      const color = getModelColor(rawModel);

      html += `
        <tr class="${isZero ? 'zero-row' : ''} hover:bg-slate-800/40 transition">
          <td class="py-2.5 px-4 text-slate-500">${grp.date}</td>
          <td class="py-2.5 px-4 text-slate-300 sub-row-cell">
            <span class="inline-block w-2 h-2 rounded-full mr-1.5" style="background:${color}"></span>
            ↳ <b>${escapeHtml(modelDisplayName)}</b>
          </td>
          <td class="py-2.5 px-4 text-slate-400">${formatTokens(m.input_tokens)}</td>
          <td class="py-2.5 px-4 text-slate-400">${formatTokens(m.output_tokens)}</td>
          <td class="py-2.5 px-4 text-amber-300/90">${formatTokens(m.cache_read_tokens || 0)}</td>
          <td class="py-2.5 px-4 font-bold text-slate-200">${formatTokens(m.total_tokens)}</td>
          <td class="py-2.5 px-4">
            <div class="text-[11px] text-slate-400 mb-0.5">${pct}%</div>
            <div class="bar-bg"><div class="bar-fill" style="width: ${pct}%; background-color: ${color};"></div></div>
          </td>
        </tr>
      `;
    }
  }

  tbody.innerHTML = html;
  if (statsEl) statsEl.textContent = `显示 ${days.length} 天 · ${totalModelRows} 条模型记录`;
  toggleZeroRows();
}

window.toggleZeroRows = function() {
  const checkbox = document.getElementById('toggle-zero');
  const show = checkbox ? checkbox.checked : true;
  document.querySelectorAll('.zero-row').forEach(row => {
    row.style.display = show ? '' : 'none';
  });
};

// 5. 调试原始上报 Master-Detail 布局
async function loadDebugRawReports() {
  const listContainer = document.getElementById('debug-list-container');
  if (!listContainer) return;

  try {
    const res = await fetch(getApiUrl('api/v1/debug/raw?limit=5'), { headers: getAuthHeaders() });
    if (!res.ok) {
      listContainer.innerHTML = `<div class="text-rose-400 text-center py-4">获取调试数据失败: HTTP ${res.status}</div>`;
      return;
    }
    const reports = await res.json();
    rawReportsCache.clear();

    if (!reports || reports.length === 0) {
      listContainer.innerHTML = `<div class="text-slate-500 text-center py-4">暂无原始上报记录</div>`;
      setText('badge-debug-count', '0');
      return;
    }

    setText('badge-debug-count', reports.length);

    let html = '';
    for (let i = 0; i < reports.length; i++) {
      const rep = reports[i];
      const rId = rep.record_id || rep.id || (i + 1);
      rawReportsCache.set(rId, rep);

      const isLatest = i === 0;
      const isSelected = (!currentDebugId && isLatest) || currentDebugId === rId;
      if (isSelected) currentDebugId = rId;

      const payload = rep.raw_payload || rep.envelope || {};
      const product = rep.product_type || payload.product || 'antigravity';
      const node = rep.node_id || 'node_default';
      const mName = payload.model ? (payload.model.display_name || payload.model.id || '') : (rep.model_id || '');
      const timeStr = rep.reported_at || rep.created_at || '--';

      html += `
        <button onclick="selectDebugRecord(${rId}, this)" class="debug-btn ${isSelected ? 'active' : ''} w-full text-left p-3.5 rounded-xl border border-slate-700 bg-slate-800/90 transition flex flex-col gap-1">
          <div class="flex justify-between items-center">
            <span class="font-bold text-sm">#${rId} ${isLatest ? '(最新上报)' : ''}</span>
            <span class="px-2 py-0.5 rounded bg-black/30 text-[11px]">${escapeHtml(product)}</span>
          </div>
          <div class="text-slate-300 text-[11px]">${escapeHtml(timeStr)}</div>
          <div class="text-slate-400 text-[11px] truncate">节点: ${escapeHtml(node)}${mName ? ' · ' + escapeHtml(mName) : ''}</div>
        </button>
      `;
    }
    listContainer.innerHTML = html;

    // 显示选中的详情
    if (currentDebugId && rawReportsCache.has(currentDebugId)) {
      displayDebugDetail(rawReportsCache.get(currentDebugId));
    }
  } catch (e) {
    listContainer.innerHTML = `<div class="text-rose-400 text-center py-4">加载异常: ${escapeHtml(e.message)}</div>`;
  }
}

window.selectDebugRecord = function(recordId, btnEl) {
  currentDebugId = recordId;
  document.querySelectorAll('.debug-btn').forEach(b => b.classList.remove('active'));
  if (btnEl) btnEl.classList.add('active');

  if (rawReportsCache.has(recordId)) {
    displayDebugDetail(rawReportsCache.get(recordId));
  }
};

function displayDebugDetail(rep) {
  const metaEl = document.getElementById('debug-meta');
  const viewer = document.getElementById('json-viewer');
  const rId = rep.record_id || rep.id || '--';
  const timeStr = rep.reported_at || rep.created_at || '--';
  if (metaEl) {
    metaEl.textContent = `当前查看: #${rId} · ${timeStr} · 节点: ${rep.node_id || 'node_default'}`;
  }
  if (viewer) {
    const rawData = rep.raw_payload || rep.envelope || rep;
    viewer.innerHTML = syntaxHighlightJSON(rawData);
  }
}

window.copyCurrentDebugJSON = function() {
  if (!currentDebugId || !rawReportsCache.has(currentDebugId)) {
    alert('暂无选中的原始上报数据');
    return;
  }
  const rep = rawReportsCache.get(currentDebugId);
  const rawData = rep.raw_payload || rep.envelope || rep;
  const jsonStr = JSON.stringify(rawData, null, 2);
  navigator.clipboard.writeText(jsonStr).then(() => {
    const btn = document.getElementById('btn-copy-debug');
    if (btn) {
      const oldText = btn.innerHTML;
      btn.innerHTML = '✓ 已复制到剪贴板';
      setTimeout(() => { btn.innerHTML = oldText; }, 2000);
    }
  }).catch(e => {
    alert('复制失败: ' + e.message);
  });
};

// 6. 别名管理弹窗与 CRUD
const MODULE_DESCRIPTIONS = {
  'model': { name: '模型标识与推理深度', desc: 'Phase 1 缩为短名 [Flash-H]；Phase 3 极端保底', defaultOrder: 1 },
  'context': { name: '上下文进度条与百分比', desc: 'Phase 2 进度条 8 斜杠压缩为 4 斜杠', defaultOrder: 2 },
  'quota': { name: '供应商配额余量与倒计时', desc: 'Phase 1 缩为 G:/C:；Phase 2 双窗口缩为单窗口', defaultOrder: 3 },
  'vcs': { name: 'Git 仓库分支与脏标记', desc: '代码分支 (main*)；Phase 3 空间不足时裁剪', defaultOrder: 4 },
  'tokens': { name: '会话 Token 吞吐计数器', desc: '吞吐指标 ↑1.5k ↓230；Phase 3 空间不足时裁剪', defaultOrder: 5 },
  'account': { name: '登录账号标识与邮箱', desc: 'Phase 1 缩为短名或邮箱前缀；Phase 3 裁剪', defaultOrder: 6 },
  'dir': { name: '工作区目录', desc: 'Phase 1 缩为目录名 [statusline]；Phase 3 最先裁剪', defaultOrder: 7 }
};

window.toggleAliasModal = function(show) {
  isAliasesOpen = show;
  const modal = document.getElementById('alias-modal');
  if (!modal) return;
  modal.classList.toggle('hidden', !show);
  if (show) {
    renderAliasTable();
  }
};

function renderAliasTable() {
  const tbody = document.getElementById('alias-tbody');
  const thKey = document.getElementById('alias-th-key');
  const thName = document.querySelector('#alias-table th:nth-child(2)');
  const thShort = document.querySelector('#alias-table th:nth-child(3)');
  const thSort = document.getElementById('alias-th-sort');
  const thAction = document.querySelector('#alias-table th:nth-child(5)');
  const btnAdd = document.getElementById('btn-add-alias');
  const btnReset = document.getElementById('btn-reset-priorities');
  const tipEl = document.getElementById('alias-extra-tip');
  if (!tbody) return;

  if (btnAdd) btnAdd.classList.toggle('hidden', currentAliasTab === 'priority');
  if (btnReset) btnReset.classList.toggle('hidden', currentAliasTab !== 'priority');

  if (currentAliasTab === 'priority') {
    if (thKey) thKey.textContent = '模块标识 (Key)';
    if (thName) thName.textContent = '模块说明 (宽屏)';
    if (thShort) thShort.textContent = '空间降级行为';
    if (thSort) thSort.textContent = '保护优先级';
    if (thAction) thAction.textContent = '快速排序微调';

    let priorities = activeAliases.priorities || [];
    if (priorities.length === 0) {
      priorities = Object.keys(MODULE_DESCRIPTIONS).map(k => ({
        category: 'priority',
        key: k,
        name: MODULE_DESCRIPTIONS[k].name,
        short_name: 'P' + MODULE_DESCRIPTIONS[k].defaultOrder,
        sort_order: MODULE_DESCRIPTIONS[k].defaultOrder
      })).sort((a, b) => a.sort_order - b.sort_order);
      activeAliases.priorities = priorities;
    }

    let html = '';
    for (let i = 0; i < priorities.length; i++) {
      const p = priorities[i];
      const descInfo = MODULE_DESCRIPTIONS[p.key] || { name: p.name || p.key, desc: '--' };
      const order = i + 1;

      let badgeClass = 'bg-rose-950/80 text-rose-300 border-rose-800';
      let labelText = '最先降级';
      if (order === 1) {
        badgeClass = 'bg-emerald-950/80 text-emerald-300 border-emerald-800';
        labelText = '最高保护';
      } else if (order <= 3) {
        badgeClass = 'bg-sky-950/80 text-sky-300 border-sky-800';
        labelText = '核心指标';
      } else if (order <= 5) {
        badgeClass = 'bg-amber-950/80 text-amber-300 border-amber-800';
        labelText = '次要指标';
      }

      const badgeHtml = `
        <span class="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-lg border text-xs font-bold ${badgeClass}">
          <span class="mono">#${order}</span>
          <span>${labelText}</span>
        </span>
      `;

      const sortControl = `
        <div class="inline-flex items-center gap-1 bg-slate-900/80 px-2 py-1 rounded-lg border border-slate-800">
          <button class="btn-action btn-xs text-sky-400 hover:text-white" ${i === 0 ? 'disabled' : ''} onclick="adjustPriorityOrder('${escapeHtml(p.key)}', -1)" title="上移优先级 (更受保护)">▲ 上移</button>
          <button class="btn-action btn-xs text-sky-400 hover:text-white" ${i === priorities.length - 1 ? 'disabled' : ''} onclick="adjustPriorityOrder('${escapeHtml(p.key)}', 1)" title="下移优先级 (先被降级)">▼ 下移</button>
        </div>
      `;

      html += `
        <tr class="hover:bg-slate-800/40 transition">
          <td class="py-2.5 px-3"><code>${escapeHtml(p.key)}</code></td>
          <td class="py-2.5 px-3"><strong>${escapeHtml(p.name || descInfo.name)}</strong></td>
          <td class="py-2.5 px-3"><span class="text-slate-400 text-xs">${escapeHtml(descInfo.desc)}</span></td>
          <td class="py-2.5 px-3 text-center">${badgeHtml}</td>
          <td class="py-2.5 px-3 text-right">${sortControl}</td>
        </tr>
      `;
    }
    tbody.innerHTML = html;

    if (tipEl) {
      tipEl.innerHTML = `
        <div class="mt-3 p-3 rounded-xl bg-slate-900/60 border border-slate-800 text-xs text-slate-400 flex items-center justify-between">
          <div>💡 <b>排版规则</b>：排在前面的模块受到最高优先级保护（窄屏最晚裁剪）；终端执行 <code class="text-sky-300">statusline init</code> 即可从云端同步生效。</div>
          <button class="btn-action btn-sm text-sky-400 border border-sky-800/60" onclick="resetDefaultPriorities()">↺ 恢复推荐排序</button>
        </div>
      `;
    }
    return;
  }

  if (thKey) {
    if (currentAliasTab === 'quota') thKey.textContent = '匹配前缀 (如 gemini, 3p, gemini-argon)';
    else if (currentAliasTab === 'model') thKey.textContent = '模型匹配标识 (如 gemini-3.8-flash, sonnet)';
    else thKey.textContent = '账号邮箱 (如 dev@example.com)';
  }
  if (thName) thName.textContent = '完整显示名 (宽屏)';
  if (thShort) thShort.textContent = '紧凑缩写 (窄屏)';
  if (thSort) thSort.textContent = '展示排序';
  if (thAction) thAction.textContent = '操作';

  const listKey = currentAliasTab === 'quota' ? 'quotas' : currentAliasTab === 'model' ? 'models' : 'accounts';
  const items = activeAliases[listKey] || [];

  let html = '';

  if (addingNewAlias) {
    const sortInputHtml = currentAliasTab === 'account'
      ? `<input id="input-new-sort" type="number" min="0" max="999" class="alias-input text-center" placeholder="排序" value="${newAliasPrefill.sort || 0}">`
      : `<span class="text-slate-500">--</span>`;

    html += `
      <tr class="bg-sky-950/20">
        <td class="p-2.5"><input id="input-new-key" class="alias-input" placeholder="输入匹配 Key / 前缀" value="${escapeHtml(newAliasPrefill.key || '')}"></td>
        <td class="p-2.5"><input id="input-new-name" class="alias-input" placeholder="输入完整显示名 (宽屏 display_name)" value="${escapeHtml(newAliasPrefill.name || '')}"></td>
        <td class="p-2.5"><input id="input-new-short" class="alias-input" placeholder="输入紧凑缩写 (窄屏 short_name)" value="${escapeHtml(newAliasPrefill.short || '')}"></td>
        <td class="p-2.5 text-center">${sortInputHtml}</td>
        <td class="p-2.5 text-right space-x-1">
          <button class="btn-action btn-sm btn-primary" onclick="confirmSaveNewAlias()">保存</button>
          <button class="btn-action btn-sm" onclick="cancelAddAlias()">取消</button>
        </td>
      </tr>
    `;
  }

  if (items.length === 0 && !addingNewAlias) {
    html += `<tr><td colspan="5" class="py-8 text-center text-slate-500">暂无 ${currentAliasTab} 映射规则，点击右上角「+ 新增映射」创建</td></tr>`;
  } else {
    for (const item of items) {
      if (editingAliasKey === item.key) {
        const sortInputHtml = currentAliasTab === 'account'
          ? `<input id="input-edit-sort" type="number" min="0" max="999" class="alias-input text-center" value="${item.sort_order || 0}">`
          : `<span class="text-slate-500">--</span>`;

        html += `
          <tr class="bg-sky-950/20">
            <td class="p-2.5"><code>${escapeHtml(item.key)}</code></td>
            <td class="p-2.5"><input id="input-edit-name" class="alias-input" value="${escapeHtml(item.name)}"></td>
            <td class="p-2.5"><input id="input-edit-short" class="alias-input" value="${escapeHtml(item.short_name || '')}"></td>
            <td class="p-2.5 text-center">${sortInputHtml}</td>
            <td class="p-2.5 text-right space-x-1">
              <button class="btn-action btn-sm btn-primary" onclick="confirmSaveEditAlias('${escapeHtml(item.key)}')">保存</button>
              <button class="btn-action btn-sm" onclick="cancelEditAlias()">取消</button>
            </td>
          </tr>
        `;
      } else {
        let sortCellHtml = `<span class="text-slate-500 mono">${item.sort_order > 0 ? '#' + item.sort_order : '--'}</span>`;
        if (currentAliasTab === 'account') {
          sortCellHtml = `
            <div class="inline-flex items-center gap-1.5 bg-slate-900/80 px-2 py-0.5 rounded-lg border border-slate-800">
              <button class="btn-action btn-xs text-sky-400 hover:text-white" onclick="adjustAccountOrder('${escapeHtml(item.key)}', -1)" title="上移 (排在前面)">▲</button>
              <span class="mono text-xs font-bold ${item.sort_order > 0 ? 'text-amber-400' : 'text-slate-500'}" style="min-width: 24px; text-align: center;">${item.sort_order > 0 ? '#' + item.sort_order : '--'}</span>
              <button class="btn-action btn-xs text-sky-400 hover:text-white" onclick="adjustAccountOrder('${escapeHtml(item.key)}', 1)" title="下移 (排在后面)">▼</button>
            </div>
          `;
        }

        html += `
          <tr class="hover:bg-slate-800/40 transition">
            <td class="py-2.5 px-3"><code>${escapeHtml(item.key)}</code></td>
            <td class="py-2.5 px-3"><strong>${escapeHtml(item.name)}</strong></td>
            <td class="py-2.5 px-3"><span class="px-2 py-0.5 rounded bg-slate-800 text-sky-400 mono">${escapeHtml(item.short_name || '--')}</span></td>
            <td class="py-2.5 px-3 text-center">${sortCellHtml}</td>
            <td class="py-2.5 px-3 text-right space-x-1">
              <button class="btn-action btn-sm" onclick="startEditAlias('${escapeHtml(item.key)}')">编辑</button>
              <button class="btn-action btn-sm btn-danger" onclick="confirmDeleteAlias('${escapeHtml(item.key)}')">删除</button>
            </td>
          </tr>
        `;
      }
    }
  }

  tbody.innerHTML = html;

  // 账号未配置提示与一键快捷添加
  if (tipEl) {
    if (currentAliasTab === 'account' && Array.isArray(lastRawQuotas) && lastRawQuotas.length > 0) {
      const mappedKeys = new Set((activeAliases.accounts || []).map(a => (a.key || '').toLowerCase()));
      const unmapped = [];
      for (const q of lastRawQuotas) {
        const email = (q.account_email || '').trim();
        if (email && !mappedKeys.has(email.toLowerCase()) && !unmapped.includes(email)) {
          unmapped.push(email);
        }
      }
      if (unmapped.length > 0) {
        const nextOrder = (activeAliases.accounts || []).length + 1;
        tipEl.innerHTML = `
          <div class="mt-3 p-3 rounded-xl bg-slate-900/60 border border-slate-800 flex flex-wrap items-center justify-between gap-2 text-xs">
            <div class="text-slate-400">检测到活跃上报账号尚未配置别名与排序：<b class="text-amber-400">${escapeHtml(unmapped.join(', '))}</b></div>
            <button class="btn-action btn-sm text-sky-400 border border-sky-800/60" onclick="quickAddAlias('account', '${escapeHtml(unmapped[0])}', '${escapeHtml(unmapped[0].split('@')[0])}', '', ${nextOrder})">+ 添加并排序首个</button>
          </div>
        `;
      } else {
        tipEl.innerHTML = '';
      }
    } else {
      tipEl.innerHTML = '';
    }
  }
}

window.adjustAccountOrder = async function(key, delta) {
  const accounts = activeAliases.accounts || [];
  const idx = accounts.findIndex(a => (a.key || '').toLowerCase() === (key || '').toLowerCase());
  if (idx === -1) return;

  const targetIdx = idx + delta;
  if (targetIdx < 0 || targetIdx >= accounts.length) return;

  // Swap
  const temp = accounts[idx];
  accounts[idx] = accounts[targetIdx];
  accounts[targetIdx] = temp;

  // Reassign 1-based sequential sort_order
  accounts.forEach((acc, i) => {
    acc.sort_order = i + 1;
  });

  try {
    const payload = accounts.map(a => ({
      category: 'account',
      key: a.key,
      name: a.name,
      short_name: a.short_name || '',
      sort_order: a.sort_order
    }));

    const res = await requestWithAuth(getApiUrl('api/v1/config/aliases'), {
      method: 'POST',
      body: JSON.stringify(payload)
    });
    if (!res.ok) {
      const err = await res.json().catch(() => ({}));
      const msg = res.status === 401 ? '未授权或 API Key 错误 (401 Unauthorized)' : (err.error || res.statusText);
      alert('调整排序失败: ' + msg);
      return;
    }
    await loadAliases();
    renderAliasTable();
    await refreshDashboard(true);
  } catch (e) {
    alert('请求异常: ' + e.message);
  }
};

window.adjustPriorityOrder = async function(key, delta) {
  const priorities = activeAliases.priorities || [];
  const idx = priorities.findIndex(p => (p.key || '').toLowerCase() === (key || '').toLowerCase());
  if (idx === -1) return;

  const targetIdx = idx + delta;
  if (targetIdx < 0 || targetIdx >= priorities.length) return;

  // Swap
  const temp = priorities[idx];
  priorities[idx] = priorities[targetIdx];
  priorities[targetIdx] = temp;

  // Reassign 1-based sequential sort_order
  priorities.forEach((p, i) => {
    p.sort_order = i + 1;
  });

  try {
    const payload = priorities.map(p => ({
      category: 'priority',
      key: p.key,
      name: p.name || MODULE_DESCRIPTIONS[p.key]?.name || p.key,
      short_name: 'P' + p.sort_order,
      sort_order: p.sort_order
    }));

    const res = await requestWithAuth(getApiUrl('api/v1/config/aliases'), {
      method: 'POST',
      body: JSON.stringify(payload)
    });
    if (!res.ok) {
      const err = await res.json().catch(() => ({}));
      const msg = res.status === 401 ? '未授权或 API Key 错误 (401 Unauthorized)' : (err.error || res.statusText);
      alert('调整优先级失败: ' + msg);
      return;
    }
    await loadAliases();
    renderAliasTable();
  } catch (e) {
    alert('请求异常: ' + e.message);
  }
};

window.resetDefaultPriorities = async function() {
  if (!confirm('确定将 7 大模块优先级恢复为官方推荐顺序吗？\n(model > context > quota > vcs > tokens > account > dir)')) {
    return;
  }
  const defaultKeys = ['model', 'context', 'quota', 'vcs', 'tokens', 'account', 'dir'];
  const payload = defaultKeys.map((k, i) => ({
    category: 'priority',
    key: k,
    name: MODULE_DESCRIPTIONS[k]?.name || k,
    short_name: 'P' + (i + 1),
    sort_order: i + 1
  }));

  try {
    const res = await requestWithAuth(getApiUrl('api/v1/config/aliases'), {
      method: 'POST',
      body: JSON.stringify(payload)
    });
    if (!res.ok) {
      const err = await res.json().catch(() => ({}));
      const msg = res.status === 401 ? '未授权或 API Key 错误 (401 Unauthorized)' : (err.error || res.statusText);
      alert('恢复默认排序失败: ' + msg);
      return;
    }
    await loadAliases();
    renderAliasTable();
  } catch (e) {
    alert('请求异常: ' + e.message);
  }
};

window.startEditAlias = function(key) {
  editingAliasKey = key;
  addingNewAlias = false;
  renderAliasTable();
};

window.cancelEditAlias = function() {
  editingAliasKey = null;
  renderAliasTable();
};

window.confirmSaveEditAlias = async function(key) {
  const nameEl = document.getElementById('input-edit-name');
  const shortEl = document.getElementById('input-edit-short');
  const sortEl = document.getElementById('input-edit-sort');
  const name = nameEl ? nameEl.value.trim() : '';
  const shortName = shortEl ? shortEl.value.trim() : '';
  const sortOrder = sortEl ? (parseInt(sortEl.value, 10) || 0) : 0;
  if (!name) {
    alert('完整显示名不能为空');
    return;
  }

  try {
    const res = await requestWithAuth(getApiUrl('api/v1/config/aliases'), {
      method: 'POST',
      body: JSON.stringify([{ category: currentAliasTab, key, name, short_name: shortName, sort_order: sortOrder }])
    });
    if (!res.ok) {
      const err = await res.json().catch(() => ({}));
      const msg = res.status === 401 ? '未授权或 API Key 错误 (401 Unauthorized)' : (err.error || res.statusText);
      alert('保存失败: ' + msg);
      return;
    }
    editingAliasKey = null;
    await loadAliases();
    renderAliasTable();
    await refreshDashboard(true);
  } catch (e) {
    alert('请求异常: ' + e.message);
  }
};

window.cancelAddAlias = function() {
  addingNewAlias = false;
  newAliasPrefill = { key: '', name: '', short: '', sort: 0 };
  renderAliasTable();
};

window.confirmSaveNewAlias = async function() {
  const keyEl = document.getElementById('input-new-key');
  const nameEl = document.getElementById('input-new-name');
  const shortEl = document.getElementById('input-new-short');
  const sortEl = document.getElementById('input-new-sort');
  const key = keyEl ? keyEl.value.trim() : '';
  const name = nameEl ? nameEl.value.trim() : '';
  const shortName = shortEl ? shortEl.value.trim() : '';
  const sortOrder = sortEl ? (parseInt(sortEl.value, 10) || 0) : 0;

  if (!key || !name) {
    alert('匹配 Key / 前缀 和 完整显示名 均不能为空');
    return;
  }

  try {
    const res = await requestWithAuth(getApiUrl('api/v1/config/aliases'), {
      method: 'POST',
      body: JSON.stringify([{ category: currentAliasTab, key, name, short_name: shortName, sort_order: sortOrder }])
    });
    if (!res.ok) {
      const err = await res.json().catch(() => ({}));
      const msg = res.status === 401 ? '未授权或 API Key 错误 (401 Unauthorized)' : (err.error || res.statusText);
      alert('保存失败: ' + msg);
      return;
    }
    addingNewAlias = false;
    newAliasPrefill = { key: '', name: '', short: '', sort: 0 };
    await loadAliases();
    renderAliasTable();
    await refreshDashboard(true);
  } catch (e) {
    alert('请求异常: ' + e.message);
  }
};

window.confirmDeleteAlias = async function(key) {
  if (!confirm(`确定要删除规则 [${key}] 吗？`)) return;
  try {
    const res = await requestWithAuth(getApiUrl(`api/v1/config/aliases?category=${currentAliasTab}&key=${encodeURIComponent(key)}`), {
      method: 'DELETE'
    });
    if (!res.ok) {
      const err = await res.json().catch(() => ({}));
      const msg = res.status === 401 ? '未授权或 API Key 错误 (401 Unauthorized)' : (err.error || res.statusText);
      alert('删除失败: ' + msg);
      return;
    }
    await loadAliases();
    renderAliasTable();
    await refreshDashboard(true);
  } catch (e) {
    alert('请求异常: ' + e.message);
  }
};

window.quickAddAlias = function(category, key, defaultName, defaultShort, defaultSort = 0) {
  currentAliasTab = category;
  toggleAliasModal(true);
  addingNewAlias = true;
  editingAliasKey = null;
  newAliasPrefill = { key, name: defaultName, short: defaultShort, sort: defaultSort };

  document.querySelectorAll('.alias-tab').forEach(tab => {
    tab.classList.toggle('active', tab.getAttribute('data-tab') === currentAliasTab);
  });

  renderAliasTable();
  setTimeout(() => {
    const inputName = document.getElementById('input-new-name');
    if (inputName) inputName.focus();
  }, 100);
};

function formatBytes(bytes) {
  if (bytes === 0 || !bytes) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
}

// 7. 版本分发与发布包管理 (Release Packages & Upgrade Distribution)
window.toggleReleasesModal = function(show) {
  const modal = document.getElementById('releases-modal');
  if (!modal) return;
  modal.classList.toggle('hidden', !show);
  if (show) {
    loadReleases();
  }
};

async function loadReleases() {
  const tbody = document.getElementById('releases-tbody');
  if (!tbody) return;
  try {
    const res = await fetch(getApiUrl('api/v1/version'));
    if (!res.ok) {
      throw new Error(`HTTP ${res.status}: ${res.statusText}`);
    }
    const data = await res.json();
    renderReleasesTable(data);
  } catch (err) {
    tbody.innerHTML = `<tr><td colspan="6" class="py-4 text-center text-rose-400">加载发布包失败: ${escapeHtml(err.message)}</td></tr>`;
  }
}

function renderReleasesTable(catalog) {
  const tbody = document.getElementById('releases-tbody');
  if (!tbody) return;

  const packages = catalog.packages || [];
  if (packages.length === 0) {
    tbody.innerHTML = `
      <tr>
        <td colspan="6" class="py-8 text-center text-slate-500">
          <div class="text-xl mb-1">📭</div>
          <div>当前服务端暂无托管的发布包</div>
          <div class="text-[11px] text-slate-600 mt-1">请拖拽安装包到上方区域上传，或点击右上角「从 GitHub 同步最新 Release」</div>
        </td>
      </tr>`;
    return;
  }

  let html = '';
  packages.forEach(pkg => {
    const isLatest = catalog.latest_version && pkg.version === catalog.latest_version;
    const verBadge = isLatest
      ? `<span class="px-2 py-0.5 rounded text-[10px] font-semibold bg-emerald-950/60 text-emerald-300 border border-emerald-800/60">${escapeHtml(pkg.version)} (最新)</span>`
      : `<span class="px-2 py-0.5 rounded text-[10px] bg-slate-800 text-slate-300 border border-slate-700">${escapeHtml(pkg.version)}</span>`;

    const platColor = pkg.platform.includes('windows')
      ? 'text-sky-300'
      : pkg.platform.includes('darwin')
      ? 'text-purple-300'
      : 'text-amber-300';
    const platBadge = `<span class="mono ${platColor}">${escapeHtml(pkg.platform)}</span>`;

    const checksumShort = pkg.checksum
      ? `<span class="text-slate-400 hover:text-white cursor-pointer" title="SHA256: ${escapeHtml(pkg.checksum)}">${escapeHtml(pkg.checksum.substring(0, 8))}...</span>`
      : `<span class="text-slate-600">--</span>`;

    const downloadUrl = getApiUrl(pkg.download_path || `api/v1/upgrade/download?filename=${encodeURIComponent(pkg.filename)}`);

    html += `
      <tr class="hover:bg-slate-900/40 transition">
        <td class="py-2.5 px-3 text-white flex items-center gap-2">
          <span>📦</span>
          <span>${escapeHtml(pkg.filename)}</span>
        </td>
        <td class="py-2.5 px-3">${verBadge}</td>
        <td class="py-2.5 px-3">${platBadge}</td>
        <td class="py-2.5 px-3 text-slate-400">${formatBytes(pkg.size)}</td>
        <td class="py-2.5 px-3">${checksumShort}</td>
        <td class="py-2.5 px-3 text-right">
          <a href="${downloadUrl}" download class="text-sky-400 hover:text-sky-300 font-medium hover:underline">下载</a>
        </td>
      </tr>`;
  });

  tbody.innerHTML = html;
}

window.syncReleasesFromGitHub = async function() {
  const btn = document.getElementById('btn-sync-github');
  if (btn) {
    btn.disabled = true;
    btn.innerHTML = '↻ 同步中 (可能需要数十秒)...';
  }
  const statusEl = document.getElementById('upload-status');
  if (statusEl) {
    statusEl.classList.remove('hidden');
    statusEl.className = 'text-xs text-sky-400 mt-2 mono';
    statusEl.textContent = '正在从 GitHub Releases 下载最新安装包并缓存至本地...';
  }

  try {
    const res = await requestWithAuth(getApiUrl('api/v1/upgrade/sync-github'), {
      method: 'POST'
    });
    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: res.statusText }));
      throw new Error(err.error || res.statusText);
    }
    const data = await res.json();
    const count = data.downloaded ? data.downloaded.length : 0;
    if (statusEl) {
      statusEl.className = 'text-xs text-emerald-400 mt-2 mono';
      statusEl.textContent = `同步成功! 已下载版本 ${data.version || ''} 的 ${count} 个资产文件`;
      setTimeout(() => { statusEl.classList.add('hidden'); }, 5000);
    }
    await loadReleases();
  } catch (err) {
    if (statusEl) {
      statusEl.className = 'text-xs text-rose-400 mt-2 mono';
      statusEl.textContent = `GitHub 同步失败: ${err.message}`;
    }
    alert('从 GitHub 同步失败: ' + err.message);
  } finally {
    if (btn) {
      btn.disabled = false;
      btn.innerHTML = '↻ 从 GitHub 同步最新 Release';
    }
  }
};

window.handleReleaseFilesSelected = async function(files) {
  if (!files || files.length === 0) return;
  const statusEl = document.getElementById('upload-status');
  if (statusEl) {
    statusEl.classList.remove('hidden');
    statusEl.className = 'text-xs text-sky-400 mt-2 mono';
    statusEl.textContent = `正在上传 ${files.length} 个文件...`;
  }

  const formData = new FormData();
  for (let i = 0; i < files.length; i++) {
    formData.append('files', files[i]);
  }

  try {
    const res = await requestWithAuth(getApiUrl('api/v1/upgrade/upload'), {
      method: 'POST',
      body: formData
    });
    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: res.statusText }));
      throw new Error(err.error || res.statusText);
    }
    const data = await res.json();
    const saved = data.saved_files ? data.saved_files.length : 0;
    if (statusEl) {
      statusEl.className = 'text-xs text-emerald-400 mt-2 mono';
      statusEl.textContent = `上传成功! 已成功保存 ${saved} 个文件`;
      setTimeout(() => { statusEl.classList.add('hidden'); }, 4000);
    }
    await loadReleases();
  } catch (err) {
    if (statusEl) {
      statusEl.className = 'text-xs text-rose-400 mt-2 mono';
      statusEl.textContent = `文件上传失败: ${err.message}`;
    }
    alert('上传失败: ' + err.message);
  } finally {
    const fileInput = document.getElementById('releases-file-input');
    if (fileInput) fileInput.value = '';
  }
};

// 8. 全局看板数据拉取与刷新
async function refreshDashboard(forceRedraw = false) {
  try {
    const overviewPromise = fetch(getApiUrl('api/v1/overview'));
    const tokensDailyPromise = fetch(getApiUrl('api/v1/tokens/daily?days=90'));
    const quotasPromise = fetch(getApiUrl('api/v1/quotas'));
    const sessionsPromise = fetch(getApiUrl('api/v1/sessions'));

    const [overviewRes, dailyRes, quotasRes, sessionsRes] = await Promise.all([
      overviewPromise, tokensDailyPromise, quotasPromise, sessionsPromise
    ]);

    if (!overviewRes.ok || !dailyRes.ok || !quotasRes.ok || !sessionsRes.ok) {
      setConnectionState(false, '服务响应异常');
      return;
    }

    setConnectionState(true, '已连接');
    setText('last-updated', new Date().toLocaleTimeString());

    const overview = await overviewRes.json();
    const dailyTokens = await dailyRes.json();
    const quotas = await quotasRes.json();
    const sessions = await sessionsRes.json();

    lastRenderCache.rawSessions = sessions;

    // 更新 Header 指标
    const totalSessions = overview.active_sessions || 0;
    setText('val-sessions', totalSessions);
    const agyCount = overview.antigravity_sessions || 0;
    const claudeCount = overview.claude_sessions || 0;
    setText('val-sources', `${agyCount} agy / ${claudeCount} claude`);

    // 计算今日消耗 Token
    let todayTokens = 0;
    let todayInput = 0;
    let todayOutput = 0;
    let todayCache = 0;
    const todayStr = new Date().toISOString().slice(0, 10);
    if (dailyTokens && Array.isArray(dailyTokens)) {
      for (const row of dailyTokens) {
        if (row.date === todayStr) {
          todayInput += (row.total_input_tokens != null ? row.total_input_tokens : (row.input_tokens || 0));
          todayOutput += (row.total_output_tokens != null ? row.total_output_tokens : (row.output_tokens || 0));
          todayCache += (row.total_cache_read_tokens != null ? row.total_cache_read_tokens : (row.cache_read_tokens || 0));
          todayTokens += (row.total_tokens || 0);
        }
      }
    }

    setText('val-tokens-today', formatTokens(todayTokens));
    setText('badge-tokens-count', formatTokens(todayTokens));
    setHtml('val-tokens-today-detail', `↑${formatTokens(todayInput)} ↓${formatTokens(todayOutput)}<br>⚡${formatTokens(todayCache)}`);

    const totalTokens = overview.total_tokens || overview.tokens_total || 0;
    const totalInput = overview.total_input_tokens || 0;
    const totalOutput = overview.total_output_tokens || 0;
    const totalCache = overview.total_cache_read_tokens || 0;
    setText('val-tokens-total', formatTokens(totalTokens));
    setHtml('val-tokens-total-detail', `↑${formatTokens(totalInput)} ↓${formatTokens(totalOutput)}<br>⚡${formatTokens(totalCache)}`);

    setText('val-nodes', overview.active_nodes || 0);

    // 渲染各模块
    renderAccountQuotas(quotas, forceRedraw);
    renderSessions(sessions, forceRedraw);
    renderDailyTokensView(dailyTokens, forceRedraw);

    if (activeMainTab === 'tab-debug') {
      loadDebugRawReports();
    }
  } catch (e) {
    console.error('[statusline] refresh error:', e);
    setConnectionState(false, '连接中断');
  }
}

// 事件绑定与初始化
document.addEventListener('DOMContentLoaded', async () => {
  // 别名 Tab 切换
  document.querySelectorAll('.alias-tab').forEach(tab => {
    tab.addEventListener('click', () => {
      document.querySelectorAll('.alias-tab').forEach(t => t.classList.remove('active'));
      tab.classList.add('active');
      currentAliasTab = tab.getAttribute('data-tab');
      addingNewAlias = false;
      editingAliasKey = null;
      renderAliasTable();
    });
  });

  const btnAdd = document.getElementById('btn-add-alias');
  if (btnAdd) {
    btnAdd.addEventListener('click', () => {
      addingNewAlias = true;
      editingAliasKey = null;
      let nextSort = 0;
      if (currentAliasTab === 'account') {
        const accs = activeAliases.accounts || [];
        nextSort = accs.length + 1;
      }
      newAliasPrefill = { key: '', name: '', short: '', sort: nextSort };
      renderAliasTable();
      setTimeout(() => {
        const inputKey = document.getElementById('input-new-key');
        if (inputKey) inputKey.focus();
      }, 50);
    });
  }

  // 手动刷新按钮
  const btnRefresh = document.getElementById('btn-manual-refresh');
  if (btnRefresh) {
    btnRefresh.addEventListener('click', async () => {
      btnRefresh.disabled = true;
      const oldHtml = btnRefresh.innerHTML;
      btnRefresh.innerHTML = '↻ 刷新中...';
      try {
        await loadAliases();
        await loadNodes();
        await refreshDashboard(true);
      } finally {
        btnRefresh.innerHTML = oldHtml;
        btnRefresh.disabled = false;
      }
    });
  }

  // 发布包拖拽上传区域事件绑定
  const dropzone = document.getElementById('releases-dropzone');
  if (dropzone) {
    ['dragenter', 'dragover'].forEach(eventName => {
      dropzone.addEventListener(eventName, (e) => {
        e.preventDefault();
        e.stopPropagation();
        dropzone.classList.add('border-sky-400', 'bg-sky-950/40');
      }, false);
    });
    ['dragleave', 'drop'].forEach(eventName => {
      dropzone.addEventListener(eventName, (e) => {
        e.preventDefault();
        e.stopPropagation();
        dropzone.classList.remove('border-sky-400', 'bg-sky-950/40');
      }, false);
    });
    dropzone.addEventListener('drop', (e) => {
      const dt = e.dataTransfer;
      const files = dt ? dt.files : null;
      if (files && files.length > 0) {
        window.handleReleaseFilesSelected(files);
      }
    }, false);
  }

  // 节点下拉选择变更
  const nodeSelect = document.getElementById('node-filter-select');
  if (nodeSelect) {
    nodeSelect.addEventListener('change', () => {
      filterSessions();
    });
  }

  // 检查 URL 是否带 api_key 参数，若有则自动持久化到 localStorage
  const urlApiKey = new URLSearchParams(window.location.search).get('api_key');
  if (urlApiKey) {
    localStorage.setItem('statusline_api_key', urlApiKey);
  }
  updateApiKeyBtnText();

  // 初始化拉取
  await loadAliases();
  await loadNodes();
  await refreshDashboard(true);

  // 定时刷新 (10s)
  function startPolling() {
    stopPolling();
    refreshTimer = setInterval(() => {
      refreshDashboard(false);
    }, 10000);
  }

  function stopPolling() {
    if (refreshTimer) {
      clearInterval(refreshTimer);
      refreshTimer = null;
    }
  }

  startPolling();

  // 页面隐藏时暂停轮询，恢复时立即刷新并重启轮询
  document.addEventListener('visibilitychange', () => {
    if (document.hidden) {
      stopPolling();
    } else {
      refreshDashboard(false);
      startPolling();
    }
  });
});

