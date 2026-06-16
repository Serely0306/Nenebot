(function () {
    var apiBase = '/api/apply';

    function showMessage(text, type) {
        var el = document.getElementById('message');
        el.textContent = text;
        el.className = 'message ' + type;
    }
    function hideMessage() {
        document.getElementById('message').className = 'message';
    }
    function escapeHtml(str) {
        if (!str) return '';
        return String(str)
            .replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;')
            .replace(/"/g, '&quot;')
            .replace(/'/g, '&#039;');
    }

    // 提交表单
    document.getElementById('applyForm').addEventListener('submit', async function (e) {
        e.preventDefault();
        var btn = this.querySelector('.btn-submit');
        btn.disabled = true;
        hideMessage();

        var data = {
            group_id: document.getElementById('group_id').value.trim(),
            purpose: document.getElementById('purpose').value.trim(),
            applicant: document.getElementById('applicant').value.trim(),
        };

        try {
            var resp = await fetch(apiBase, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(data),
            });
            var result = await resp.json();
            if (resp.ok && result.success) {
                showMessage('申请已提交！', 'success');
                document.getElementById('applyForm').reset();
            } else {
                showMessage(result.error || '提交失败', 'error');
            }
        } catch (err) {
            showMessage('网络错误，请重试', 'error');
        }
        btn.disabled = false;
    });

    // 查询状态
    document.getElementById('queryBtn').addEventListener('click', async function () {
        var qq = document.getElementById('queryQQ').value.trim();
        var container = document.getElementById('queryResult');
        if (!qq || !/^\d+$/.test(qq)) {
            container.innerHTML = '<p style="color:#d06060;font-size:0.85rem;">请输入有效的 QQ 号</p>';
            return;
        }

        try {
            var resp = await fetch(apiBase + '/status?applicant=' + encodeURIComponent(qq));
            var results = await resp.json();
            if (!Array.isArray(results) || results.length === 0) {
                container.innerHTML = '<p style="color:#9b8eb8;font-size:0.85rem;">暂无申请记录</p>';
                return;
            }
            var statusMap = { pending: '⏳ 待审核', approved: '✓ 已通过', rejected: '✗ 已拒绝' };
            container.innerHTML = results.map(function (r) {
                return '<div class="status-card">' +
                    '<div><div class="group-info">群 ' + escapeHtml(r.group_id) + '</div>' +
                    '<div class="group-meta">' + escapeHtml(r.purpose) + (r.member_count > 0 ? ' · ' + r.member_count + '人' : '') + '</div>' +
                    (r.admin_note ? '<div class="group-meta" style="color:#d06060;margin-top:2px;">' + escapeHtml(r.admin_note) + '</div>' : '') +
                    '</div>' +
                    '<span class="pill ' + r.status + '">' + (statusMap[r.status] || r.status) + '</span>' +
                    '</div>';
            }).join('');
        } catch (err) {
            container.innerHTML = '<p style="color:#d06060;font-size:0.85rem;">查询失败</p>';
        }
    });
})();
