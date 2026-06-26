// EPUB Downloader

var EpubDownloader = {
    generateAndDownloadEpub: function(article, articleHtml, commentsHtml) {
        var writer = new EpubWriter();
        writer.generate(article, articleHtml, commentsHtml);
    },

    emailSelectedArticles: function(selectedArticles, to, callback) {
        try {
            var feedTitle = getText(document.getElementById("feed-title")) || "Articles";
            var _d = new Date(), _m = _d.getMonth() + 1, _day = _d.getDate();
            var _dateStr = _d.getFullYear() + "-" + (_m < 10 ? "0" + _m : _m) + "-" + (_day < 10 ? "0" + _day : _day);
            var filename = feedTitle.replace(/[^a-z0-9]/gi, "_") + "_articles_" + _dateStr + ".epub";
            var progressEl = document.getElementById("download-all-progress");
            removeClass(progressEl, "hidden");
            setText(progressEl, "Preparing articles for email: 0/" + selectedArticles.length);

            if (AppConfig.USE_BACKEND) {
                var urls = [];
                for (var i = 0; i < selectedArticles.length; i++) {
                    if (selectedArticles[i].link) { urls.push(selectedArticles[i].link); }
                }
                addClass(progressEl, "hidden");
                BackendClient.emailBulk(urls, to, "epub", feedTitle, callback);
                return;
            }

            var allArticlesHtml = [];
            var processedCount = 0;

            allArticlesHtml.push("<h1>" + escapeHtml(feedTitle) + "</h1>");
            allArticlesHtml.push("<hr/>");

            function processNextArticle(index) {
                if (index >= selectedArticles.length) {
                    addClass(progressEl, "hidden");
                    var fakeArticle = { title: feedTitle };
                    callback(new Error("Email requires backend mode to be enabled."));
                    return;
                }

                var article = selectedArticles[index];
                setText(progressEl, "Preparing articles for email: " + (index + 1) + "/" + selectedArticles.length + " - " + article.title);

                if (!article.link) {
                    allArticlesHtml.push("<h2>" + escapeHtml(article.title) + "</h2>");
                    if (article.pubDate) { allArticlesHtml.push("<p><em>" + escapeHtml(article.pubDate) + "</em></p>"); }
                    allArticlesHtml.push(article.content || article.description || "");
                    allArticlesHtml.push("<hr/>");
                    processedCount++;
                    processNextArticle(index + 1);
                    return;
                }

                ArticleFetcher.fetchFullArticleWithRetry(article.link, AppConfig.MAX_FETCH_RETRIES, function(error, extractedArticle) {
                    allArticlesHtml.push("<h2>" + escapeHtml(article.title) + "</h2>");
                    if (article.pubDate) { allArticlesHtml.push("<p><em>" + escapeHtml(article.pubDate) + "</em></p>"); }
                    allArticlesHtml.push("<p><a href=\"" + escapeHtml(article.link) + "\">" + escapeHtml(article.link) + "</a></p>");
                    if (error) {
                        allArticlesHtml.push("<p><strong>[Failed to fetch full article]</strong></p>");
                        allArticlesHtml.push(article.content || article.description || "");
                    } else {
                        allArticlesHtml.push(extractedArticle.content);
                        processedCount++;
                    }
                    allArticlesHtml.push("<hr/>");
                    processNextArticle(index + 1);
                });
            }

            processNextArticle(0);
        } catch (e) {
            addClass(document.getElementById("download-all-progress"), "hidden");
            callback(new Error("Email EPUB error: " + e.message));
        }
    },

    downloadSelectedArticles: function(selectedArticles) {
        try {
            var feedTitle = getText(document.getElementById("feed-title"));
            var _d = new Date(), _m = _d.getMonth() + 1, _day = _d.getDate();
            var _dateStr = _d.getFullYear() + "-" + (_m < 10 ? "0" + _m : _m) + "-" + (_day < 10 ? "0" + _day : _day);
            var filename = feedTitle.replace(/[^a-z0-9]/gi, "_") + "_selected_articles_" + _dateStr + ".epub";

            if (AppConfig.USE_BACKEND) {
                var urls = [];
                for (var i = 0; i < selectedArticles.length; i++) {
                    if (selectedArticles[i].link) {
                        urls.push(selectedArticles[i].link);
                    }
                }
                BackendClient.downloadEpubBulk(urls, feedTitle, feedTitle, filename);
                return;
            }

            var progressEl = document.getElementById("download-all-progress");
            removeClass(progressEl, "hidden");
            setText(progressEl, "Downloading articles: 0/" + selectedArticles.length);

            var allArticlesHtml = [];
            var processedCount = 0;

            allArticlesHtml.push("<h1>" + escapeHtml(feedTitle) + "</h1>");
            allArticlesHtml.push("<p>Downloaded: " + escapeHtml(new Date().toLocaleString()) + "</p>");
            allArticlesHtml.push("<p>Total articles: " + selectedArticles.length + "</p>");
            allArticlesHtml.push("<hr/>");

            function processNextArticle(index) {
                if (index >= selectedArticles.length) {
                    addClass(progressEl, "hidden");

                    var fakeArticle = { title: feedTitle };
                    var writer = new EpubWriter();
                    writer.generate(fakeArticle, allArticlesHtml.join("\n"), "");

                    alert("Downloaded " + processedCount + " articles successfully as EPUB!");
                    return;
                }

                var article = selectedArticles[index];
                setText(progressEl, "Downloading articles: " + (index + 1) + "/" + selectedArticles.length + " - " + article.title);

                if (!article.link) {
                    allArticlesHtml.push("<h2>" + escapeHtml(article.title) + "</h2>");
                    if (article.pubDate) {
                        allArticlesHtml.push("<p><em>" + escapeHtml(article.pubDate) + "</em></p>");
                    }
                    allArticlesHtml.push(article.content || article.description || "");
                    allArticlesHtml.push("<hr/>");
                    processedCount++;
                    processNextArticle(index + 1);
                    return;
                }

                ArticleFetcher.fetchFullArticleWithRetry(article.link, AppConfig.MAX_FETCH_RETRIES, function(error, extractedArticle) {
                    allArticlesHtml.push("<h2>" + escapeHtml(article.title) + "</h2>");
                    if (article.pubDate) {
                        allArticlesHtml.push("<p><em>" + escapeHtml(article.pubDate) + "</em></p>");
                    }
                    allArticlesHtml.push("<p><a href=\"" + escapeHtml(article.link) + "\">" + escapeHtml(article.link) + "</a></p>");

                    if (error) {
                        allArticlesHtml.push("<p><strong>[Failed to fetch full article]</strong></p>");
                        allArticlesHtml.push(article.content || article.description || "");
                    } else {
                        allArticlesHtml.push(extractedArticle.content);
                        processedCount++;
                    }

                    allArticlesHtml.push("<hr/>");
                    processNextArticle(index + 1);
                });
            }

            processNextArticle(0);
        } catch (e) {
            alert("downloadSelectedArticlesEpub error: " + e.message);
            addClass(document.getElementById("download-all-progress"), "hidden");
        }
    }
};
