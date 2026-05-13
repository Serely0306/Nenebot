document.addEventListener('DOMContentLoaded', function () {
    // 上传模式: 'direct' = 直接上传(suite), 'query' = 查询绑定后上传(mysekai)
    const uploadMode = window.UPLOAD_MODE || 'query';
    const dataType = window.UPLOAD_DATA_TYPE || 'suite';
    const apiBase = new URL('../api/', window.location.href);

    // 通用元素引用
    const regionSelect = document.getElementById('region');
    const uploadArea = document.getElementById('uploadArea');
    const fileInput = document.getElementById('fileInput');
    const fileInfo = document.getElementById('fileInfo');
    const fileName = document.getElementById('fileName');
    const fileSize = document.getElementById('fileSize');
    const clearFile = document.getElementById('clearFile');
    const uploadBtn = document.getElementById('uploadBtn');
    const progressContainer = document.getElementById('progressContainer');
    const progressFill = document.getElementById('progressFill');
    const progressText = document.getElementById('progressText');
    const message = document.getElementById('message');

    // 状态
    let selectedFile = null;
    let selectedAccount = null;

    // ==================== 查询模式专用 (mysekai) ====================
    if (uploadMode === 'query') {
        const qqIdInput = document.getElementById('qqId');
        const queryBtn = document.getElementById('queryBtn');
        const queryResult = document.getElementById('queryResult');
        const step2 = document.getElementById('step2');
        const step3 = document.getElementById('step3');
        const accountsList = document.getElementById('accountsList');
        const selectedAccountInfo = document.getElementById('selectedAccountInfo');
        let boundAccounts = [];

        // QQ 号输入回车触发查询
        if (qqIdInput) {
            qqIdInput.addEventListener('keypress', (e) => {
                if (e.key === 'Enter') queryBtn.click();
            });
        }

        // 查询绑定账号
        if (queryBtn) {
            queryBtn.addEventListener('click', async () => {
                const qqId = qqIdInput.value.trim();
                const region = regionSelect.value;

                if (!qqId) { showMessage('请输入 QQ 号', 'error'); return; }
                if (!/^\d+$/.test(qqId)) { showMessage('QQ 号格式不正确', 'error'); return; }

                queryBtn.disabled = true;
                queryBtn.classList.add('loading');
                hideMessage();

                try {
                    const response = await fetch(new URL('query_binding', apiBase), {
                        method: 'POST',
                        headers: { 'Content-Type': 'application/json' },
                        body: JSON.stringify({ qq_id: qqId, region: region })
                    });

                    let data;
                    const responseText = await response.text();
                    try {
                        data = JSON.parse(responseText);
                    } catch (e) {
                        if (response.status === 404) {
                            throw new Error('找不到查询接口，请确保已重启 server.py 以加载最新代码');
                        } else {
                            throw new Error(`服务器返回了非 JSON 响应 (Status ${response.status})`);
                        }
                    }

                    if (!data.success) {
                        queryResult.innerHTML = `<div class="query-error">${data.error}</div>`;
                        queryResult.style.display = 'block';
                        step2.style.display = 'none';
                        step3.style.display = 'none';
                        selectedAccount = null;
                        boundAccounts = [];
                    } else {
                        queryResult.innerHTML = `<div class="query-success">✓ 查询成功，找到 ${data.accounts.length} 个绑定账号</div>`;
                        queryResult.style.display = 'block';
                        boundAccounts = data.accounts;
                        renderAccountsList(data.accounts, data.region_name);
                        step2.style.display = 'block';
                        step3.style.display = 'none';
                        selectedAccount = null;
                    }
                } catch (error) {
                    queryResult.innerHTML = `<div class="query-error">查询失败: ${error.message}</div>`;
                    queryResult.style.display = 'block';
                } finally {
                    queryBtn.disabled = false;
                    queryBtn.classList.remove('loading');
                }
            });
        }

        // 渲染账号列表
        function renderAccountsList(accounts, regionName) {
            accountsList.innerHTML = '';
            accounts.forEach(account => {
                const div = document.createElement('div');
                div.className = 'account-item';
                div.dataset.gameId = account.game_id;
                div.innerHTML = `
                    <div class="account-info">
                        <span class="account-index">账号 ${account.index}</span>
                        <span class="account-id">${account.display_id}</span>
                        ${account.is_main ? '<span class="account-main-badge">主账号</span>' : ''}
                    </div>
                    <div class="account-select-icon">○</div>
                `;
                div.addEventListener('click', () => selectAccount(account, div));
                accountsList.appendChild(div);
            });
        }

        // 选择账号
        function selectAccount(account, element) {
            document.querySelectorAll('.account-item').forEach(el => {
                el.classList.remove('selected');
                el.querySelector('.account-select-icon').textContent = '○';
            });
            element.classList.add('selected');
            element.querySelector('.account-select-icon').textContent = '●';
            selectedAccount = account;
            step3.style.display = 'block';
            selectedAccountInfo.innerHTML = `
                <div class="selected-info">
                    <span>将为账号 <strong>${account.display_id}</strong> 上传数据</span>
                    ${account.is_main ? '<span class="badge">主账号</span>' : ''}
                </div>
            `;
            updateUploadButtonState();
        }
    }

    // ==================== 文件上传（通用） ====================

    // 点击上传区域触发文件选择
    uploadArea.addEventListener('click', () => fileInput.click());

    // 文件选择变化
    fileInput.addEventListener('change', (e) => {
        if (e.target.files.length > 0) handleFile(e.target.files[0]);
    });

    // 拖拽事件处理
    uploadArea.addEventListener('dragover', (e) => {
        e.preventDefault();
        uploadArea.classList.add('dragover');
    });
    uploadArea.addEventListener('dragleave', (e) => {
        e.preventDefault();
        uploadArea.classList.remove('dragover');
    });
    uploadArea.addEventListener('drop', (e) => {
        e.preventDefault();
        uploadArea.classList.remove('dragover');
        if (e.dataTransfer.files.length > 0) handleFile(e.dataTransfer.files[0]);
    });

    // 处理选中的文件
    function handleFile(file) {
        const knownExtensions = ['.json', '.bin', '.msgpack', '.dat'];
        const ext = file.name.toLowerCase().substring(file.name.lastIndexOf('.'));
        const hasExt = file.name.includes('.');

        // 有扩展名时检查是否为已知类型，无扩展名或 octet-stream 视为二进制文件放行
        if (hasExt && !knownExtensions.includes(ext) && file.type !== 'application/octet-stream' && file.type !== '') {
            showMessage('不支持的文件格式，请上传 JSON 或二进制文件', 'error');
            return;
        }
        if (file.size > 50 * 1024 * 1024) {
            showMessage('文件大小不能超过 50MB', 'error');
            return;
        }

        selectedFile = file;
        uploadArea.style.display = 'none';
        fileInfo.style.display = 'flex';
        fileName.textContent = file.name;
        fileSize.textContent = formatFileSize(file.size);
        hideMessage();
        updateUploadButtonState();
    }

    // 更新上传按钮状态
    function updateUploadButtonState() {
        if (uploadMode === 'direct') {
            // 直接上传模式：只需要文件
            uploadBtn.disabled = !selectedFile;
        } else {
            // 查询模式：需要文件和账号
            uploadBtn.disabled = !(selectedFile && selectedAccount);
        }
    }

    // 清除选中的文件
    clearFile.addEventListener('click', () => {
        selectedFile = null;
        fileInput.value = '';
        uploadArea.style.display = 'block';
        fileInfo.style.display = 'none';
        updateUploadButtonState();
        hideMessage();
    });

    // 上传按钮点击
    uploadBtn.addEventListener('click', async () => {
        if (!selectedFile) return;
        if (uploadMode === 'query' && !selectedAccount) return;

        const region = regionSelect.value;

        uploadBtn.disabled = true;
        uploadBtn.classList.add('loading');
        progressContainer.style.display = 'block';
        progressFill.style.width = '0%';
        progressText.textContent = '正在上传...';
        hideMessage();

        const formData = new FormData();
        formData.append('file', selectedFile);
        formData.append('region', region);

        // 查询模式下才附带 game_id
        if (uploadMode === 'query' && selectedAccount) {
            formData.append('game_id', selectedAccount.game_id);
        }

        try {
            const xhr = new XMLHttpRequest();

            xhr.upload.addEventListener('progress', (e) => {
                if (e.lengthComputable) {
                    const percent = Math.round((e.loaded / e.total) * 100);
                    progressFill.style.width = percent + '%';
                    progressText.textContent = `上传中... ${percent}%`;
                }
            });

            xhr.addEventListener('load', () => {
                uploadBtn.classList.remove('loading');
                progressContainer.style.display = 'none';

                if (xhr.status === 200) {
                    try {
                        const response = JSON.parse(xhr.responseText);
                        if (uploadMode === 'direct') {
                            showMessage(`✓ 上传成功！数据已保存到 ${response.region_name}，用户ID: ${response.display_id}`, 'success');
                        } else {
                            showMessage(`✓ 上传成功！数据已保存到 ${response.region_name} 账号 ${response.display_id}`, 'success');
                        }
                        // 重置文件选择
                        selectedFile = null;
                        fileInput.value = '';
                        uploadArea.style.display = 'block';
                        fileInfo.style.display = 'none';
                    } catch (e) {
                        showMessage('上传成功，但响应解析失败', 'info');
                    }
                } else {
                    try {
                        const response = JSON.parse(xhr.responseText);
                        showMessage(`上传失败: ${response.error || '未知错误'}`, 'error');
                    } catch (e) {
                        showMessage(`上传失败: ${xhr.statusText || '服务器错误'}`, 'error');
                    }
                    uploadBtn.disabled = false;
                }
            });

            xhr.addEventListener('error', () => {
                uploadBtn.classList.remove('loading');
                progressContainer.style.display = 'none';
                showMessage('网络错误，请检查网络连接后重试', 'error');
                uploadBtn.disabled = false;
            });

            xhr.open('POST', new URL(`upload/${dataType}`, apiBase));
            xhr.send(formData);

        } catch (error) {
            uploadBtn.classList.remove('loading');
            progressContainer.style.display = 'none';
            showMessage(`上传出错: ${error.message}`, 'error');
            uploadBtn.disabled = false;
        }
    });

    // ==================== 工具函数 ====================

    function showMessage(text, type) {
        message.textContent = text;
        message.className = 'message ' + type;
    }

    function hideMessage() {
        message.className = 'message';
        message.textContent = '';
    }

    function formatFileSize(bytes) {
        if (bytes < 1024) return bytes + ' B';
        if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB';
        return (bytes / (1024 * 1024)).toFixed(1) + ' MB';
    }
});
