// MOBI Downloader

var MobiDownloader = {
    // Detect image format from magic bytes. Returns 'jpeg', 'png', 'gif', or 'other'.
    _detectFormat: function(bytes) {
        if (bytes.length < 4) { return 'other'; }
        if (bytes[0] === 0xFF && bytes[1] === 0xD8) { return 'jpeg'; }
        if (bytes[0] === 0x89 && bytes[1] === 0x50 && bytes[2] === 0x4E && bytes[3] === 0x47) { return 'png'; }
        if (bytes[0] === 0x47 && bytes[1] === 0x49 && bytes[2] === 0x46) { return 'gif'; }
        return 'other';
    },

    // Convert raw image bytes to JPEG using Canvas. Falls back to original bytes on error.
    _convertToJpeg: function(bytes, callback) {
        try {
            var uint8 = new Uint8Array(bytes);
            var blob = new Blob([uint8]);
            var url = URL.createObjectURL(blob);
            var img = new Image();
            img.onload = function() {
                try {
                    var canvas = document.createElement('canvas');
                    canvas.width = img.naturalWidth || 1;
                    canvas.height = img.naturalHeight || 1;
                    canvas.getContext('2d').drawImage(img, 0, 0);
                    URL.revokeObjectURL(url);
                    var dataUrl = canvas.toDataURL('image/jpeg', 0.85);
                    var base64 = dataUrl.split(',')[1];
                    var binary = atob(base64);
                    var result = [];
                    for (var i = 0; i < binary.length; i++) {
                        result.push(binary.charCodeAt(i));
                    }
                    callback(result);
                } catch (e) {
                    URL.revokeObjectURL(url);
                    callback(bytes);
                }
            };
            img.onerror = function() { URL.revokeObjectURL(url); callback(bytes); };
            img.src = url;
        } catch (e) {
            callback(bytes);
        }
    },

    // Fetch a single image URL via CORS proxy, return Kindle-compatible byte array via callback.
    _fetchImageBytes: function(url, callback) {
        var xhr = new XMLHttpRequest();
        xhr.open("GET", AppConfig.CORS_PROXY_URL + encodeURIComponent(url), true);
        xhr.responseType = "arraybuffer";
        xhr.onload = function() {
            if (xhr.status >= 200 && xhr.status < 300 && xhr.response) {
                var bytes = [];
                if (typeof Uint8Array !== "undefined") {
                    var uint8 = new Uint8Array(xhr.response);
                    for (var i = 0; i < uint8.length; i++) {
                        bytes.push(uint8[i]);
                    }
                }
                var fmt = MobiDownloader._detectFormat(bytes);
                if (fmt === 'jpeg' || fmt === 'png' || fmt === 'gif') {
                    callback(null, bytes);
                } else {
                    MobiDownloader._convertToJpeg(bytes, function(converted) {
                        callback(null, converted);
                    });
                }
            } else {
                callback(new Error("HTTP " + xhr.status));
            }
        };
        xhr.onerror = function() { callback(new Error("Network error")); };
        xhr.send();
    },

    // Replace <img src="URL"> with <img recindex="N"> and collect image byte arrays.
    // Returns via callback(null, { html, imageRecords }) or skips broken images.
    _embedImages: function(html, callback) {
        // Collect unique http/https image URLs in order of first appearance
        var urls = [];
        var seen = {};
        var imgTagRegex = /<img([^>]*)>/gi;
        var match;
        while ((match = imgTagRegex.exec(html)) !== null) {
            var srcMatch = /src=["']([^"']+)["']/i.exec(match[1]);
            if (srcMatch) {
                var url = srcMatch[1];
                if (!seen[url] && (url.indexOf("http://") === 0 || url.indexOf("https://") === 0)) {
                    urls.push(url);
                    seen[url] = true;
                }
            }
        }

        if (urls.length === 0) {
            callback(null, { html: html, imageRecords: [] });
            return;
        }

        var imageRecords = [];
        var urlToIndex = {};

        function fetchNext(i) {
            if (i >= urls.length) {
                // Replace img tags: keep only alt + recindex, strip src/srcset/sizes/loading/etc.
                var newHtml = html.replace(/<img([^>]*)>/gi, function(imgTag, attrs) {
                    var srcMatch = /src=["']([^"']+)["']/i.exec(attrs);
                    if (!srcMatch || urlToIndex[srcMatch[1]] === undefined) { return imgTag; }
                    var altMatch = /alt=["']([^"']*)["']/i.exec(attrs);
                    var altAttr = altMatch ? " alt=\"" + altMatch[1] + "\"" : "";
                    return "<img" + altAttr + " recindex=\"" + urlToIndex[srcMatch[1]] + "\">";
                });
                callback(null, { html: newHtml, imageRecords: imageRecords });
                return;
            }

            MobiDownloader._fetchImageBytes(urls[i], function(err, bytes) {
                if (!err && bytes.length > 0) {
                    imageRecords.push(bytes);
                    urlToIndex[urls[i]] = imageRecords.length; // 1-based index
                }
                fetchNext(i + 1);
            });
        }

        fetchNext(0);
    },

    // Generate and download MOBI for a single article
    generateAndDownloadMobi: function(article, articleHtml, commentsHtml) {
        var htmlContent =
            "<html><body><h1>" +
            escapeHtml(article.title) +
            "</h1>" +
            articleHtml;

        if (commentsHtml) {
            htmlContent += "<hr/><h2>Comments</h2>" + commentsHtml;
        }

        htmlContent += "</body></html>";

        var title = article.title || "Article";
        var filename = title.replace(/[^a-z0-9]/gi, "_") + ".mobi";
        var feedTitle = getText(document.getElementById("feed-title")) || "RSS Reader";

        if (AppConfig.USE_BACKEND) {
            BackendClient.downloadMobi(article.link, title, feedTitle, article.comments || "", filename);
            return;
        }

        function writeBook(html, imageRecords) {
            var book = new MobiBook(title, feedTitle);
            book.setHtmlContent(html);
            var writer = new MobiWriter();
            var result = writer.write(book, filename, false, imageRecords);
            if (!result.success) {
                alert("Error generating MOBI: " + result.error);
            }
        }

        if (AppConfig.MOBI_EMBED_IMAGES) {
            MobiDownloader._embedImages(htmlContent, function(err, result) {
                writeBook(result.html, result.imageRecords);
            });
        } else {
            writeBook(htmlContent, []);
        }
    },

    emailSelectedArticles: function(selectedArticles, to, callback) {
        try {
            var feedTitle = getText(document.getElementById("feed-title")) || "Articles";
            var _d = new Date(), _m = _d.getMonth() + 1, _day = _d.getDate();
            var _dateStr = _d.getFullYear() + "-" + (_m < 10 ? "0" + _m : _m) + "-" + (_day < 10 ? "0" + _day : _day);
            var filename = feedTitle.replace(/[^a-z0-9]/gi, "_") + "_articles_" + _dateStr + ".mobi";
            var progressEl = document.getElementById("download-all-progress");
            removeClass(progressEl, "hidden");
            setText(progressEl, "Preparing articles for email: 0/" + selectedArticles.length);

            if (AppConfig.USE_BACKEND) {
                var urls = [];
                for (var i = 0; i < selectedArticles.length; i++) {
                    if (selectedArticles[i].link) { urls.push(selectedArticles[i].link); }
                }
                addClass(progressEl, "hidden");
                BackendClient.emailBulk(urls, to, "mobi", feedTitle, callback);
                return;
            }

            var allArticlesHtml = [];
            var processedCount = 0;

            allArticlesHtml.push("<html><body>");
            allArticlesHtml.push("<h1>" + escapeHtml(feedTitle) + "</h1>");
            allArticlesHtml.push("<hr/>");

            function processNextArticle(index) {
                if (index >= selectedArticles.length) {
                    addClass(progressEl, "hidden");
                    var htmlContent = allArticlesHtml.join("\n") + "</body></html>";
                    var book = new MobiBook(feedTitle, feedTitle);
                    book.setHtmlContent(htmlContent);
                    var writer = new MobiWriter();
                    var result = writer.write(book, filename, true);
                    if (!result.success) {
                        callback(new Error(result.error || "MOBI generation failed"));
                        return;
                    }
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
            callback(new Error("Email MOBI error: " + e.message));
        }
    },

    // Download selected articles as MOBI (unified, replaces duplicate "All" version)
    downloadSelectedArticles: function(selectedArticles) {
        try {
            var feedTitle = getText(document.getElementById("feed-title"));
            var _d = new Date(), _m = _d.getMonth() + 1, _day = _d.getDate();
            var _dateStr = _d.getFullYear() + "-" + (_m < 10 ? "0" + _m : _m) + "-" + (_day < 10 ? "0" + _day : _day);
            var filename = feedTitle.replace(/[^a-z0-9]/gi, "_") + "_selected_articles_" + _dateStr + ".mobi";

            if (AppConfig.USE_BACKEND) {
                var urls = [];
                for (var i = 0; i < selectedArticles.length; i++) {
                    if (selectedArticles[i].link) {
                        urls.push(selectedArticles[i].link);
                    }
                }
                BackendClient.downloadMobiBulk(urls, feedTitle, feedTitle, filename);
                return;
            }

            var progressEl = document.getElementById("download-all-progress");
            removeClass(progressEl, "hidden");
            setText(progressEl, "Downloading articles: 0/" + selectedArticles.length);

            var allArticlesHtml = [];
            var processedCount = 0;

            allArticlesHtml.push("<html><body>");
            allArticlesHtml.push("<h1>" + escapeHtml(feedTitle) + "</h1>");
            allArticlesHtml.push("<p>Downloaded: " + escapeHtml(new Date().toLocaleString()) + "</p>");
            allArticlesHtml.push("<p>Total articles: " + selectedArticles.length + "</p>");
            allArticlesHtml.push("<hr/>");

            function processNextArticle(index) {
                if (index >= selectedArticles.length) {
                    addClass(progressEl, "hidden");

                    allArticlesHtml.push("</body></html>");
                    var htmlContent = allArticlesHtml.join("\n");

                    var book = new MobiBook(feedTitle, feedTitle);
                    book.setHtmlContent(htmlContent);
                    var writer = new MobiWriter();
                    writer.write(book, filename);

                    alert("Downloaded " + processedCount + " articles successfully as MOBI!");
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
                        allArticlesHtml.push("<p><strong>[Failed to fetch full article after " + AppConfig.MAX_FETCH_RETRIES + " attempts]</strong></p>");
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
            alert("downloadSelectedArticlesMobi error: " + e.message);
            addClass(document.getElementById("download-all-progress"), "hidden");
        }
    }
};
