function formatTokens(n) {
  if (!n || n < 0) return '0';
  if (n < 1000) return String(n);
  if (n < 10000) return (n / 1000).toFixed(1) + 'k';
  if (n < 1000000) return Math.round(n / 1000) + 'k';
  return (n / 1000000).toFixed(1) + 'M';
}

function getFillClass(pct) {
  if (pct >= 30) return 'fill-green';
  if (pct >= 10) return 'fill-yellow';
  return 'fill-red';
}

function escapeHtml(str) {
  if (!str) return '';
  return String(str)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

function drawTrendChart(metrics) {
  const svg = document.getElementById('trend-svg');
  if (!svg) return;

  if (!metrics || metrics.length < 2) {
    svg.innerHTML = `
      <text x="400" y="80" text-anchor="middle" fill="#64748b" font-size="13">
        数据采集中，产生 Token 交互后将自动绘制趋势...
      </text>
    `;
    return;
  }

  const width = 800;
  const height = 160;
  const padTop = 15;
  const padBottom = 20;
  const padLeft = 20;
  const padRight = 20;

  const minTime = metrics[0].timestamp;
  const maxTime = metrics[metrics.length - 1].timestamp;
  const timeSpan = Math.max(1, maxTime - minTime);

  let minVal = metrics[0].total_tokens;
  let maxVal = metrics[0].total_tokens;
  for (const m of metrics) {
    if (m.total_tokens < minVal) minVal = m.total_tokens;
    if (m.total_tokens > maxVal) maxVal = m.total_tokens;
  }
  const valSpan = Math.max(1, maxVal - minVal);

  const points = metrics.map(m => {
    const x = padLeft + ((m.timestamp - minTime) / timeSpan) * (width - padLeft - padRight);
    const y = height - padBottom - ((m.total_tokens - minVal) / valSpan) * (height - padTop - padBottom);
    return { x, y, tokens: m.total_tokens, ts: m.timestamp };
  });

  const pathD = points.reduce((acc, p, idx) => {
    return idx === 0 ? `M ${p.x} ${p.y}` : `${acc} L ${p.x} ${p.y}`;
  }, '');

  const areaD = `${pathD} L ${points[points.length - 1].x} ${height - padBottom} L ${points[0].x} ${height - padBottom} Z`;

  // 更新轴文本标签
  const startLabel = document.getElementById('chart-start-time');
  const endLabel = document.getElementById('chart-end-time');
  const midLabel = document.getElementById('chart-mid-tokens');
  if (startLabel) startLabel.textContent = new Date(minTime * 1000).toLocaleTimeString();
  if (endLabel) endLabel.textContent = new Date(maxTime * 1000).toLocaleTimeString();
  if (midLabel) midLabel.textContent = `区间变化: +${formatTokens(maxVal - minVal)} Tokens (峰值 ${formatTokens(maxVal)})`;

  const lastPt = points[points.length - 1];

  svg.innerHTML = `
    <defs>
      <linearGradient id="tokenGrad" x1="0" y1="0" x2="0" y2="1">
        <stop offset="0%" stop-color="#38bdf8" stop-opacity="0.35"/>
        <stop offset="100%" stop-color="#38bdf8" stop-opacity="0.0"/>
      </linearGradient>
    </defs>
    <!-- 背景网格参考线 -->
    <line x1="${padLeft}" y1="${padTop}" x2="${width - padRight}" y2="${padTop}" stroke="rgba(255,255,255,0.05)" stroke-dasharray="4"/>
    <line x1="${padLeft}" y1="${(height - padBottom + padTop)/2}" x2="${width - padRight}" y2="${(height - padBottom + padTop)/2}" stroke="rgba(255,255,255,0.05)" stroke-dasharray="4"/>
    <line x1="${padLeft}" y1="${height - padBottom}" x2="${width - padRight}" y2="${height - padBottom}" stroke="rgba(255,255,255,0.08)"/>

    <!-- 渐变阴影填充 -->
    <path d="${areaD}" fill="url(#tokenGrad)"/>
    <!-- 趋势折线 -->
    <path d="${pathD}" fill="none" stroke="#38bdf8" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"/>

    <!-- 最新数据点高亮与光晕 -->
    <circle cx="${lastPt.x}" cy="${lastPt.y}" r="6" fill="#38bdf8" opacity="0.3"/>
    <circle cx="${lastPt.x}" cy="${lastPt.y}" r="3.5" fill="#38bdf8"/>
  `;
}

async function refreshDashboard() {
  try {
    const [overviewRes, sessionsRes] = await Promise.all([
      fetch('/api/overview'),
      fetch('/api/sessions')
    ]);

    if (!overviewRes.ok || !sessionsRes.ok) {
      document.getElementById('connection-status').textContent = '服务异常';
      return;
    }

    const overview = await overviewRes.json();
    const sessions = await sessionsRes.json();

    document.getElementById('connection-status').textContent = '已连接';
    document.getElementById('last-updated').textContent = new Date().toLocaleTimeString();

    // 自动刷新频率展示
    const refreshSec = overview.refresh_interval_seconds;
    const refreshTag = document.getElementById('auto-refresh-tag');
    if (refreshTag) {
      if (refreshSec && refreshSec > 0) {
        refreshTag.textContent = `自动刷新: ${refreshSec}s`;
      } else {
        refreshTag.textContent = '自动刷新: 未启用';
      }
    }

    // 1. 顶部总览卡片
    document.getElementById('val-sessions').textContent = overview.active_sessions_count || 0;
    const agyCount = (overview.source_breakdown && overview.source_breakdown.antigravity) || 0;
    const claudeCount = (overview.source_breakdown && overview.source_breakdown.claudecode) || 0;
    document.getElementById('val-sources').textContent = `${agyCount} agy · ${claudeCount} claude`;

    document.getElementById('val-tokens').textContent = formatTokens(overview.total_tokens || 0);
    document.getElementById('val-tokens-detail').textContent =
      `输入 ↑${formatTokens(overview.total_input_tokens || 0)} · 输出 ↓${formatTokens(overview.total_output_tokens || 0)}`;

    // 账号信息展示
    if (overview.latest_account && (overview.latest_account.email || overview.latest_account.plan_tier)) {
      const email = overview.latest_account.email || '已授权账号';
      const tier = overview.latest_account.plan_tier || '默认订阅';
      document.getElementById('val-account-email').textContent = email;
      document.getElementById('val-account-tier').textContent = `订阅级别: ${tier}`;

      // 顶部右侧徽章展示
      const headerAcc = document.getElementById('header-account');
      headerAcc.style.display = 'inline-flex';
      document.getElementById('header-account-email').textContent = email;
      document.getElementById('header-account-tier').textContent = tier;
    } else {
      document.getElementById('val-account-email').textContent = '未绑定账号';
      document.getElementById('val-account-tier').textContent = '暂无订阅数据';
    }

    // 模型信息展示
    if (overview.model_breakdown && Object.keys(overview.model_breakdown).length > 0) {
      const modelNames = Object.keys(overview.model_breakdown);
      document.getElementById('val-active-model').textContent = modelNames[0];
      document.getElementById('val-model-count').textContent = `共运行 ${modelNames.length} 种模型`;
    } else {
      document.getElementById('val-active-model').textContent = '-';
      document.getElementById('val-model-count').textContent = '等待会话激活';
    }

    // 速率展示
    const rate = Math.round(overview.token_rate_per_min || 0);
    const rateEl = document.getElementById('val-token-rate');
    if (rateEl) {
      rateEl.textContent = `${formatTokens(rate)} / min`;
    }

    // 绘制 Token 趋势图
    drawTrendChart(overview.metrics);

    // 2. 配额卡片更新
    if (overview.latest_quota) {
      const g = overview.latest_quota.gemini || { short_percentage: 100, short_reset: '--', long_percentage: 100, long_reset: '--' };
      const c = overview.latest_quota.claude || { short_percentage: 100, short_reset: '--', long_percentage: 100, long_reset: '--' };

      // Gemini
      document.getElementById('gemini-short-label').textContent = `${g.short_percentage}%`;
      const gShortBar = document.getElementById('gemini-short-bar');
      gShortBar.style.width = `${Math.min(100, g.short_percentage)}%`;
      gShortBar.className = `meter-fill ${getFillClass(g.short_percentage)}`;
      document.getElementById('gemini-short-time').textContent = `重置倒计时: ${g.short_reset}`;

      document.getElementById('gemini-long-label').textContent = `${g.long_percentage}%`;
      const gLongBar = document.getElementById('gemini-long-bar');
      gLongBar.style.width = `${Math.min(100, g.long_percentage)}%`;
      gLongBar.className = `meter-fill ${getFillClass(g.long_percentage)}`;
      document.getElementById('gemini-long-time').textContent = `重置倒计时: ${g.long_reset}`;

      // Claude
      document.getElementById('claude-short-label').textContent = `${c.short_percentage}%`;
      const cShortBar = document.getElementById('claude-short-bar');
      cShortBar.style.width = `${Math.min(100, c.short_percentage)}%`;
      cShortBar.className = `meter-fill ${getFillClass(c.short_percentage)}`;
      document.getElementById('claude-short-time').textContent = `重置倒计时: ${c.short_reset}`;

      document.getElementById('claude-long-label').textContent = `${c.long_percentage}%`;
      const cLongBar = document.getElementById('claude-long-bar');
      cLongBar.style.width = `${Math.min(100, c.long_percentage)}%`;
      cLongBar.className = `meter-fill ${getFillClass(c.long_percentage)}`;
      document.getElementById('claude-long-time').textContent = `重置倒计时: ${c.long_reset}`;
    }

    // 3. 会话卡片列表渲染
    const container = document.getElementById('sessions-container');
    const tip = document.getElementById('session-count-tip');
    if (!sessions || sessions.length === 0) {
      container.innerHTML = '<div class="empty-placeholder">暂未接收到任何会话遥测数据，请在终端中启动 agy 会话...</div>';
      tip.textContent = '检测到 0 个活跃会话';
      return;
    }

    tip.textContent = `检测到 ${sessions.length} 个活跃会话`;
    let html = '';
    for (const sess of sessions) {
      const isClaude = sess.source === 'claudecode';
      const sourceClass = isClaude ? 'source-claude' : 'source-agy';
      const sourceName = isClaude ? 'ClaudeCode' : 'Antigravity';
      const isWorking = sess.agent_state === 'working';
      const stateClass = isWorking ? 'state-pill-working' : 'state-pill-idle';
      const stateName = isWorking ? '⚡ 运行中' : '空闲';
      const updatedTime = new Date(sess.timestamp * 1000).toLocaleTimeString();
      const inTokens = formatTokens(sess.tokens ? sess.tokens.total_input_tokens : 0);
      const outTokens = formatTokens(sess.tokens ? sess.tokens.total_output_tokens : 0);
      const modelName = sess.model ? (sess.model.display_name || sess.model.id) : 'Gemini';
      const effort = sess.model && sess.model.effort ? ` (${sess.model.effort})` : '';
      const title = sess.conversation_title || '默认会话';
      const email = (sess.account && sess.account.email) ? sess.account.email : '';
      const tier = (sess.account && sess.account.plan_tier) ? sess.account.plan_tier : '';
      const accountBadge = email ? `<span class="account-badge">👤 ${escapeHtml(email)}${tier ? ' · ' + escapeHtml(tier) : ''}</span>` : '';
      const pidBadge = sess.pid ? `<span class="pid-badge">PID: ${sess.pid}</span>` : '';
      const ctxPct = sess.context_window ? Math.round(sess.context_window.used_percentage) : 0;
      const ctxSize = sess.context_window && sess.context_window.context_window_size ? formatTokens(sess.context_window.context_window_size) : '1M';

      html += `
        <div class="session-box">
          <div class="session-top">
            <div class="session-tags">
              <span class="tag-badge ${sourceClass}">${escapeHtml(sourceName)}</span>
              <span class="tag-badge ${stateClass}">${escapeHtml(stateName)}</span>
              ${pidBadge}
              ${accountBadge}
            </div>
            <span class="time-stamp">更新时间: ${escapeHtml(updatedTime)}</span>
          </div>
          <div class="session-mid">
            <span class="session-title">${escapeHtml(title)}</span>
            <span class="model-pill">[${escapeHtml(modelName)}${escapeHtml(effort)}]</span>
          </div>
          <div class="session-path">${escapeHtml(sess.cwd || '~')}</div>
          <div class="session-stats-bar">
            <div>输入: <span class="metric-val">↑${inTokens}</span></div>
            <div>输出: <span class="metric-val">↓${outTokens}</span></div>
            <div>上下文: <span class="metric-val">${ctxPct}% [${ctxSize}]</span></div>
          </div>
        </div>
      `;
    }
    container.innerHTML = html;

  } catch (err) {
    document.getElementById('connection-status').textContent = '连接中断';
  }
}

// 启动执行并每 2 秒定时轮询
refreshDashboard();
setInterval(refreshDashboard, 2000);

// 手动刷新按钮逻辑
const btnRefresh = document.getElementById('btn-manual-refresh');
if (btnRefresh) {
  btnRefresh.addEventListener('click', async () => {
    btnRefresh.disabled = true;
    const origText = btnRefresh.textContent;
    btnRefresh.textContent = '刷新中...';
    try {
      const res = await fetch('/api/refresh', { method: 'POST' });
      if (res.ok) {
        const data = await res.json();
        btnRefresh.textContent = `已刷新 (${data.refreshed_count || 0})`;
      } else {
        btnRefresh.textContent = '刷新失败';
      }
    } catch (e) {
      btnRefresh.textContent = '连接失败';
    }
    setTimeout(() => {
      btnRefresh.textContent = origText;
      btnRefresh.disabled = false;
      refreshDashboard();
    }, 1200);
  });
}
