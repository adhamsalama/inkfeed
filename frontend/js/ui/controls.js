// UI Controls for Font Size, Spacing, and Line Height

// Global functions (called from HTML onclick handlers)

function applyContentStyles() {
    document.body.style.fontFamily = AppState.currentFontFamily || "";
    var els = document.querySelectorAll(".article-content");
    for (var i = 0; i < els.length; i++) {
        if (AppState.currentFontSize > 0) { els[i].style.fontSize = AppState.currentFontSize + "px"; }
        els[i].style.letterSpacing = AppState.currentLetterSpacing + "px";
        els[i].style.wordSpacing = AppState.currentLetterSpacing * 2 + "px";
        els[i].style.lineHeight = AppState.currentLineHeight;
    }
}

var FONT_OPTIONS = [
    { label: "Default",      value: "" },
    { label: "Merriweather", value: "Merriweather, serif" },
    { label: "Lora",         value: "Lora, serif" },
    { label: "Source Serif", value: '"Source Serif 4", serif' },
    { label: "Open Sans",    value: '"Open Sans", sans-serif' },
    { label: "Roboto",       value: "Roboto, sans-serif" },
    { label: "Roboto Mono",  value: '"Roboto Mono", monospace' }
];

function setFontFamily(value) {
    AppState.currentFontFamily = value;
    applyContentStyles();
    if (value) {
        localStorage.setItem("fontFamily", value);
    } else {
        localStorage.removeItem("fontFamily");
    }
    updateFontPicker();
    PreferencesSync.pushPrefs();
}

function updateFontPicker() {
    var btns = document.querySelectorAll(".font-option-btn");
    for (var i = 0; i < btns.length; i++) {
        if (btns[i].getAttribute("data-font") === AppState.currentFontFamily) {
            addClass(btns[i], "btn-active");
        } else {
            removeClass(btns[i], "btn-active");
        }
    }
    var preview = document.getElementById("font-preview");
    if (preview) {
        preview.style.fontFamily = AppState.currentFontFamily || "inherit";
        preview.style.fontWeight = AppState.boldText ? "bold" : "";
    }
}

function adjustFontSize(delta) {
    if (AppState.currentFontSize === 0) {
        var el = document.querySelector(".article-content");
        AppState.currentFontSize = el
            ? parseFloat(window.getComputedStyle(el).fontSize) || 16
            : 16;
    }
    AppState.currentFontSize = Math.max(
        AppConfig.MIN_FONT_SIZE,
        Math.min(AppConfig.MAX_FONT_SIZE, AppState.currentFontSize + delta)
    );
    applyContentStyles();
    localStorage.setItem("fontSize", AppState.currentFontSize);
    PreferencesSync.pushPrefs();
}

function adjustSpacing(delta) {
    AppState.currentLetterSpacing = Math.max(
        AppConfig.MIN_LETTER_SPACING,
        Math.min(AppConfig.MAX_LETTER_SPACING, AppState.currentLetterSpacing + delta)
    );
    applyContentStyles();
    localStorage.setItem("letterSpacing", AppState.currentLetterSpacing);
    PreferencesSync.pushPrefs();
}

function adjustLineHeight(delta) {
    AppState.currentLineHeight = Math.max(
        AppConfig.MIN_LINE_HEIGHT,
        Math.min(AppConfig.MAX_LINE_HEIGHT, AppState.currentLineHeight + delta)
    );
    applyContentStyles();
    localStorage.setItem("lineHeight", AppState.currentLineHeight);
    PreferencesSync.pushPrefs();
}

function updateBackendToggleBtn() {
    var btn = document.getElementById("use-backend-btn");
    if (!btn) { return; }
    if (AppConfig.USE_BACKEND) {
        btn.textContent = "Backend: ON";
        btn.className = "";
    } else {
        btn.textContent = "Backend: OFF";
        btn.className = "secondary";
    }
}

function toggleUseBackend() {
    AppConfig.USE_BACKEND = !AppConfig.USE_BACKEND;
    localStorage.setItem("backendEnabled", AppConfig.USE_BACKEND ? "true" : "false");
    updateBackendToggleBtn();
    setEmailButtonVisible(AppConfig.USE_BACKEND);
    var favToggleBtn = document.getElementById("favorites-toggle-btn");
    if (favToggleBtn) { favToggleBtn.style.display = AppConfig.USE_BACKEND ? "" : "none"; }
}

function toggleEpubEmbedImages() {
    AppConfig.EPUB_EMBED_IMAGES = document.getElementById("epub-embed-images-checkbox").checked;
    localStorage.setItem("epubEmbedImages", AppConfig.EPUB_EMBED_IMAGES ? "true" : "false");
    PreferencesSync.pushPrefs();
}

function toggleMobiEmbedImages() {
    AppConfig.MOBI_EMBED_IMAGES = document.getElementById("mobi-embed-images-checkbox").checked;
    localStorage.setItem("mobiEmbedImages", AppConfig.MOBI_EMBED_IMAGES ? "true" : "false");
    PreferencesSync.pushPrefs();
}

function toggleGroupSortByDate() {
    AppConfig.GROUP_SORT_BY_DATE = document.getElementById("group-sort-by-date-checkbox").checked;
    localStorage.setItem("groupSortByDate", AppConfig.GROUP_SORT_BY_DATE ? "true" : "false");
    PreferencesSync.pushPrefs();
}

