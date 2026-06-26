var ArticleSelectionManager = {
    startArticleSelection: function(downloadType) {
        try {
            if (!AppState.currentArticles || AppState.currentArticles.length === 0) {
                alert("No articles to download");
                return;
            }

            if (downloadType === "email-mobi" || downloadType === "email-epub") {
                var to = localStorage.getItem("emailTo") || "";
                if (!to) {
                    openSettings("email");
                    return;
                }
            }

            ArticleSelectionState.downloadType = downloadType;
            ArticleSelectionState.selectedIndices = new Set();
            ArticleSelectionState.inSelectionMode = true;

            document.getElementById("normal-nav-buttons").style.display = "none";
            document.getElementById("selection-nav-buttons").style.display = "flex";

            var actionBtn = document.getElementById("selection-action-btn");
            if (actionBtn) {
                actionBtn.textContent = (downloadType === "email-mobi" || downloadType === "email-epub")
                    ? "Email Selected" : "Download Selected";
            }

            var articleList = document.getElementById("article-list");
            var items = articleList.getElementsByClassName("article-item");
            for (var i = 0; i < items.length; i++) {
                this.addSelectionButtonsToArticle(items[i], i);
            }

            this.updateSelectionCounter();
        } catch (e) {
            alert("Error starting article selection: " + e.message);
        }
    },

    addSelectionButtonsToArticle: function(articleItem, index) {
        if (articleItem.querySelector(".article-selection-buttons")) return;

        var buttonsDiv = document.createElement("div");
        buttonsDiv.className = "article-selection-buttons";

        var toggleBtn = document.createElement("button");
        toggleBtn.textContent = "Select";
        toggleBtn.className = "secondary";
        toggleBtn.onclick = function(e) {
            e.stopPropagation();
            ArticleSelectionManager.toggleArticleSelection(index, articleItem, toggleBtn);
        };

        buttonsDiv.appendChild(toggleBtn);
        articleItem.appendChild(buttonsDiv);
    },

    toggleArticleSelection: function(index, articleItem, btn) {
        if (ArticleSelectionState.selectedIndices.has(index)) {
            ArticleSelectionState.selectedIndices.delete(index);
            removeClass(articleItem, "selected");
            btn.textContent = "Select";
            btn.className = "secondary";
        } else {
            ArticleSelectionState.selectedIndices.add(index);
            addClass(articleItem, "selected");
            btn.textContent = "Selected";
            btn.className = "primary";
        }
        this.updateSelectionCounter();
    },

    updateSelectionCounter: function() {
        setText(document.getElementById("selection-count"), ArticleSelectionState.selectedIndices.size);
    },

    selectAllArticles: function() {
        var articleList = document.getElementById("article-list");
        var items = articleList.getElementsByClassName("article-item");
        for (var i = 0; i < items.length; i++) {
            ArticleSelectionState.selectedIndices.add(i);
            addClass(items[i], "selected");
            var btn = items[i].querySelector(".article-selection-buttons button");
            if (btn) { btn.textContent = "Selected"; btn.className = "primary"; }
        }
        this.updateSelectionCounter();
    },

    cancelArticleSelection: function() {
        var articleList = document.getElementById("article-list");
        var items = articleList.getElementsByClassName("article-item");
        for (var i = 0; i < items.length; i++) {
            var buttons = items[i].querySelector(".article-selection-buttons");
            if (buttons) { buttons.remove(); }
            removeClass(items[i], "selected");
        }

        ArticleSelectionState.downloadType = null;
        ArticleSelectionState.selectedIndices = new Set();
        ArticleSelectionState.inSelectionMode = false;

        document.getElementById("normal-nav-buttons").style.display = "";
        document.getElementById("selection-nav-buttons").style.display = "none";
    },

    downloadSelectedArticles: function() {
        if (ArticleSelectionState.selectedIndices.size === 0) {
            alert("No articles selected");
            return;
        }

        var selectedArticles = [];
        ArticleSelectionState.selectedIndices.forEach(function(index) {
            selectedArticles.push(AppState.currentArticles[index]);
        });

        var downloadType = ArticleSelectionState.downloadType;
        this.cancelArticleSelection();

        if (downloadType === "text") {
            TextDownloader.downloadSelectedArticles(selectedArticles);
        } else if (downloadType === "mobi") {
            MobiDownloader.downloadSelectedArticles(selectedArticles);
        } else if (downloadType === "epub") {
            EpubDownloader.downloadSelectedArticles(selectedArticles);
        } else if (downloadType === "email-mobi" || downloadType === "email-epub") {
            var emailTo = localStorage.getItem("emailTo") || "";
            var statusEl = document.getElementById("email-all-status");
            var downloader = downloadType === "email-mobi" ? MobiDownloader : EpubDownloader;
            if (statusEl) { statusEl.textContent = "Sending..."; }
            downloader.emailSelectedArticles(selectedArticles, emailTo, function(error) {
                if (statusEl) {
                    statusEl.textContent = error ? "Error: " + error.message : "Sent!";
                    if (!error) { setTimeout(function() { statusEl.textContent = ""; }, 3000); }
                }
            });
        }
    }
};

function startArticleSelection(downloadType) { ArticleSelectionManager.startArticleSelection(downloadType); }
function downloadSelectedArticles() { ArticleSelectionManager.downloadSelectedArticles(); }
function cancelArticleSelection() { ArticleSelectionManager.cancelArticleSelection(); }
function selectAllArticles() { ArticleSelectionManager.selectAllArticles(); }
