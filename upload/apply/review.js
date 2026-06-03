(function () {
    let token = '';
    let currentStatus = '';
    let showDeleted = false;

    async function api(path, options) {
        if (!options) options = {};
        if (!options.headers) options.headers = {};
        if (token) options.headers['X-Admin-Token'] = token;
        const resp = await fetch(path, options);
        if (resp.status === 403) {
            lock();
            return null;
        }
        return resp;
    }

    function lock() {
        token = '';
        document.getElementById('lockScreen').style.display = 'block';
        document.getElementById('panelScreen').style.display = 'none';
        document.getElementById('tokenInput').value = '';
    }

    async function unlock(key) {
        const resp = await fetch('/api/apply/auth', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ token: key }),
        });
        if (resp.ok) {
            token = key;
            document.getElementById('lockScreen').style.display = 'none';
            document.getElementById('panelScreen').style.display = 'block';
            await Promise.all([loadList(), loadIpMeta()]);
        }
    }

    const statusMap = { pending: '⏳ 待审核', approved: '✓ 已通过', rejected: '✗ 已拒绝' };
    const statusClass = { pending: 'pending', approved: 'approved', rejected: 'rejected' };

    function escapeHtml(str) {
        if (!str) return '';
        return String(str)
            .replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;')
            .replace(/"/g, '&quot;')
            .replace(/'/g, '&#039;');
    }

    function groupAvatar(id) {
        return 'https://p.qlogo.cn/gh/' + id + '/' + id + '/100';
    }
    function userAvatar(qq) {
        return 'https://q.qlogo.cn/headimg_dl?dst_uin=' + qq + '&spec=100';
    }

    function renderVerifyBadge(record) {
        const v = record.verified;
        if (v == null && !record.verified_at) {
            return '<span style="color: #c9a050; font-size: 0.85rem;">⏳ 待验证</span>';
        }
        if (v == null) {
            return '<span style="color: #ffa940; font-size: 0.85rem;" title="' + escapeHtml(record.verification_note || '') + '">⚠️ 无法验证</span>';
        }
        if (v === true) {
            return '<span style="color: #44c7b4; font-size: 0.85rem;">✅ 已验证</span>';
        }
        if (v === false && record.status === 'rejected') {
            return '<span style="color: #d06060; font-size: 0.85rem;" title="' + escapeHtml(record.verification_note || '') + '">❌ 验证失败（已自动拒绝）</span>';
        }
        return '<span style="color: #ffa940; font-size: 0.85rem;" title="' + escapeHtml(record.verification_note || '') + '">⚠️ 无法验证</span>';
    }

    function renderList(records) {
        const container = document.getElementById('appList');
        document.getElementById('totalCount').textContent = '共 ' + records.length + ' 条';
        if (records.length === 0) {
            container.innerHTML = '<p style="color:#9b8eb8;text-align:center;padding:40px;">暂无申请记录</p>';
            return;
        }
        container.innerHTML = records.map(r => {
            const canAct = r.status === 'pending';
            const groupTitle = r.group_name || ('群 ' + r.group_id);
            const applicantTitle = r.applicant_nickname || r.applicant;
            return '<div class="app-card ' + (statusClass[r.status] || '') + '">' +
                // Group profile
                '<div class="profile-row">' +
                '<img src="' + groupAvatar(r.group_id) + '" onerror="this.style.display=\'none\'" alt="">' +
                '<div class="profile-info">' +
                '<div class="name">' + escapeHtml(groupTitle) + '</div>' +
                '<div class="sub">群号 ' + r.group_id + ' · ' + r.member_count + '人</div>' +
                '</div>' +
                '<span class="pill ' + (statusClass[r.status] || '') + '">' + (statusMap[r.status] || r.status) + '</span>' +
                '</div>' +
                // Applicant profile
                '<div class="profile-row">' +
                '<img src="' + userAvatar(r.applicant) + '" onerror="this.style.display=\'none\'" alt="">' +
                '<div class="profile-info">' +
                '<div class="name" style="font-size:0.82rem;">' + escapeHtml(applicantTitle) + '</div>' +
                '<div class="sub">QQ ' + r.applicant + '</div>' +
                '</div>' +
                '</div>' +
                '<hr class="card-divider">' +
                '<div style="font-size: 0.8rem; color: #9b8eb8; margin-bottom: 4px;">' +
                'IP: <span class="app-ip">' + escapeHtml(r.client_ip || '未知') + '</span></div>' +
                '<div class="verify-badge" style="margin-bottom: 4px;">' + renderVerifyBadge(r) + '</div>' +
                '<div class="app-card-body">' + escapeHtml(r.purpose) + '</div>' +
                '<div class="app-card-meta">' + new Date(r.created_at).toLocaleString('zh-CN') + '</div>' +
                (r.admin_note ? '<div class="app-card-note" style="font-size:0.82rem;color:#d06060;margin-top:4px;">' + escapeHtml(r.admin_note) +
                ' <button onclick="window._applyEditNote(\'' + r.id + '\',this)" title="编辑备注" style="font-size:0.7rem;padding:0 6px;border:1px solid #e2ddf0;border-radius:4px;background:#fff;color:#9b8eb8;cursor:pointer;">✎</button></div>' : '') +
                '<div class="app-card-actions">' +
                (canAct ? '<button class="btn-approve" onclick="window._applyApprove(\'' + r.id + '\')">通过</button>' +
                '<button class="btn-reject" onclick="window._applyReject(\'' + r.id + '\')">拒绝</button>' : '') +
                (r.status !== 'pending' ? '<button class="btn-revoke" onclick="window._applyRevoke(\'' + r.id + '\', this)" style="margin-left: 8px;">撤销</button>' : '') +
                '<button class="btn-delete" onclick="window._applyDelete(\'' + r.id + '\', this)" style="margin-left: 8px;padding:4px 12px;border:1px solid #e2ddf0;border-radius:8px;background:#fff;color:#9b8eb8;cursor:pointer;font-size:0.78rem;">删除</button>' +
                '</div>' +
                '</div>';
        }).join('');
    }

    async function loadList() {
        const params = [];
        if (currentStatus) params.push('status=' + currentStatus);
        if (showDeleted) params.push('show_deleted=1');
        const query = params.length ? '?' + params.join('&') : '';
        const resp = await api('/api/apply/list' + query);
        if (resp) {
            const records = await resp.json();
            renderList(records);
        }
    }

    // ── IP Meta Management ──

    function renderIpPanel(meta) {
        const body = document.getElementById('ipPanelBody');
        const bl = meta.ip_blacklist || [];
        const fc = meta.ip_fake_counts || {};
        const fcEntries = Object.entries(fc).sort((a, b) => b[1] - a[1]);

        let html = '';

        if (bl.length === 0 && fcEntries.length === 0) {
            html = '<p style="color:#9b8eb8;font-size:0.82rem;">暂无 IP 数据</p>';
        }

        if (bl.length > 0) {
            html += '<div style="margin-bottom:10px;"><span style="font-weight:600;font-size:0.8rem;">黑名单</span></div>';
            html += '<table class="ip-table"><tr><th>IP</th><th>操作</th></tr>';
            bl.forEach(function (ip) {
                html += '<tr><td class="mono">' + escapeHtml(ip) + '</td>' +
                    '<td><button class="btn-ip-remove" onclick="window._ipRemove(\'' + ip + '\')">移除</button></td></tr>';
            });
            html += '</table>';
        }

        if (fcEntries.length > 0) {
            html += '<div style="margin-top:12px;margin-bottom:6px;"><span style="font-weight:600;font-size:0.8rem;">虚假申请计数</span></div>';
            html += '<table class="ip-table"><tr><th>IP</th><th>次数</th></tr>';
            fcEntries.forEach(function (entry) {
                const ip = entry[0];
                const count = entry[1];
                const isBlocked = bl.indexOf(ip) >= 0;
                html += '<tr><td class="mono">' + escapeHtml(ip) + (isBlocked ? ' <span style="color:#d06060;font-size:0.7rem;">[已封]</span>' : '') + '</td>' +
                    '<td>' + count + '</td></tr>';
            });
            html += '</table>';
        }

        html += '<div style="margin-top:10px;display:flex;gap:8px;">' +
            '<input type="text" id="newIpInput" placeholder="输入 IP 地址" style="padding:5px 10px;border:1px solid #e2ddf0;border-radius:6px;font-size:0.78rem;flex:1;">' +
            '<button class="btn-ip-add" onclick="window._ipAdd()">封禁 IP</button>' +
            '</div>';

        body.innerHTML = html;
    }

    async function loadIpMeta() {
        const resp = await api('/api/apply/meta');
        if (resp) {
            const meta = await resp.json();
            renderIpPanel(meta);
        }
    }

    async function ipRemove(ip) {
        if (!confirm('确认将 ' + ip + ' 移出黑名单？')) return;
        const resp = await api('/api/apply/meta/ip-blacklist', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ action: 'remove', ip: ip }),
        });
        if (resp && resp.ok) {
            await loadIpMeta();
        }
    }

    async function ipAdd() {
        const input = document.getElementById('newIpInput');
        const ip = (input && input.value || '').trim();
        if (!ip) return;
        if (!confirm('确认封禁 IP ' + ip + '？')) return;
        const resp = await api('/api/apply/meta/ip-blacklist', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ action: 'add', ip: ip }),
        });
        if (resp && resp.ok) {
            await loadIpMeta();
        }
    }

    window._ipRemove = ipRemove;
    window._ipAdd = ipAdd;

    // ── Actions ──

    async function approve(id) {
        const note = prompt('审核备注（可选）：');
        if (note === null) return;
        const resp = await api('/api/apply/' + id + '/approve', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ admin_note: note.trim() }),
        });
        if (resp && resp.ok) await loadList();
    }
    async function reject(id) {
        const note = prompt('拒绝原因（可选，将展示给申请人）：');
        if (note === null) return;
        const resp = await api('/api/apply/' + id + '/reject', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ admin_note: note.trim() }),
        });
        if (resp && resp.ok) await loadList();
    }
    async function revokeApplication(appId, btn) {
        if (!token) return;
        const originalText = btn.textContent;
        btn.disabled = true;
        btn.textContent = '撤销中...';
        try {
            const resp = await fetch('/api/apply/' + appId + '/revoke', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json', 'X-Admin-Token': token },
                body: JSON.stringify({ admin_note: '管理员撤销' }),
            });
            if (resp.status === 403) { lock(); return; }
            if (resp.ok) {
                await loadList();
            }
        } catch (e) {
            console.error('撤销失败:', e);
        } finally {
            btn.disabled = false;
            btn.textContent = originalText;
        }
    }

    async function editNote(appId, btn) {
        if (!token) return;
        const note = prompt('编辑备注：');
        if (note === null) return;
        const originalText = btn.textContent;
        btn.disabled = true;
        btn.textContent = '...';
        try {
            const resp = await fetch('/api/apply/' + appId + '/note', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json', 'X-Admin-Token': token },
                body: JSON.stringify({ admin_note: note.trim() }),
            });
            if (resp.status === 403) { lock(); return; }
            if (resp.ok) {
                await loadList();
            }
        } catch (e) {
            console.error('编辑备注失败:', e);
        } finally {
            btn.disabled = false;
            btn.textContent = originalText;
        }
    }

    async function deleteRecord(appId, btn) {
        if (!token) return;
        if (!confirm('确认删除此申请记录？')) return;
        const originalText = btn.textContent;
        btn.disabled = true;
        btn.textContent = '...';
        try {
            const resp = await fetch('/api/apply/' + appId + '/delete', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json', 'X-Admin-Token': token },
            });
            if (resp.status === 403) { lock(); return; }
            if (resp.ok) {
                await loadList();
            }
        } catch (e) {
            console.error('删除失败:', e);
        } finally {
            btn.disabled = false;
            btn.textContent = originalText;
        }
    }

    // 绑定全局函数供 onclick 使用
    window._applyApprove = approve;
    window._applyReject = reject;
    window._applyRevoke = revokeApplication;
    window._applyEditNote = editNote;
    window._applyDelete = deleteRecord;

    // 事件绑定
    document.getElementById('unlockBtn').addEventListener('click', function () {
        const key = document.getElementById('tokenInput').value.trim();
        if (key) unlock(key);
    });
    document.getElementById('tokenInput').addEventListener('keypress', function (e) {
        if (e.key === 'Enter') {
            const key = this.value.trim();
            if (key) unlock(key);
        }
    });

    document.getElementById('ipPanelToggle').addEventListener('click', function () {
        document.getElementById('ipPanel').classList.toggle('open');
    });

    document.querySelectorAll('.filter-tab').forEach(function (tab) {
        tab.addEventListener('click', function () {
            if (this.dataset.showDeleted) {
                showDeleted = !showDeleted;
                this.classList.toggle('active', showDeleted);
                loadList();
                return;
            }
            showDeleted = false;
            document.getElementById('showDeletedTab').classList.remove('active');
            document.querySelectorAll('.filter-tab').forEach(function (t) { t.classList.remove('active'); });
            this.classList.add('active');
            currentStatus = this.dataset.status || '';
            loadList();
        });
    });

    // 初始锁定
    lock();
})();
