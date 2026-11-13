// State
let selectedNodeId = '';
let isDebugOpen = false;
let isAliasesOpen = false;
let currentAliasTab = 'quota'; // 'quota' | 'model' | 'account'
let addingNewAlias = false;
let newAliasPrefill = { key: '', name: '', short: '' };
let editingAliasKey = null;
let activeAliases = { quotas: [], models: [], accounts: [] };
let knownNodes = [];
const expandedSessionIds = new Set();
const rawReportsCache = new Map();
let lastRenderCache = { daily: '', quotas: '', sessions: '' };
let refreshTimer = null;

function setText(id, text) {
  const el = document.getElementById(id);
  if (el) el.textContent = text;
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
    statusEl.style.color = isConnected ? 'var(--accent-emerald)' : 'var(--accent-rose)';
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

function matchModelAlias(modelID, displayName) {
  const target = (modelID || '').toLowerCase();
  const disp = (displayName || '').toLowerCase();
  for (const item of (activeAliases.models || [])) {
    const kLower = (item.key || '').toLowerCase();
    if ((target && target.includes(kLower)) || (disp && disp.includes(kLower))) {
      return { name: item.name, shortName: item.short_name, matched: true };
    }
  }
  return { name: displayName || modelID, shortName: '', matched: false };
}

function matchAccountAlias(email) {
  if (!email) return { name: '', shortName: '', matched: false };
  const lower = email.trim().toLowerCase();
  for (const item of (activeAliases.accounts || [])) {
    const kLower = (item.key || '').toLowerCase();
    if (lower === kLower || lower.startsWith(kLower)) {
      return { name: item.name, shortName: item.short_name, matched: true };
    }
  }
  return { name: email, shortName: '', matched: false };
}

async function loadAliases() {
  try {
    const res = await fetch(getApiUrl('api/v1/config/aliases'), { headers: getAuthHeaders() });
    if (!res.ok) return;
    const data = await res.json();
    activeAliases = {
      quotas: data.quotas || [],
      models: data.models || [],
      accounts: data.accounts || []
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

// 2. 渲染每日各模型 Token 表格 (带数据无变化跳过 & 明细行日期去重)
function renderDailyTokensTable(dailyTokens, forceRedraw = false) {
  const tbody = document.getElementById('daily-tokens-tbody');
  if (!tbody) return;

  const cacheKey = JSON.stringify(dailyTokens || []);
  if (!forceRedraw && lastRenderCache.daily === cacheKey) {
    return; // 数据无变化，跳过 DOM 重绘
  }
  lastRenderCache.daily = cacheKey;

  if (!dailyTokens || dailyTokens.length === 0) {
    tbody.innerHTML = '<tr><td colspan="7" class="table-empty">暂无每日 Token 统计数据，终端产生对话后自动展示</td></tr>';
    return;
  }

  const todayStr = new Date().toISOString().slice(0, 10);
  let html = '';

  for (const day of dailyTokens) {
    const isToday = day.date === todayStr;
    const dayTag = isToday ? ' (今日)' : '';
    const dayTotalIn = formatTokens(day.total_input_tokens);
    const dayTotalOut = formatTokens(day.total_output_tokens);
    const dayTotal = formatTokens(day.total_tokens);
    const dayCost = formatUSD(day.total_cost_usd);

    // 日期汇总行
    html += `
      <tr class="day-header-row">
        <td colspan="2">📅 <strong>${escapeHtml(day.date)}${dayTag}</strong></td>
        <td><span class="token-in">↑${dayTotalIn}</span></td>
        <td><span class="token-out">↓${dayTotalOut}</span></td>
        <td><span class="token-total">${dayTotal}</span></td>
        <td><span class="pct-text" style="width:auto; font-weight: 600;">当天总计</span></td>
        <td class="cost-text"><strong>${dayCost}</strong></td>
      </tr>
    `;

    // 模型明细行 (去重重复日期，以树状符号展现)
    const models = day.models || [];
    for (let i = 0; i < models.length; i++) {
      const m = models[i];
      const pct = m.percentage || 0;
      const mIn = formatTokens(m.input_tokens);
      const mOut = formatTokens(m.output_tokens);
      const mTot = formatTokens(m.total_tokens);
      const mCost = formatUSD(m.cost_usd);
      const subPrefix = i === 0 ? '↳ 明细' : '↳';

      const matchedModel = matchModelAlias(m.model_id, m.display_name);
      const displayModel = matchedModel.name || m.display_name || m.model_id;

      html += `
        <tr>
          <td class="sub-row-cell">${subPrefix}</td>
          <td><span class="model-tag" title="${escapeHtml(m.model_id || '')}">${escapeHtml(displayModel)}</span></td>
          <td><span class="token-in">↑${mIn}</span></td>
          <td><span class="token-out">↓${mOut}</span></td>
          <td><span class="token-total">${mTot}</span></td>
          <td>
            <div class="share-cell">
              <div class="pct-bar-wrap">
                <div class="pct-fill" style="width: ${Math.min(100, Math.max(2, pct))}%;"></div>
              </div>
              <span class="pct-text">${pct.toFixed(1)}%</span>
            </div>
          </td>
          <td class="cost-text">${mCost}</td>
        </tr>
      `;
    }
  }

  tbody.innerHTML = html;
}

// 解析倒计时字符串为秒数 (例如 "2h10m" -> 7800, "5d18h" -> 496800)
function parseCountdownToSeconds(str) {
  if (!str || typeof str !== 'string') return 0;
  let total = 0;
  const days = str.match(/(\d+)\s*d/);
  if (days) total += parseInt(days[1], 10) * 86400;
  const hours = str.match(/(\d+)\s*h/);
  if (hours) total += parseInt(hours[1], 10) * 3600;
  const mins = str.match(/(\d+)\s*m/);
  if (mins) total += parseInt(mins[1], 10) * 60;
  const secs = str.match(/(\d+)\s*s/);
  if (secs) total += parseInt(secs[1], 10);
  return total;
}

// 渲染单个同心双环进度条 (外环: 额度余量, 内环: 倒计时进度)
function renderConcentricRing(pct, resetCountdown, resetInSec, isShortWindow, windowLabel) {
  const p = Math.max(0, Math.min(100, Math.round(pct != null ? pct : 100)));

  // 外环颜色映射 (≥70% 绿色 #10b981, 30-69% 黄色 #f59e0b, <30% 红色 #ef4444)
  let outerColor = '#10b981';
  if (p < 30) {
    outerColor = '#ef4444';
  } else if (p < 70) {
    outerColor = '#f59e0b';
  }

  // 外环几何参数: R=23, 周长 C = 2 * π * 23 ≈ 144.51
  const outerCircumference = 144.51;
  const outerOffset = (outerCircumference * (1 - p / 100)).toFixed(2);

  // 内环颜色映射 (短期窗口 5h: 青色 #06b6d4, 周度窗口 7d: 紫色 #a855f7)
  const innerColor = isShortWindow ? '#06b6d4' : '#a855f7';
  const totalWindowSec = isShortWindow ? 18000 : 604800; // 5h=18000s, 7d=604800s

  let remainSec = resetInSec;
  if (!remainSec || remainSec <= 0) {
    remainSec = parseCountdownToSeconds(resetCountdown);
  }
  const ratio = (totalWindowSec > 0 && remainSec > 0)
    ? Math.max(0, Math.min(1, remainSec / totalWindowSec))
    : 1;

  // 内环几何参数: R=16.5, 周长 C = 2 * π * 16.5 ≈ 103.67
  const innerCircumference = 103.67;
  const innerOffset = (innerCircumference * (1 - ratio)).toFixed(2);

  const countdownDisplay = resetCountdown ? escapeHtml(resetCountdown) : '--';
  const tooltip = `${windowLabel} 窗口 | 额度余量: ${p}% | 倒计时: ${countdownDisplay}`;

  return `
    <div class="quota-ring-item" title="${tooltip}">
      <div class="quota-ring-svg-wrap">
        <svg width="56" height="56" viewBox="0 0 56 56" class="concentric-ring-svg">
          <!-- 外环背景 -->
          <circle cx="28" cy="28" r="23" fill="none" stroke="#21262d" stroke-width="3.0" />
          <!-- 外环进度 (额度余量) -->
          <circle cx="28" cy="28" r="23" fill="none" stroke="${outerColor}" stroke-width="3.0"
            stroke-dasharray="${outerCircumference}" stroke-dashoffset="${outerOffset}"
            stroke-linecap="round" transform="rotate(-90 28 28)" />
          <!-- 内环背景 -->
          <circle cx="28" cy="28" r="16.5" fill="none" stroke="#161b22" stroke-width="2.2" />
          <!-- 内环进度 (倒计时进度) -->
          <circle cx="28" cy="28" r="16.5" fill="none" stroke="${innerColor}" stroke-width="2.2"
            stroke-dasharray="${innerCircumference}" stroke-dashoffset="${innerOffset}"
            stroke-linecap="round" transform="rotate(-90 28 28)" />
          <!-- 中心百分比数值 -->
          <text x="28" y="32" text-anchor="middle" font-size="11" font-weight="700" fill="#f0f6fc">${p}%</text>
        </svg>
      </div>
      <div class="quota-ring-caption">
        <span class="caption-window">${escapeHtml(windowLabel)}</span>
        <span class="caption-time" style="color: ${innerColor};">${countdownDisplay}</span>
      </div>
    </div>
  `;
}

// 提取并聚合账号下的所有 Provider 配额窗口（支持 Gemini, Argon, Claude 及任意动态前缀）
function extractAllProviderGroups(acc) {
  const groupsMap = new Map();

  // 1. 优先从 acc.buckets 提取 (动态多 Provider 支持)
  if (acc.buckets && Array.isArray(acc.buckets) && acc.buckets.length > 0) {
    for (const b of acc.buckets) {
      const qKey = b.quota_key || b.prefix || '';
      const alias = matchQuotaAlias(qKey);
      const isShort = (alias.window === '5h') || (b.reset_in_seconds && b.reset_in_seconds <= 86400);
      const label = alias.window || (isShort ? '5h' : '7d');

      let grp = groupsMap.get(alias.name);
      if (!grp) {
        const lower = alias.name.toLowerCase();
        let pillClass = 'other-pill';
        if (lower.includes('gemini')) pillClass = 'gemini-pill';
        else if (lower.includes('argon')) pillClass = 'argon-pill';
        else if (lower.includes('claude') || lower.includes('3p')) pillClass = 'claude-pill';

        grp = {
          name: alias.name,
          shortName: alias.shortName,
          prefix: alias.prefix,
          matched: alias.matched,
          pillClass: pillClass,
          windows: []
        };
        groupsMap.set(alias.name, grp);
      }

      grp.windows.push({
        label,
        isShort,
        pct: b.remaining_pct != null ? b.remaining_pct : Math.round((b.remaining_fraction || 0) * 100),
        countdown: b.reset_countdown || (b.reset_in_seconds ? formatDurationCompact(b.reset_in_seconds) : '--'),
        resetInSec: b.reset_in_seconds || 0
      });
    }
  }

  // 2. 若 buckets 为空，从老字段 gemini / claude 回退兼容
  if (groupsMap.size === 0) {
    if (acc.gemini) {
      const gWin = [];
      if (acc.gemini.short_percentage != null || acc.gemini.short_reset) {
        gWin.push({
          label: '5h',
          isShort: true,
          pct: acc.gemini.short_percentage != null ? acc.gemini.short_percentage : 100,
          countdown: acc.gemini.short_reset || '--',
          resetInSec: parseCountdownToSeconds(acc.gemini.short_reset)
        });
      }
      if (acc.gemini.long_percentage != null || acc.gemini.long_reset) {
        gWin.push({
          label: '7d',
          isShort: false,
          pct: acc.gemini.long_percentage != null ? acc.gemini.long_percentage : 100,
          countdown: acc.gemini.long_reset || '--',
          resetInSec: parseCountdownToSeconds(acc.gemini.long_reset)
        });
      }
      if (gWin.length > 0) {
        groupsMap.set('Gemini', {
          name: 'Gemini',
          shortName: 'G',
          prefix: 'gemini',
          matched: true,
          pillClass: 'gemini-pill',
          windows: gWin
        });
      }
    }

    if (acc.claude) {
      const cWin = [];
      if (acc.claude.short_percentage != null || acc.claude.short_reset) {
        cWin.push({
          label: '5h',
          isShort: true,
          pct: acc.claude.short_percentage != null ? acc.claude.short_percentage : 100,
          countdown: acc.claude.short_reset || '--',
          resetInSec: parseCountdownToSeconds(acc.claude.short_reset)
        });
      }
      if (acc.claude.long_percentage != null || acc.claude.long_reset) {
        cWin.push({
          label: '7d',
          isShort: false,
          pct: acc.claude.long_percentage != null ? acc.claude.long_percentage : 100,
          countdown: acc.claude.long_reset || '--',
          resetInSec: parseCountdownToSeconds(acc.claude.long_reset)
        });
      }
      if (cWin.length > 0) {
        groupsMap.set('Claude', {
          name: 'Claude',
          shortName: 'C',
          prefix: 'claude',
          matched: true,
          pillClass: 'claude-pill',
          windows: cWin
        });
      }
    }
  }

  // 排序：Gemini -> Argon -> Claude -> 其他
  const priority = (name) => {
    const l = name.toLowerCase();
    if (l.includes('gemini')) return 1;
    if (l.includes('argon')) return 2;
    if (l.includes('claude') || l.includes('3p')) return 3;
    return 10;
  };

  const groups = Array.from(groupsMap.values());
  for (const g of groups) {
    g.windows.sort((a, b) => a.resetInSec - b.resetInSec);
  }
  groups.sort((a, b) => {
    const pA = priority(a.name);
    const pB = priority(b.name);
    if (pA !== pB) return pA - pB;
    return a.name.localeCompare(b.name);
  });

  return groups;
}

// 3. 渲染按账号区分的模型配额池 (紧凑彩色同心双环仪表盘，带防重绘闪烁)
function renderAccountQuotas(accountQuotas, forceRedraw = false) {
  const container = document.getElementById('quotas-container');
  const tip = document.getElementById('quota-accounts-tip');
  if (!container) return;

  const cacheKey = JSON.stringify(accountQuotas || []) + '|' + JSON.stringify(activeAliases.quotas || []);
  if (!forceRedraw && lastRenderCache.quotas === cacheKey) {
    return; // 数据无变化，跳过 DOM 重绘以保持 hover 与动画
  }
  lastRenderCache.quotas = cacheKey;

  if (!accountQuotas || accountQuotas.length === 0) {
    container.innerHTML = '<div class="empty-placeholder">暂未检测到配额信息，Antigravity 账号产生会话后将在此按账号独立呈现配额池...</div>';
    if (tip) tip.textContent = '0 个配额账号';
    return;
  }

  if (tip) tip.textContent = `${accountQuotas.length} 个配额账号`;

  let html = '';
  for (const acc of accountQuotas) {
    const email = acc.account_email || '未命名账号';
    const accAlias = matchAccountAlias(email);
    const displayEmail = accAlias.matched ? `${accAlias.name} (${email})` : email;
    const lastNode = acc.last_node_id ? `上报节点: ${escapeHtml(acc.last_node_id)}` : '';
    let updateTimeStr = '--';
    if (acc.updated_at) {
      const d = new Date(acc.updated_at);
      updateTimeStr = isNaN(d.getTime()) ? acc.updated_at : d.toLocaleTimeString();
    }

    const providerGroups = extractAllProviderGroups(acc);
    let groupsHtml = '';

    for (const grp of providerGroups) {
      const rings = grp.windows.map(w => renderConcentricRing(w.pct, w.countdown, w.resetInSec, w.isShort, w.label)).join('');
      const unmappedBtn = !grp.matched
        ? `<button class="btn-quick-alias" title="点击为此配额前缀快速配置别名" onclick="quickAddAlias('quota', '${escapeHtml(grp.prefix)}', '${escapeHtml(grp.name)}', '${escapeHtml(grp.shortName)}')">+ 映射</button>`
        : '';

      groupsHtml += `
        <div class="quota-provider-group ${escapeHtml(grp.pillClass.replace('-pill', '-group'))}">
          <div class="provider-badge-row">
            <span class="provider-pill ${escapeHtml(grp.pillClass)}">${escapeHtml(grp.name)}</span>
            ${unmappedBtn}
          </div>
          <div class="provider-rings-row">
            ${rings}
          </div>
        </div>
      `;
    }

    if (!groupsHtml) {
      groupsHtml = '<div class="empty-placeholder" style="padding: 10px 14px; font-size: 12px; margin: 0;">该账号暂无配额限制数据</div>';
    }

    html += `
      <div class="account-quota-card compact-card">
        <div class="account-card-header compact-header">
          <div class="account-badge-email">
            <span class="user-avatar-icon">👤</span>
            <span class="account-email-text" title="${escapeHtml(email)}">${escapeHtml(displayEmail)}</span>
          </div>
          <div class="account-node-meta">
            ${lastNode ? `<span>${escapeHtml(lastNode)}</span><span>·</span>` : ''}
            <span>更新于: ${escapeHtml(updateTimeStr)}</span>
          </div>
        </div>
        <div class="quota-providers-flex">
          ${groupsHtml}
        </div>
      </div>
    `;
  }

  container.innerHTML = html;
}

// 别名管理表格渲染与 CRUD 操作
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

  // 如果处于新增行状态，在顶部渲染输入框
  if (addingNewAlias) {
    html += `
      <tr class="alias-edit-row">
        <td><input id="input-new-key" class="alias-input" placeholder="输入匹配 Key / 前缀" value="${escapeHtml(newAliasPrefill.key || '')}"></td>
        <td><input id="input-new-name" class="alias-input" placeholder="输入完整显示名 (宽屏 display_name)" value="${escapeHtml(newAliasPrefill.name || '')}"></td>
        <td><input id="input-new-short" class="alias-input" placeholder="输入紧凑缩写 (窄屏 short_name)" value="${escapeHtml(newAliasPrefill.short || '')}"></td>
        <td class="alias-actions-cell">
          <button class="btn-action btn-sm btn-primary" onclick="confirmSaveNewAlias()">保存</button>
          <button class="btn-action btn-sm" onclick="cancelAddAlias()">取消</button>
        </td>
      </tr>
    `;
  }

  if (items.length === 0 && !addingNewAlias) {
    html += `<tr><td colspan="4" class="table-empty">暂无 ${currentAliasTab} 映射规则，点击右上角「+ 新增映射」创建</td></tr>`;
  } else {
    for (const item of items) {
      if (editingAliasKey === item.key) {
        html += `
          <tr class="alias-edit-row">
            <td><code>${escapeHtml(item.key)}</code></td>
            <td><input id="input-edit-name" class="alias-input" value="${escapeHtml(item.name)}"></td>
            <td><input id="input-edit-short" class="alias-input" value="${escapeHtml(item.short_name || '')}"></td>
            <td class="alias-actions-cell">
              <button class="btn-action btn-sm btn-primary" onclick="confirmSaveEditAlias('${escapeHtml(item.key)}')">保存</button>
              <button class="btn-action btn-sm" onclick="cancelEditAlias()">取消</button>
            </td>
          </tr>
        `;
      } else {
        html += `
          <tr>
            <td><code>${escapeHtml(item.key)}</code></td>
            <td><strong>${escapeHtml(item.name)}</strong></td>
            <td><span class="badge-range">${escapeHtml(item.short_name || '--')}</span></td>
            <td class="alias-actions-cell">
              <button class="btn-action btn-sm" onclick="startEditAlias('${escapeHtml(item.key)}')">编辑</button>
              <button class="btn-action btn-sm btn-danger" onclick="confirmDeleteAlias('${escapeHtml(item.key)}')">删除</button>
            </td>
          </tr>
        `;
      }
    }
  }

  tbody.innerHTML = html;
}

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
  const name = nameEl ? nameEl.value.trim() : '';
  const shortName = shortEl ? shortEl.value.trim() : '';
  if (!name) {
    alert('完整显示名不能为空');
    return;
  }

  try {
    const res = await fetch(getApiUrl('api/v1/config/aliases'), {
      method: 'POST',
      headers: getAuthHeaders(),
      body: JSON.stringify([{ category: currentAliasTab, key, name, short_name: shortName }])
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
  newAliasPrefill = { key: '', name: '', short: '' };
  renderAliasTable();
};

window.confirmSaveNewAlias = async function() {
  const keyEl = document.getElementById('input-new-key');
  const nameEl = document.getElementById('input-new-name');
  const shortEl = document.getElementById('input-new-short');
  const key = keyEl ? keyEl.value.trim() : '';
  const name = nameEl ? nameEl.value.trim() : '';
  const shortName = shortEl ? shortEl.value.trim() : '';

  if (!key || !name) {
    alert('匹配 Key / 前缀 和 完整显示名 均不能为空');
    return;
  }

  try {
    const res = await fetch(getApiUrl('api/v1/config/aliases'), {
      method: 'POST',
      headers: getAuthHeaders(),
      body: JSON.stringify([{ category: currentAliasTab, key, name, short_name: shortName }])
    });
    if (!res.ok) {
      const err = await res.json().catch(() => ({}));
      alert('保存失败: ' + (err.error || res.statusText));
      return;
    }
    addingNewAlias = false;
    newAliasPrefill = { key: '', name: '', short: '' };
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

window.quickAddAlias = function(category, key, defaultName, defaultShort) {
  currentAliasTab = category;
  isAliasesOpen = true;
  addingNewAlias = true;
  editingAliasKey = null;
  newAliasPrefill = { key, name: defaultName, short: defaultShort };

  const modal = document.getElementById('alias-modal');
  if (modal) {
    modal.style.display = 'block';
    modal.scrollIntoView({ behavior: 'smooth' });
  }

  document.querySelectorAll('.alias-tab').forEach(tab => {
    tab.classList.toggle('active', tab.getAttribute('data-tab') === currentAliasTab);
  });

  renderAliasTable();
  setTimeout(() => {
    const inputName = document.getElementById('input-new-name');
    if (inputName) inputName.focus();
  }, 100);
};

// 4. 渲染终端会话列表 (支持点击卡片展开详情，防重绘闪烁)
function renderSessions(sessions, forceRedraw = false) {
  const container = document.getElementById('sessions-container');
  if (!container) return;

  const stateToken = Array.from(expandedSessionIds).sort().join(',');
  const cacheKey = JSON.stringify(sessions || []) + '|' + stateToken;
  if (!forceRedraw && lastRenderCache.sessions === cacheKey) {
    return; // 无数据或展开状态变化，跳过重绘
  }
  lastRenderCache.sessions = cacheKey;

  if (!sessions || sessions.length === 0) {
    const filterTip = selectedNodeId ? `节点 [${escapeHtml(selectedNodeId)}] 下暂无活跃会话` : '暂未接收到任何会话遥测数据';
    container.innerHTML = `<div class="empty-placeholder">${filterTip}</div>`;
    return;
  }

  let html = '';
  for (const sess of sessions) {
    const isClaude = sess.source === 'claudecode' || sess.product_type === 'claude_code';
    const sourceClass = isClaude ? 'source-claude' : 'source-agy';
    const sourceName = isClaude ? 'ClaudeCode' : 'Antigravity';
    const isWorking = sess.agent_state === 'working';
    const stateClass = isWorking ? 'state-pill-working' : 'state-pill-idle';
    const stateName = isWorking ? '⚡ 运行中' : '空闲';

    const nodeBadge = sess.node_id ? `<span class="node-badge">🖥️ ${escapeHtml(sess.node_id)}</span>` : '';
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
    const planTier = (sess.account && sess.account.plan_tier) ? sess.account.plan_tier : '';
    const accAlias = matchAccountAlias(email);
    const displayAcc = accAlias.name || email;
    const accountBadge = email ? `<span class="account-badge" title="${escapeHtml(email)}">👤 ${escapeHtml(displayAcc)}</span>` : '';
    const ctxPct = sess.context_window ? Math.round(sess.context_window.used_percentage) : 0;
    const ctxSize = sess.context_window && sess.context_window.context_window_size ? formatTokens(sess.context_window.context_window_size) : '1M';
    const cost = formatUSD(sess.cost_usd);

    const isExpanded = expandedSessionIds.has(sess.session_id);
    const boxClass = isExpanded ? 'session-box expanded' : 'session-box';

    html += `
      <div class="${boxClass}" data-session-id="${escapeHtml(sess.session_id)}">
        <div class="session-top">
          <div class="session-tags">
            <span class="tag-badge ${sourceClass}">${escapeHtml(sourceName)}</span>
            <span class="tag-badge ${stateClass}">${escapeHtml(stateName)}</span>
            ${nodeBadge}
            ${accountBadge}
          </div>
          <div class="session-expand-hint" title="点击展开/收起完整调试信息">
            <span class="chevron">▶</span>
            <span class="time-stamp">${updatedRelative}</span>
          </div>
        </div>
        <div class="session-mid">
          <span class="session-title" title="${escapeHtml(title)}">${escapeHtml(title)}</span>
          <span class="model-pill">[${escapeHtml(modelName)}]</span>
        </div>
        <div class="session-path" title="${escapeHtml(sess.cwd || '~')}">📂 ${escapeHtml(sess.cwd || '~')}</div>
        <div class="session-stats-bar">
          <div>输入: <span class="metric-val">↑${inTokens}</span></div>
          <div>输出: <span class="metric-val">↓${outTokens}</span></div>
          <div>上下文: <span class="metric-val">${ctxPct}% [${ctxSize}]</span></div>
          <div>费用: <span class="metric-val">${cost}</span></div>
        </div>

        <!-- 详细抽屉 (点击卡片展开) -->
        <div class="session-detail-drawer">
          <div class="detail-grid">
            <div class="detail-item">
              <span class="detail-key">会话 ID (Session ID)</span>
              <span class="detail-val">${escapeHtml(sess.session_id)}</span>
            </div>
            ${sess.conversation_id ? `
            <div class="detail-item">
              <span class="detail-key">对话 ID (Conversation ID)</span>
              <span class="detail-val">${escapeHtml(sess.conversation_id)}</span>
            </div>` : ''}
            <div class="detail-item">
              <span class="detail-key">物理节点 (Node)</span>
              <span class="detail-val">${escapeHtml(sess.node_id || '--')}</span>
            </div>
            <div class="detail-item">
              <span class="detail-key">最后活跃时间 (Last Active)</span>
              <span class="detail-val">${escapeHtml(updatedFull)}</span>
            </div>
            ${planTier ? `
            <div class="detail-item">
              <span class="detail-key">账户计划 (Plan Tier)</span>
              <span class="detail-val">${escapeHtml(planTier)}</span>
            </div>` : ''}
          </div>
        </div>
      </div>
    `;
  }

  container.innerHTML = html;

  // 绑定卡片点击展开事件
  container.querySelectorAll('.session-box').forEach(box => {
    box.addEventListener('click', (e) => {
      // 避免选中文字时触发折叠
      const selection = window.getSelection();
      if (selection && selection.toString().length > 0) return;

      const sid = box.getAttribute('data-session-id');
      if (!sid) return;
      if (expandedSessionIds.has(sid)) {
        expandedSessionIds.delete(sid);
      } else {
        expandedSessionIds.add(sid);
      }
      renderSessions(sessions, true);
    });
  });
}

// 5. 调试原始上报抽屉 (语法高亮与安全内存复制)
async function loadDebugData() {
  const container = document.getElementById('debug-container');
  if (!container) return;

  try {
    const res = await fetch(getApiUrl('api/v1/debug/raw?limit=5'));
    if (!res.ok) {
      container.innerHTML = '<div class="empty-placeholder">获取调试数据失败</div>';
      return;
    }
    const reports = await res.json();
    if (!reports || reports.length === 0) {
      container.innerHTML = '<div class="empty-placeholder">暂无任何上报事件记录</div>';
      return;
    }

    rawReportsCache.clear();
    let html = '';
    for (const r of reports) {
      const jsonFormatted = JSON.stringify(r.raw_payload, null, 2);
      rawReportsCache.set(String(r.id), jsonFormatted);

      html += `
        <div class="debug-card">
          <div class="debug-card-header">
            <div class="debug-meta">
              <strong>#${r.id}</strong>
              <span>${escapeHtml(r.reported_at)}</span>
              <span>节点: ${escapeHtml(r.node_id)}</span>
              <span>产品: ${escapeHtml(r.product_type)}</span>
              <span>模型: ${escapeHtml(r.model_id)}</span>
            </div>
            <button class="btn-action btn-sm btn-copy" data-report-id="${r.id}">复制 JSON</button>
          </div>
          <pre class="debug-json"><code>${syntaxHighlightJSON(r.raw_payload)}</code></pre>
        </div>
      `;
    }
    container.innerHTML = html;

    // 绑定复制按钮 (直接从内存缓存提取，避免超长 DOM 属性)
    container.querySelectorAll('.btn-copy').forEach(btn => {
      btn.addEventListener('click', (e) => {
        e.stopPropagation();
        const rid = btn.getAttribute('data-report-id');
        const json = rawReportsCache.get(rid) || '';
        navigator.clipboard.writeText(json).then(() => {
          btn.textContent = '已复制!';
          setTimeout(() => { btn.textContent = '复制 JSON'; }, 1500);
        });
      });
    });

  } catch (e) {
    container.innerHTML = `<div class="empty-placeholder">加载异常: ${escapeHtml(e.message)}</div>`;
  }
}

// 6. 全局定时刷新总逻辑
async function refreshDashboard(forceRedraw = false) {
  try {
    const sessionsPath = selectedNodeId
      ? `api/v1/sessions?node_id=${encodeURIComponent(selectedNodeId)}`
      : 'api/v1/sessions';

    const [overviewRes, dailyRes, quotasRes, sessionsRes] = await Promise.all([
      fetch(getApiUrl('api/v1/overview')),
      fetch(getApiUrl('api/v1/tokens/daily?days=14')),
      fetch(getApiUrl('api/v1/quotas')),
      fetch(getApiUrl(sessionsPath))
    ]);

    if (!overviewRes.ok || !sessionsRes.ok) {
      setConnectionState(false, '服务异常');
      return;
    }

    const overview = await overviewRes.json();
    const dailyTokens = dailyRes.ok ? await dailyRes.json() : [];
    const accountQuotas = quotasRes.ok ? await quotasRes.json() : [];
    const sessions = await sessionsRes.json();

    setConnectionState(true, '已连接');
    setText('last-updated', new Date().toLocaleTimeString());

    // 顶部卡片更新
    setText('val-sessions', overview.active_sessions_count || 0);
    const agyCount = (overview.source_breakdown && overview.source_breakdown.antigravity) || 0;
    const claudeCount = (overview.source_breakdown && overview.source_breakdown.claudecode) || 0;
    setText('val-sources', `${agyCount} agy · ${claudeCount} claude`);

    // 今日 Token
    const todayStr = new Date().toISOString().slice(0, 10);
    const todaySummary = (dailyTokens && dailyTokens.find(d => d.date === todayStr)) || null;
    const todayTokens = todaySummary ? todaySummary.total_tokens : 0;
    const todayIn = todaySummary ? todaySummary.total_input_tokens : 0;
    const todayOut = todaySummary ? todaySummary.total_output_tokens : 0;
    setText('val-tokens-today', formatTokens(todayTokens));
    setText('val-tokens-today-detail', `输入 ↑${formatTokens(todayIn)} · 输出 ↓${formatTokens(todayOut)}`);

    // 历史总 Token
    setText('val-tokens-total', formatTokens(overview.total_tokens || 0));
    setText('val-cost-total', `累计估算: ${formatUSD(overview.total_cost_usd || 0)}`);

    // 节点数与账号数
    const nodeCount = overview.active_nodes_count || knownNodes.length || 0;
    setText('val-nodes', nodeCount);
    setText('val-nodes-sub', `${nodeCount} 个在线节点`);

    const accountCount = accountQuotas.length || (overview.latest_account && overview.latest_account.email ? 1 : 0);
    setText('val-accounts', accountCount);
    setText('val-accounts-sub', `${accountCount} 个已上报账号`);

    // 渲染每日各模型 Token 表格
    renderDailyTokensTable(dailyTokens, forceRedraw);

    // 渲染按账号区分的配额池
    renderAccountQuotas(accountQuotas, forceRedraw);

    // 渲染会话列表
    renderSessions(sessions, forceRedraw);

  } catch (err) {
    console.error('[statusline] refreshDashboard error:', err);
    setConnectionState(false, '连接中断');
  }
}

// 调度下一次定时刷新 (10s 间隔，页面隐藏时静默暂停)
function scheduleNextRefresh() {
  if (refreshTimer) clearTimeout(refreshTimer);
  refreshTimer = setTimeout(async () => {
    if (!document.hidden) {
      await refreshDashboard();
    }
    scheduleNextRefresh();
  }, 10000);
}

// 7. 事件绑定与初始化
function init() {
  loadNodes();
  loadAliases();
  refreshDashboard(true);
  scheduleNextRefresh();

  // 监听页面前后台切换
  document.addEventListener('visibilitychange', () => {
    if (!document.hidden) {
      refreshDashboard();
      scheduleNextRefresh();
    }
  });

  // 节点筛选下拉事件
  const nodeSelect = document.getElementById('node-filter-select');
  if (nodeSelect) {
    nodeSelect.addEventListener('change', (e) => {
      selectedNodeId = e.target.value;
      refreshDashboard(true);
    });
  }

  // 立即刷新按钮 (使用 try...finally 精确控制按钮状态)
  const btnRefresh = document.getElementById('btn-manual-refresh');
  if (btnRefresh) {
    btnRefresh.addEventListener('click', async () => {
      btnRefresh.disabled = true;
      btnRefresh.textContent = '刷新中...';
      try {
        await fetch(getApiUrl('api/v1/refresh'), { method: 'POST' });
        await loadNodes();
        await loadAliases();
        await refreshDashboard(true);
        if (isDebugOpen) await loadDebugData();
      } catch (e) {
        console.error('[statusline] manual refresh error:', e);
      } finally {
        btnRefresh.textContent = '⟳ 立即刷新';
        btnRefresh.disabled = false;
      }
    });
  }

  // 别名映射面板切换与操作
  const btnToggleAliases = document.getElementById('btn-toggle-aliases');
  const btnCloseAlias = document.getElementById('btn-close-alias');
  const aliasModal = document.getElementById('alias-modal');
  const btnAddAlias = document.getElementById('btn-add-alias');

  if (btnToggleAliases && aliasModal) {
    btnToggleAliases.addEventListener('click', async () => {
      isAliasesOpen = !isAliasesOpen;
      aliasModal.style.display = isAliasesOpen ? 'block' : 'none';
      if (isAliasesOpen) {
        await loadAliases();
        renderAliasTable();
        aliasModal.scrollIntoView({ behavior: 'smooth' });
      }
    });
  }

  if (btnCloseAlias && aliasModal) {
    btnCloseAlias.addEventListener('click', () => {
      isAliasesOpen = false;
      aliasModal.style.display = 'none';
    });
  }

  if (btnAddAlias) {
    btnAddAlias.addEventListener('click', () => {
      addingNewAlias = true;
      editingAliasKey = null;
      newAliasPrefill = { key: '', name: '', short: '' };
      renderAliasTable();
      setTimeout(() => {
        const kInput = document.getElementById('input-new-key');
        if (kInput) kInput.focus();
      }, 50);
    });
  }

  // 别名 Tab 切换
  document.querySelectorAll('.alias-tab').forEach(tab => {
    tab.addEventListener('click', () => {
      const targetTab = tab.getAttribute('data-tab');
      if (!targetTab || targetTab === currentAliasTab) return;
      currentAliasTab = targetTab;
      addingNewAlias = false;
      editingAliasKey = null;
      document.querySelectorAll('.alias-tab').forEach(t => {
        t.classList.toggle('active', t.getAttribute('data-tab') === currentAliasTab);
      });
      renderAliasTable();
    });
  });

  // 调试面板切换
  const btnToggleDebug = document.getElementById('btn-toggle-debug');
  const btnCloseDebug = document.getElementById('btn-close-debug');
  const debugDrawer = document.getElementById('debug-drawer');

  if (btnToggleDebug && debugDrawer) {
    btnToggleDebug.addEventListener('click', () => {
      isDebugOpen = !isDebugOpen;
      debugDrawer.style.display = isDebugOpen ? 'block' : 'none';
      if (isDebugOpen) {
        loadDebugData();
        debugDrawer.scrollIntoView({ behavior: 'smooth' });
      }
    });
  }

  if (btnCloseDebug && debugDrawer) {
    btnCloseDebug.addEventListener('click', () => {
      isDebugOpen = false;
      debugDrawer.style.display = 'none';
    });
  }
}

// 确保无论脚本在何种时机执行（loading / interactive / complete），都能立即初始化
if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', init);
} else {
  init();
}
