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
      })
    };
  } catch (e) {
    console.error('[statusline] loadAliases error:', e);
  }
}

function formatTokens(n) {
  if (!n || n < 0) return '0';
  if (n < 1000) return String(n);
  if (n < 10000) return (n / 1000).toFixed(1) + 'k';
  if (n < 1000000) return Math.round(n / 1000) + 'k';
  return (n / 1000000).toFixed(2) + 'M';
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
      const shortPct = Math.round(shortFraction * 100);

      const weeklyFraction = weeklyItem ? Number(weeklyItem.remaining_fraction || 0) : 1;
      const weeklyReset = weeklyItem ? Number(weeklyItem.reset_in_seconds || 0) : 0;
      const weeklyPct = Math.round(weeklyFraction * 100);

      const hasWarning = shortPct < 20 || weeklyPct < 20;
      const isCritical = shortPct < 10 || weeklyPct < 10;

      // Concentric Rings Math (ViewBox: 0 0 120 120)
      // Outer Ring: R=52, Circumference = 2 * PI * 52 ≈ 326.7
      const C1 = 326.7;
      const offset1 = (C1 * (1 - Math.min(1, Math.max(0, shortFraction)))).toFixed(1);
      
      // Inner Ring: R=38, Circumference = 2 * PI * 38 ≈ 238.7
      const C2 = 238.7;
      const offset2 = (C2 * (1 - Math.min(1, Math.max(0, weeklyFraction)))).toFixed(1);

      // Colors
      let outerColor = '#38bdf8'; // sky
      if (ent.displayName.toLowerCase().includes('claude')) {
        outerColor = '#10b981'; // emerald
      }
      if (shortPct < 20) {
        outerColor = '#f59e0b'; // amber
      } else if (shortPct < 10) {
        outerColor = '#f43f5e'; // rose
      }

      let innerColor = '#0284c7'; // dark sky
      if (ent.displayName.toLowerCase().includes('claude')) {
        innerColor = weeklyPct < 20 ? '#f59e0b' : '#a855f7';
      } else if (weeklyPct < 20) {
        innerColor = '#f59e0b';
      }

      // Border style for model box
      const boxBorderClass = hasWarning ? 'border border-amber-500/40' : 'border border-slate-800/90';

      // Provider label or warning tag
      let tagHtml = `<span class="text-xs text-slate-400">${ent.displayName.toLowerCase().includes('claude') ? 'Anthropic' : 'Google AI'}</span>`;
      if (hasWarning) {
        tagHtml = `<span class="text-xs text-amber-400 font-semibold animate-pulse">● ${weeklyPct < 20 ? '周度预警' : '短期预警'}</span>`;
      }

      // Badge style
      const badgeBg = ent.displayName.toLowerCase().includes('claude') ? 'bg-amber-950 text-amber-400 border border-amber-800/60' : 'bg-sky-950 text-sky-400 border border-sky-800/60';

      entityBoxesHtml += `
        <div class="quota-unit-box ${boxBorderClass}">
          <div class="w-full flex justify-between items-center">
            <span class="px-2.5 py-1 text-xs font-bold ${badgeBg} rounded-md">${escapeHtml(ent.displayName.toUpperCase())}</span>
            ${tagHtml}
          </div>

          <!-- 巨型同心双环 SVG (自动适配至 180px~204px) -->
          <div class="quota-ring-svg-wrap my-3">
            <svg class="w-full h-full" viewBox="0 0 120 120">
              <!-- 外环底轨与进度条 (5h 短期 R=52) -->
              <circle cx="60" cy="60" r="52" fill="none" stroke="#152033" stroke-width="9"/>
              <circle class="ring-circle" cx="60" cy="60" r="52" fill="none" stroke="${outerColor}" stroke-width="9" stroke-linecap="round" stroke-dasharray="326.7" stroke-dashoffset="${offset1}"/>
              <!-- 内环底轨与进度条 (7d 周度 R=38) -->
              <circle cx="60" cy="60" r="38" fill="none" stroke="#152033" stroke-width="8"/>
              <circle class="ring-circle" cx="60" cy="60" r="38" fill="none" stroke="${innerColor}" stroke-width="8" stroke-linecap="round" stroke-dasharray="238.7" stroke-dashoffset="${offset2}"/>
            </svg>
            <div class="absolute inset-0 flex flex-col items-center justify-center text-center">
              <span class="text-[11px] text-slate-400">5h 短期</span>
              <span class="text-3xl font-extrabold ${shortPct < 20 ? 'text-amber-400' : 'text-white'} mono tracking-tight">${shortPct}%</span>
              <span class="text-xs font-bold ${weeklyPct < 20 ? 'text-amber-300 bg-amber-950 border border-amber-600/50' : 'text-sky-400 bg-sky-950/60'} mono mt-1 px-2 py-0.5 rounded">7d: ${weeklyPct}%</span>
            </div>
          </div>

          <!-- 环下方详情药丸框 -->
          <div class="w-full space-y-2 text-xs border-t border-slate-800/80 pt-3">
            <div class="flex justify-between items-center bg-slate-900/60 px-2.5 py-1.5 rounded-lg">
              <span class="text-slate-300 flex items-center gap-1.5"><i class="w-2 h-2 rounded-full" style="background-color: ${outerColor};"></i>短期 (5h)</span>
              <span class="${shortPct < 20 ? 'text-amber-400' : 'text-slate-300'} mono"><b>${shortPct}%</b> · ${formatDurationCompact(shortReset)}</span>
            </div>
            <div class="flex justify-between items-center ${weeklyPct < 20 ? 'bg-amber-950/40 border border-amber-800/40' : 'bg-slate-900/60'} px-2.5 py-1.5 rounded-lg">
              <span class="${weeklyPct < 20 ? 'text-amber-300' : 'text-slate-300'} flex items-center gap-1.5"><i class="w-2 h-2 rounded-full" style="background-color: ${innerColor};"></i>周度 (7d)</span>
              <span class="${weeklyPct < 20 ? 'text-amber-400' : 'text-slate-300'} mono"><b>${weeklyPct}%</b> · ${formatDurationCompact(weeklyReset)}</span>
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
        nodes: new Set()
      });
    }
    const group = projectMap.get(normKey);
    group.sessions.push(s);
    if (s.tokens) {
      group.totalInput += s.tokens.total_input_tokens || 0;
      group.totalOutput += s.tokens.total_output_tokens || 0;
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
          <span>输入 <b class="text-emerald-400">↑ ${inTokens}</b> · 输出 <b class="text-rose-400">↓ ${outTokens}</b></span>
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
                  <span>1 个活跃会话</span> · <span>汇总: <b class="text-emerald-400">↑${formatTokens(group.totalInput)}</b> <b class="text-rose-400">↓${formatTokens(group.totalOutput)}</b></span>
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
            <span>项目总吞吐: <b class="text-emerald-400">↑ ${formatTokens(group.totalInput)}</b> <b class="text-rose-400">↓ ${formatTokens(group.totalOutput)}</b></span>
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

// 4. 渲染每日各模型 Token 表格
function renderDailyTokensTable(rows, forceRedraw = false) {
  const tbody = document.getElementById('daily-tokens-tbody');
  if (!tbody) return;

  const cacheKey = JSON.stringify(rows || []);
  if (!forceRedraw && lastRenderCache.daily === cacheKey) {
    return;
  }
  lastRenderCache.daily = cacheKey;

  const items = rows || [];
  if (items.length === 0) {
    tbody.innerHTML = `<tr><td colspan="7" class="py-8 text-center text-slate-500">近 14 天暂无模型 Token 消耗记录</td></tr>`;
    return;
  }

  // 计算每日常模吞吐与聚合
  const dateGroups = new Map();
  for (const r of items) {
    const d = r.date || '未知日期';
    if (!dateGroups.has(d)) {
      dateGroups.set(d, {
        date: d,
        totalIn: 0,
        totalOut: 0,
        totalTokens: 0,
        totalCost: 0,
        models: []
      });
    }
    const grp = dateGroups.get(d);
    grp.totalIn += r.input_tokens || 0;
    grp.totalOut += r.output_tokens || 0;
    grp.totalTokens += r.total_tokens || 0;
    grp.models.push(r);
  }

  const todayStr = new Date().toISOString().slice(0, 10);
  let html = '';

  for (const grp of dateGroups.values()) {
    const isToday = grp.date === todayStr;
    const dateLabel = isToday ? `📅 ${grp.date} (今日)` : `📅 ${grp.date}`;

    // 汇总行
    html += `
      <tr class="bg-sky-950/20 font-semibold text-slate-200">
        <td class="py-3 px-4 font-bold text-sky-400">${dateLabel}</td>
        <td class="py-3 px-4 text-slate-300">当天各模型总计</td>
        <td class="py-3 px-4 text-emerald-400">${formatTokens(grp.totalIn)}</td>
        <td class="py-3 px-4 text-rose-400">${formatTokens(grp.totalOut)}</td>
        <td class="py-3 px-4 font-bold text-white">${formatTokens(grp.totalTokens)}</td>
        <td class="py-3 px-4">
          <div class="text-[11px] text-slate-400 mb-1">当天总占比 100%</div>
          <div class="bar-bg"><div class="bar-fill" style="width: 100%; background-color: var(--accent-sky);"></div></div>
        </td>
      </tr>
    `;

    // 模型明细行
    for (const m of grp.models) {
      const alias = matchModelAlias(m.model, m.model);
      const modelDisplayName = alias.matched && alias.name ? `${alias.name}` : m.model;
      const isZero = (m.total_tokens || 0) === 0;
      const pct = grp.totalTokens > 0 ? ((m.total_tokens / grp.totalTokens) * 100).toFixed(1) : '0.0';

      html += `
        <tr class="${isZero ? 'zero-row' : ''} hover:bg-slate-800/40 transition">
          <td class="py-2.5 px-4 text-slate-500">${grp.date}</td>
          <td class="py-2.5 px-4 text-slate-300 sub-row-cell">
            ↳ <b>${escapeHtml(modelDisplayName)}</b>
          </td>
          <td class="py-2.5 px-4 text-slate-400">${formatTokens(m.input_tokens)}</td>
          <td class="py-2.5 px-4 text-slate-400">${formatTokens(m.output_tokens)}</td>
          <td class="py-2.5 px-4 font-bold text-slate-200">${formatTokens(m.total_tokens)}</td>
          <td class="py-2.5 px-4">
            <div class="text-[11px] text-slate-400 mb-0.5">${pct}%</div>
            <div class="bar-bg"><div class="bar-fill" style="width: ${pct}%;"></div></div>
          </td>
        </tr>
      `;
    }
  }

  tbody.innerHTML = html;
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
  if (!tbody) return;

  if (thKey) {
    if (currentAliasTab === 'quota') thKey.textContent = '匹配前缀 (如 gemini, 3p, gemini-argon)';
    else if (currentAliasTab === 'model') thKey.textContent = '模型匹配标识 (如 gemini-3.8-flash, sonnet)';
    else thKey.textContent = '账号邮箱 (如 dev@example.com)';
  }

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
  const tipEl = document.getElementById('alias-extra-tip');
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

    const res = await fetch(getApiUrl('api/v1/config/aliases'), {
      method: 'POST',
      headers: getAuthHeaders(),
      body: JSON.stringify(payload)
    });
    if (!res.ok) {
      const err = await res.json().catch(() => ({}));
      alert('调整排序失败: ' + (err.error || res.statusText));
      return;
    }
    await loadAliases();
    renderAliasTable();
    await refreshDashboard(true);
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
    const res = await fetch(getApiUrl('api/v1/config/aliases'), {
      method: 'POST',
      headers: getAuthHeaders(),
      body: JSON.stringify([{ category: currentAliasTab, key, name, short_name: shortName, sort_order: sortOrder }])
    });
    if (!res.ok) {
      const err = await res.json().catch(() => ({}));
      alert('保存失败: ' + (err.error || res.statusText));
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
    const res = await fetch(getApiUrl('api/v1/config/aliases'), {
      method: 'POST',
      headers: getAuthHeaders(),
      body: JSON.stringify([{ category: currentAliasTab, key, name, short_name: shortName, sort_order: sortOrder }])
    });
    if (!res.ok) {
      const err = await res.json().catch(() => ({}));
      alert('保存失败: ' + (err.error || res.statusText));
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
    const res = await fetch(getApiUrl(`api/v1/config/aliases?category=${currentAliasTab}&key=${encodeURIComponent(key)}`), {
      method: 'DELETE',
      headers: getAuthHeaders()
    });
    if (!res.ok) {
      const err = await res.json().catch(() => ({}));
      alert('删除失败: ' + (err.error || res.statusText));
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

// 7. 全局看板数据拉取与刷新
async function refreshDashboard(forceRedraw = false) {
  try {
    const overviewPromise = fetch(getApiUrl('api/v1/overview'));
    const tokensDailyPromise = fetch(getApiUrl('api/v1/tokens/daily?days=14'));
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
    const todayStr = new Date().toISOString().slice(0, 10);
    if (dailyTokens && Array.isArray(dailyTokens)) {
      for (const row of dailyTokens) {
        if (row.date === todayStr) {
          todayInput += (row.input_tokens || 0);
          todayOutput += (row.output_tokens || 0);
          todayTokens += (row.total_tokens || 0);
        }
      }
    }

    setText('val-tokens-today', formatTokens(todayTokens));
    setText('badge-tokens-count', formatTokens(todayTokens));
    setHtml('val-tokens-today-detail', `↑${formatTokens(todayInput)}<br>↓${formatTokens(todayOutput)}`);

    const totalTokens = overview.total_tokens || overview.tokens_total || 0;
    const totalInput = overview.total_input_tokens || 0;
    const totalOutput = overview.total_output_tokens || 0;
    setText('val-tokens-total', formatTokens(totalTokens));
    setHtml('val-tokens-total-detail', `↑${formatTokens(totalInput)}<br>↓${formatTokens(totalOutput)}`);

    setText('val-nodes', overview.active_nodes || 0);

    // 渲染各模块
    renderAccountQuotas(quotas, forceRedraw);
    renderSessions(sessions, forceRedraw);
    renderDailyTokensTable(dailyTokens, forceRedraw);

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

  // 节点下拉选择变更
  const nodeSelect = document.getElementById('node-filter-select');
  if (nodeSelect) {
    nodeSelect.addEventListener('change', () => {
      filterSessions();
    });
  }

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
