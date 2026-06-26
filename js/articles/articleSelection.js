// Article Selection Manager

var ArticleSelectionManager = {
    // Start article selection process
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

            // Initialize selection state
            ArticleSelectionState.downloadType = downloadType;
            ArticleSelectionState.selectedIndices = new Set();
            ArticleSelectionState.inSelectionMode = true;

            // Show selection buttons, hide normal buttons
            document.getElementById("normal-nav-buttons").style.display = "none";
            document.getElementById("selection-nav-buttons").style.display = "flex";

            var actionBtn = document.getElementById("selection-action-btn");
            if (downloadType === "email-mobi" || downloadType === "email-epub") {
                if (actionBtn) { actionBtn.textContent = "Email Selected"; }
            } else {
                if (actionBtn) { actionBtn.textContent = "Download Selected"; }
            }

            // Add selection buttons to each article (skip feed separators)
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

    // Add selection buttons to an article item
    addSelectionButtonsToArticle: function(articleItem, index) {
        // Check if buttons already exist
        var existing = articleItem.querySelector(".article-selection-buttons");
        if (existing) return;

        var buttonsDiv = document.createElement("div");
        buttonsDiv.className = "article-selection-buttons";

        var chooseBtn = document.createElement("button");
        chooseBtn.textContent = "Choose";
        chooseBtn.className = "primary";
        chooseBtn.onclick = function(e) {
            e.stopPropagation();
            ArticleSelectionManager.toggleArticleSelection(index, articleItem, true);
        };

        var skipBtn = document.createElement("button");
        skipBtn.textContent = "Skip";
        skipBtn.className = "secondary";
        skipBtn.onclick = function(e) {
            e.stopPropagation();
            ArticleSelectionManager.toggleArticleSelection(index, articleItem, false);
        };

        buttonsDiv.appendChild(chooseBtn);
        buttonsDiv.appendChild(skipBtn);
        articleItem.appendChild(buttonsDiv);
    },

    // Toggle article selection
    toggleArticleSelection: function(index, articleItem, select) {
        if (select) {
            ArticleSelectionState.selectedIndices.add(index);
            addClass(articleItem, "selected");
            removeClass(articleItem, "skipped");
        } else {
            ArticleSelectionState.selectedIndices.delete(index);
            removeClass(articleItem, "selected");
            addClass(articleItem, "skipped");
        }
        this.updateSelectionCounter();
    },

    // Update selection counter
    updateSelectionCounter: function() {
        setText(document.getElementById("selection-count"), ArticleSelectionState.selectedIndices.size);
    },

    // Cancel article selection
    cancelArticleSelection: function() {
        // Remove selection buttons from all articles
        var articleList = document.getElementById("article-list");
        var items = articleList.getElementsByClassName("article-item");

        for (var i = 0; i < items.length; i++) {
            var buttons = items[i].querySelector(".article-selection-buttons");
            if (buttons) {
                buttons.remove();
            }
            removeClass(items[i], "selected");
            removeClass(items[i], "skipped");
        }

        // Reset state
        ArticleSelectionState.downloadType = null;
        ArticleSelectionState.selectedIndices = new Set();
        ArticleSelectionState.inSelectionMode = false;

        // Show normal buttons, hide selection buttons
        document.getElementById("normal-nav-buttons").style.display = "";
        document.getElementById("selection-nav-buttons").style.display = "none";
    },

    // Download selected articles
    downloadSelectedArticles: function() {
        if (ArticleSelectionState.selectedIndices.size === 0) {
            alert("No articles selected");
            return;
        }

        // Get selected articles
        var selectedArticles = [];
        ArticleSelectionState.selectedIndices.forEach(function(index) {
            selectedArticles.push(AppState.currentArticles[index]);
        });

        // Clean up selection UI
        var downloadType = ArticleSelectionState.downloadType;
        this.cancelArticleSelection();

        // Start download based on type
        if (downloadType === "text") {
            TextDownloader.downloadSelectedArticles(selectedArticles);
        } else if (downloadType === "mobi") {
            MobiDownloader.downloadSelectedArticles(selectedArticles);
        } else if (downloadType === "epub") {
            EpubDownloader.downloadSelectedArticles(selectedArticles);
        } else if (downloadType === "email-mobi") {
            var emailTo = localStorage.getItem("emailTo") || "";
            var statusEl = document.getElementById("email-all-status");
            if (statusEl) { statusEl.textContent = "Sending..."; }
            MobiDownloader.emailSelectedArticles(selectedArticles, emailTo, function(error) {
                if (statusEl) {
                    statusEl.textContent = error ? "Error: " + error.message : "Sent!";
                    if (!error) { setTimeout(function() { statusEl.textContent = ""; }, 3000); }
                }
            });
        } else if (downloadType === "email-epub") {
            var emailToEpub = localStorage.getItem("emailTo") || "";
            var statusElEpub = document.getElementById("email-all-status");
            if (statusElEpub) { statusElEpub.textContent = "Sending..."; }
            EpubDownloader.emailSelectedArticles(selectedArticles, emailToEpub, function(error) {
                if (statusElEpub) {
                    statusElEpub.textContent = error ? "Error: " + error.message : "Sent!";
                    if (!error) { setTimeout(function() { statusElEpub.textContent = ""; }, 3000); }
                }
            });
        }
    }
};

// Global proxy functions for HTML onclick handlers
function startArticleSelection(downloadType) {
    ArticleSelectionManager.startArticleSelection(downloadType);
}

function downloadSelectedArticles() {
    ArticleSelectionManager.downloadSelectedArticles();
}

function cancelArticleSelection() {
    ArticleSelectionManager.cancelArticleSelection();
}
