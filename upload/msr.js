document.addEventListener('DOMContentLoaded', () => {
    const accessGate = document.getElementById('accessGate');
    const mainApp = document.getElementById('mainApp');
    const accessKeyInput = document.getElementById('accessKey');
    const validateBtn = document.getElementById('validateBtn');
    const regionSelect = document.getElementById('region');
    const qqIdInput = document.getElementById('qqId');
    const queryBtn = document.getElementById('queryBtn');
    const queryResult = document.getElementById('queryResult');
    const step2 = document.getElementById('step2');
    const accountsList = document.getElementById('accountsList');
    const selectedAccountInfo = document.getElementById('selectedAccountInfo');
    const renderBtn = document.getElementById('renderBtn');
    const resultPanel = document.getElementById('resultPanel');
    const resultMeta = document.getElementById('resultMeta');
    const resultImage = document.getElementById('resultImage');
    const downloadLink = document.getElementById('downloadLink');
    const message = document.getElementById('message');

    let verifiedAccessKey = '';
    let selectedAccount = null;
    let resultUrl = null;

    accessKeyInput.addEventListener('keypress', (event) => {
        if (event.key === 'Enter') {
            validateBtn.click();
        }
    });

    qqIdInput.addEventListener('keypress', (event) => {
        if (event.key === 'Enter' && !queryBtn.disabled) {
            queryBtn.click();
        }
    });

    qqIdInput.addEventListener('input', updateQueryButtonState);

    validateBtn.addEventListener('click', async () => {
        const accessKey = accessKeyInput.value.trim();
        if (!accessKey) {
            showMessage('请输入 access_key', 'error');
            return;
        }

        setButtonLoading(validateBtn, true);
        hideMessage();

        try {
            const response = await fetch('/api/msr/validate_access_key', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ access_key: accessKey }),
            });
            const data = await response.json();

            if (!response.ok || !data.success) {
                throw new Error(data.error || `HTTP ${response.status}`);
            }

            verifiedAccessKey = accessKey;
            accessGate.style.display = 'none';
            mainApp.style.display = 'block';
            hideMessage();
            updateQueryButtonState();
        } catch (error) {
            showMessage(`access_key 验证失败: ${error.message}`, 'error');
        } finally {
            setButtonLoading(validateBtn, false);
        }
    });

    queryBtn.addEventListener('click', async () => {
        const qqId = qqIdInput.value.trim();
        const region = regionSelect.value;

        if (!verifiedAccessKey) {
            showMessage('请先验证 access_key', 'error');
            return;
        }
        if (!qqId) {
            showMessage('请输入 QQ 号', 'error');
            return;
        }
        if (!/^\d+$/.test(qqId)) {
            showMessage('QQ 号格式不正确', 'error');
            return;
        }

        setButtonLoading(queryBtn, true);
        hideMessage();
        resetResult();
        queryResult.style.display = 'none';
        step2.style.display = 'none';
        selectedAccount = null;
        selectedAccountInfo.innerHTML = '';
        accountsList.innerHTML = '';
        updateRenderButtonState();

        try {
            const response = await fetch('/api/query_binding', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ qq_id: qqId, region }),
            });
            const data = await response.json();

            if (!response.ok) {
                throw new Error(data.error || `HTTP ${response.status}`);
            }
            if (!data.success) {
                throw new Error(data.error || '未找到绑定账号');
            }

            queryResult.innerHTML = `<div class="query-success">查询成功，找到 ${data.accounts.length} 个绑定账号</div>`;
            queryResult.style.display = 'block';
            renderAccounts(data.accounts);
            step2.style.display = 'block';
        } catch (error) {
            queryResult.innerHTML = `<div class="query-error">查询失败: ${escapeHtml(error.message)}</div>`;
            queryResult.style.display = 'block';
        } finally {
            setButtonLoading(queryBtn, false);
        }
    });

    renderBtn.addEventListener('click', async () => {
        const qqId = qqIdInput.value.trim();
        const region = regionSelect.value;

        if (!verifiedAccessKey) {
            showMessage('请先验证 access_key', 'error');
            return;
        }
        if (!selectedAccount) {
            showMessage('请先选择账号', 'error');
            return;
        }

        setButtonLoading(renderBtn, true);
        hideMessage();
        resetResult();

        try {
            const response = await fetch('/api/msr/query', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    qq_id: qqId,
                    region,
                    game_id: selectedAccount.game_id,
                    access_key: verifiedAccessKey,
                }),
            });

            if (!response.ok) {
                const errorData = await response.json().catch(() => ({ error: `HTTP ${response.status}` }));
                throw new Error(errorData.error || `HTTP ${response.status}`);
            }

            const blob = await response.blob();
            resultUrl = URL.createObjectURL(blob);
            resultImage.src = resultUrl;
            downloadLink.href = resultUrl;
            downloadLink.download = `msr_${region}_${selectedAccount.game_id}.png`;
            resultMeta.textContent = `QQ ${qqId} / 账号 ${selectedAccount.display_id}`;
            resultPanel.style.display = 'block';
            showMessage('MSR 地图查询成功', 'success');
        } catch (error) {
            showMessage(`MSR 查询失败: ${error.message}`, 'error');
        } finally {
            setButtonLoading(renderBtn, false);
        }
    });

    function renderAccounts(accounts) {
        accountsList.innerHTML = '';
        selectedAccount = null;

        accounts.forEach((account) => {
            const item = document.createElement('div');
            item.className = 'account-item';
            item.innerHTML = `
                <div class="account-info">
                    <span class="account-index">账号 ${account.index}</span>
                    <span class="account-id">${escapeHtml(account.display_id)}</span>
                    ${account.is_main ? '<span class="account-main-badge">主账号</span>' : ''}
                </div>
                <div class="account-select-icon">○</div>
            `;
            item.addEventListener('click', () => selectAccount(account, item));
            accountsList.appendChild(item);
        });
    }

    function selectAccount(account, item) {
        document.querySelectorAll('.account-item').forEach((element) => {
            element.classList.remove('selected');
            const icon = element.querySelector('.account-select-icon');
            if (icon) {
                icon.textContent = '○';
            }
        });

        item.classList.add('selected');
        const icon = item.querySelector('.account-select-icon');
        if (icon) {
            icon.textContent = '●';
        }

        selectedAccount = account;
        selectedAccountInfo.innerHTML = `
            <div class="selected-info">
                <span>当前选择账号 <strong>${escapeHtml(account.display_id)}</strong></span>
                ${account.is_main ? '<span class="badge">主账号</span>' : ''}
            </div>
        `;
        updateRenderButtonState();
    }

    function updateQueryButtonState() {
        queryBtn.disabled = !verifiedAccessKey || !qqIdInput.value.trim();
    }

    function updateRenderButtonState() {
        renderBtn.disabled = !verifiedAccessKey || !selectedAccount;
    }

    function setButtonLoading(button, loading) {
        button.classList.toggle('loading', loading);

        if (button === validateBtn) {
            button.disabled = loading;
            return;
        }
        if (button === queryBtn) {
            button.disabled = loading || !verifiedAccessKey || !qqIdInput.value.trim();
            return;
        }
        button.disabled = loading || !verifiedAccessKey || !selectedAccount;
    }

    function resetResult() {
        if (resultUrl) {
            URL.revokeObjectURL(resultUrl);
            resultUrl = null;
        }
        resultPanel.style.display = 'none';
        resultImage.removeAttribute('src');
        downloadLink.removeAttribute('href');
        resultMeta.textContent = '';
    }

    function showMessage(text, type) {
        message.textContent = text;
        message.className = `message ${type}`;
    }

    function hideMessage() {
        message.textContent = '';
        message.className = 'message';
    }

    function escapeHtml(value) {
        return String(value)
            .replaceAll('&', '&amp;')
            .replaceAll('<', '&lt;')
            .replaceAll('>', '&gt;')
            .replaceAll('"', '&quot;')
            .replaceAll("'", '&#39;');
    }

    updateQueryButtonState();
    updateRenderButtonState();
});
