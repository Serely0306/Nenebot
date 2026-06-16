document.addEventListener('DOMContentLoaded', () => {
    const message = document.getElementById('helpMessage');
    const tabButtons = Array.from(document.querySelectorAll('[data-tab-target]'));
    const tabPanels = Array.from(document.querySelectorAll('[data-tab-panel]'));
    const iosModuleContent = document.getElementById('iosModuleContent');
    const androidGuideContent = document.getElementById('androidGuideContent');
    const copyIosModuleBtn = document.getElementById('copyIosModuleBtn');

    let iosModuleLoaded = false;
    let androidGuideLoaded = false;

    function showMessage(text, type) {
        message.textContent = text;
        message.className = `message ${type}`;
    }

    function hideMessage() {
        message.textContent = '';
        message.className = 'message';
    }

    async function fetchText(relativePath) {
        const response = await fetch(relativePath);
        const text = await response.text();
        if (!response.ok) {
            throw new Error(text || `HTTP ${response.status}`);
        }
        return text;
    }

    async function ensureIosModuleLoaded() {
        if (iosModuleLoaded) {
            return;
        }
        iosModuleContent.textContent = '模块内容加载中...';
        try {
            iosModuleContent.textContent = await fetchText('help/ios');
            iosModuleLoaded = true;
        } catch (error) {
            iosModuleContent.textContent = '模块加载失败，请稍后重试。';
            showMessage(`加载 iOS 模块失败：${error.message}`, 'error');
        }
    }

    async function ensureAndroidGuideLoaded() {
        if (androidGuideLoaded) {
            return;
        }
        androidGuideContent.textContent = 'Android 使用方式加载中...';
        try {
            androidGuideContent.textContent = await fetchText('help/android');
            androidGuideLoaded = true;
        } catch (error) {
            androidGuideContent.textContent = 'Android 使用方式加载失败，请稍后重试。';
            showMessage(`加载 Android 使用方式失败：${error.message}`, 'error');
        }
    }

    async function activateTab(tabName) {
        hideMessage();
        tabButtons.forEach((button) => {
            button.classList.toggle('active', button.dataset.tabTarget === tabName);
        });
        tabPanels.forEach((panel) => {
            panel.classList.toggle('active', panel.dataset.tabPanel === tabName);
        });

        if (tabName === 'ios-module') {
            await ensureIosModuleLoaded();
        }
        if (tabName === 'android-guide') {
            await ensureAndroidGuideLoaded();
        }
        window.location.hash = tabName;
    }

    tabButtons.forEach((button) => {
        button.addEventListener('click', () => {
            activateTab(button.dataset.tabTarget);
        });
    });

    if (copyIosModuleBtn) {
        copyIosModuleBtn.addEventListener('click', async () => {
            try {
                await ensureIosModuleLoaded();
                await navigator.clipboard.writeText(iosModuleContent.textContent);
                showMessage('iOS 模块已复制到剪贴板。', 'success');
            } catch (error) {
                showMessage(`复制失败：${error.message}`, 'error');
            }
        });
    }

    const initialTab = window.location.hash ? window.location.hash.slice(1) : 'ios-module';
    const knownTabs = new Set(['ios-module', 'android-guide']);
    activateTab(knownTabs.has(initialTab) ? initialTab : 'ios-module');
});
