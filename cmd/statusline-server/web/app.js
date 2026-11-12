// State
let selectedNodeId = '';
let isDebugOpen = false;
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

      html += `
        <tr>
          <td class="sub-row-cell">${subPrefix}</td>
          <td><span class="model-tag">${escapeHtml(m.display_name || m.model_id)}</span></td>
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

// 提取指定 Provider (Gemini / Claude) 的配额窗口列表
function extractProviderWindows(acc, providerType) {
  const windows = [];
  const isMatchKey = (key) => {
    const k = (key || '').toLowerCase();
    if (providerType === 'gemini') {
      return k.includes('gemini');
    }
    return k.includes('claude') || k.includes('sonnet') || k.includes('opus') || k.includes('3p');
  };

  if (acc.buckets && Array.isArray(acc.buckets) && acc.buckets.length > 0) {
    const matched = acc.buckets.filter(b => isMatchKey(b.quota_key));
    for (const b of matched) {
      const isShort = b.reset_in_seconds <= 86400; // 1天以内视为短期窗口
      const label = isShort ? '5h' : '7d';
      windows.push({
        label,
        isShort,
        pct: b.remaining_pct != null ? b.remaining_pct : Math.round(b.remaining_fraction * 100),
        countdown: b.reset_countdown || '--',
        resetInSec: b.reset_in_seconds || 0
      });
    }
  }

  // 若 buckets 未匹配到，从兼容字段 gemini / claude 中回退提取
  if (windows.length === 0) {
    const p = providerType === 'gemini' ? acc.gemini : acc.claude;
    if (p) {
      if (p.short_percentage != null || p.short_reset) {
        windows.push({
          label: '5h',
          isShort: true,
          pct: p.short_percentage != null ? p.short_percentage : 100,
          countdown: p.short_reset || '--',
          resetInSec: parseCountdownToSeconds(p.short_reset)
        });
      }
      if (p.long_percentage != null || p.long_reset) {
        windows.push({
          label: '7d',
          isShort: false,
          pct: p.long_percentage != null ? p.long_percentage : 100,
          countdown: p.long_reset || '--',
          resetInSec: parseCountdownToSeconds(p.long_reset)
        });
      }
    }
  }

  return windows;
}

// 3. 渲染按账号区分的模型配额池 (紧凑彩色同心双环仪表盘，带防重绘闪烁)
function renderAccountQuotas(accountQuotas, forceRedraw = false) {
  const container = document.getElementById('quotas-container');
  const tip = document.getElementById('quota-accounts-tip');
  if (!container) return;

  const cacheKey = JSON.stringify(accountQuotas || []);
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
    const lastNode = acc.last_node_id ? `上报节点: ${escapeHtml(acc.last_node_id)}` : '';
    let updateTimeStr = '--';
    if (acc.updated_at) {
      const d = new Date(acc.updated_at);
      updateTimeStr = isNaN(d.getTime()) ? acc.updated_at : d.toLocaleTimeString();
    }

    let groupsHtml = '';

    // Gemini Provider
    const geminiWindows = extractProviderWindows(acc, 'gemini');
    if (geminiWindows.length > 0) {
      const rings = geminiWindows.map(w => renderConcentricRing(w.pct, w.countdown, w.resetInSec, w.isShort, w.label)).join('');
      groupsHtml += `
        <div class="quota-provider-group gemini-group">
          <div class="provider-badge-row">
            <span class="provider-pill gemini-pill">Gemini</span>
          </div>
          <div class="provider-rings-row">
            ${rings}
          </div>
        </div>
      `;
    }

    // Claude Provider
    const claudeWindows = extractProviderWindows(acc, 'claude');
    if (claudeWindows.length > 0) {
      const rings = claudeWindows.map(w => renderConcentricRing(w.pct, w.countdown, w.resetInSec, w.isShort, w.label)).join('');
      groupsHtml += `
        <div class="quota-provider-group claude-group">
          <div class="provider-badge-row">
            <span class="provider-pill claude-pill">Claude</span>
          </div>
          <div class="provider-rings-row">
            ${rings}
          </div>
        </div>
      `;
    }

    if (!groupsHtml) {
      groupsHtml = '<div class="empty-placeholder" style="padding: 10px 14px; font-size: 12px; margin: 0;">该账号暂无 Gemini / Claude 额度限制</div>';
    }

    html += `
      <div class="account-quota-card compact-card">
        <div class="account-card-header compact-header">
          <div class="account-badge-email">
            <span class="user-avatar-icon">👤</span>
            <span class="account-email-text">${escapeHtml(email)}</span>
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
    const modelName = sess.model ? (sess.model.display_name || sess.model.id) : 'Gemini';
    const title = sess.conversation_title || sess.session_name || '默认终端会话';
    const email = (sess.account && sess.account.email) ? sess.account.email : '';
    const planTier = (sess.account && sess.account.plan_tier) ? sess.account.plan_tier : '';
    const accountBadge = email ? `<span class="account-badge">👤 ${escapeHtml(email)}</span>` : '';
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