function applyBoldText() {
    if (AppState.boldText) {
        addClass(document.body, "bold-text");
    } else {
        removeClass(document.body, "bold-text");
    }
    var btn = document.getElementById("bold-toggle-btn");
    if (btn) {
        if (AppState.boldText) { addClass(btn, "btn-active"); } else { removeClass(btn, "btn-active"); }
    }
    updateFontPicker();
}

function toggleBoldText() {
    AppState.boldText = !AppState.boldText;
    applyBoldText();
    localStorage.setItem("boldText", AppState.boldText ? "true" : "false");
    PreferencesSync.pushPrefs();
}

function buildFontPicker() {
    var container = document.getElementById("font-picker");
    if (!container) { return; }
    container.innerHTML = "";
    for (var i = 0; i < FONT_OPTIONS.length; i++) {
        (function(opt) {
            var btn = document.createElement("button");
            btn.className = "secondary font-option-btn";
            btn.setAttribute("data-font", opt.value);
            btn.style.fontFamily = opt.value || "inherit";
            btn.style.marginRight = "4px";
            btn.style.marginBottom = "4px";
            setText(btn, opt.label);
            btn.onclick = function() { setFontFamily(opt.value); return false; };
            container.appendChild(btn);
        })(FONT_OPTIONS[i]);
    }
    updateFontPicker();
}

function openSettings(section) {
    document.getElementById("proxy-url-input").value = AppConfig.CORS_PROXY_URL;
    document.getElementById("email-to-input").value = localStorage.getItem("emailTo") || "";
    document.getElementById("epub-embed-images-checkbox").checked = AppConfig.EPUB_EMBED_IMAGES;
    document.getElementById("mobi-embed-images-checkbox").checked = AppConfig.MOBI_EMBED_IMAGES;
    document.getElementById("group-sort-by-date-checkbox").checked = AppConfig.GROUP_SORT_BY_DATE;
    document.getElementById("settings-modal").classList.remove("hidden");
    var boldBtn = document.getElementById("bold-toggle-btn");
    if (boldBtn) {
        if (AppState.boldText) { addClass(boldBtn, "btn-active"); } else { removeClass(boldBtn, "btn-active"); }
    }
    buildFontPicker();
    AccountView.render();
    if (section) {
        var sectionEl = document.getElementById("settings-" + section + "-section");
        if (sectionEl) {
            var firstInput = sectionEl.getElementsByTagName("input")[0];
            if (firstInput) { firstInput.focus(); }
        }
    }
}

function closeSettings() {
    document.getElementById("settings-modal").classList.add("hidden");
}

function saveProxyUrl() {
    var input = document.getElementById("proxy-url-input");
    var url = input.value.trim();
    if (url) {
        AppConfig.CORS_PROXY_URL = url;
        localStorage.setItem("corsProxyUrl", url);
        PreferencesSync.pushPrefs();
    }
    closeSettings();
}


function saveEmailAddress() {
    var input = document.getElementById("email-to-input");
    var email = input.value.replace(/^\s+|\s+$/g, "");
    if (email) {
        localStorage.setItem("emailTo", email);
        PreferencesSync.pushPrefs();
    }
    closeSettings();
}

function setEmailButtonVisible(visible) {
    var emailRow = document.getElementById("email-row");
    if (emailRow) {
        emailRow.style.display = visible ? "" : "none";
    }
    var emailAllRow = document.getElementById("email-all-row");
    if (emailAllRow) {
        emailAllRow.style.display = visible ? "" : "none";
    }
}

var _pendingEmailFormat = "epub";

function openEmailInput(format) {
    _pendingEmailFormat = format || "epub";
    var to = localStorage.getItem("emailTo") || "";
    if (!to) {
        openSettings("email");
        return;
    }
    if (AppState.currentArticleIndex < 0) {
        alert("No article selected");
        return;
    }
    var article = AppState.currentArticles[AppState.currentArticleIndex];
    if (!article.link) {
        alert("Article has no link");
        return;
    }
    var feedTitle = getText(document.getElementById("feed-title")) || "";
    var statusEl = document.getElementById("email-send-status");
    statusEl.textContent = "Sending...";
    BackendClient.sendEmail(article.link, to, format, feedTitle, article.comments || "", function(error) {
        if (error) {
            statusEl.textContent = "Error: " + error.message;
        } else {
            statusEl.textContent = "Sent!";
            setTimeout(function() { statusEl.textContent = ""; }, 3000);
        }
    });
}

function closeEmailInput() {
    closeSettings();
}

function applyDarkMode() {
    if (AppState.darkMode) {
        addClass(document.body, "dark-mode");
    } else {
        removeClass(document.body, "dark-mode");
    }
    var btn = document.getElementById("dark-mode-btn");
    if (btn) {
        if (AppState.darkMode) {
            setText(btn, "On");
            addClass(btn, "btn-active");
        } else {
            setText(btn, "Off");
            removeClass(btn, "btn-active");
        }
    }
}

function toggleDarkMode() {
    AppState.darkMode = !AppState.darkMode;
    applyDarkMode();
    localStorage.setItem("darkMode", AppState.darkMode ? "true" : "false");
    PreferencesSync.pushPrefs();
}

// Initialize on load.
(function() {
    updateBackendToggleBtn();
    setEmailButtonVisible(AppConfig.USE_BACKEND);
})();
