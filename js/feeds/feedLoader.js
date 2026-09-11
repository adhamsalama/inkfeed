// RSS/Atom Feed Loader

var archiveOffset = 0;
var archivePageSize = 50;
var archiveArticles = [];

function resetArchiveState() {
    archiveOffset = 0;
    archiveArticles = [];
    document.getElementById("archive-article-list").innerHTML = "";
    addClass(document.getElementById("archive-load-more"), "hidden");
    var btn = document.getElementById("show-archive-btn");
    addClass(btn, "hidden");
    btn.onclick = showFeedArchive;
    setText(btn, "Archived");
    addClass(document.getElementById("archive-loading"), "hidden");
    addClass(document.getElementById("feed-archive-section"), "hidden");
    removeClass(document.getElementById("article-list"), "hidden");
}

function showLiveFeed() {
    addClass(document.getElementById("feed-archive-section"), "hidden");
    removeClass(document.getElementById("article-list"), "hidden");
    var btn = document.getElementById("show-archive-btn");
    btn.onclick = showFeedArchive;
    setText(btn, "Archived");
}

function showFeedArchive() {
    if (!AuthState.isLoggedIn()) return;
    archiveOffset = 0;
    archiveArticles = [];
    document.getElementById("archive-article-list").innerHTML = "";
    addClass(document.getElementById("archive-load-more"), "hidden");
    // Hide live feed, show archive section
    addClass(document.getElementById("article-list"), "hidden");
    removeClass(document.getElementById("feed-archive-section"), "hidden");
    var btn = document.getElementById("show-archive-btn");
    btn.onclick = showLiveFeed;
    setText(btn, "Live");
    loadArchivePage();
}

function loadMoreArchive() {
    if (!AppConfig.USE_BACKEND) return;
    loadArchivePage();
}

function loadArchivePage() {
    var feedUrl = AppState.lastLoadedFeedUrl;
    if (!feedUrl) return;

    removeClass(document.getElementById("archive-loading"), "hidden");
    addClass(document.getElementById("archive-load-more"), "hidden");

    BackendClient.fetchFeedArchive(feedUrl, archivePageSize, archiveOffset, function(error, data) {
        addClass(document.getElementById("archive-loading"), "hidden");
        if (error || !data || !data.articles) return;

        var newArticles = data.articles;
        for (var i = 0; i < newArticles.length; i++) {
            newArticles[i].index = archiveArticles.length;
            archiveArticles.push(newArticles[i]);
        }

        archiveOffset += newArticles.length;

        renderArchiveArticles(newArticles);

        if (data.hasMore) {
            removeClass(document.getElementById("archive-load-more"), "hidden");
        }
    });
}

function renderArchiveArticles(articles) {
    var list = document.getElementById("archive-article-list");
    var fragment = document.createDocumentFragment();

    for (var i = 0; i < articles.length; i++) {
        var article = articles[i];
        var li = document.createElement("li");
        li.className = "article-item" + (article.link && AppState.readArticles.has(article.link) ? " article-read" : "");
        li.id = "archive-article-" + article.index;
        (function(idx) {
            li.onclick = function() {
                AppState.currentArticles = [archiveArticles[idx]];
                AppState.feedScrollBackId = "archive-article-" + idx;
                ArticleViewer.openArticle(0);
                return false;
            };
        })(article.index);

        var titleRow = document.createElement("div");
        titleRow.className = "article-title-row";

        var title = document.createElement("span");
        title.className = "article-title";
        setText(title, article.title);
        titleRow.appendChild(title);

        if (article.pubDate) {
            var date = new Date(article.pubDate);
            var dateText = isNaN(date.getTime())
                ? article.pubDate
                : (date.getDate() < 10 ? "0" + date.getDate() : "" + date.getDate()) + "-" +
                  (date.getMonth() + 1 < 10 ? "0" + (date.getMonth() + 1) : "" + (date.getMonth() + 1)) + "-" +
                  ("" + date.getFullYear()).slice(2);
            var meta = document.createElement("span");
            meta.className = "article-meta";
            setText(meta, dateText);
            titleRow.appendChild(meta);
        }

        var desc = document.createElement("div");
        desc.className = "article-description";
        var tempDiv = document.createElement("div");
        tempDiv.innerHTML = article.description;
        var plainText = getText(tempDiv);
        var truncated = plainText.substring(0, 200);
        if (plainText.length > 200) truncated += "...";
        setText(desc, truncated);

        li.appendChild(titleRow);
        if (getText(desc)) li.appendChild(desc);
        fragment.appendChild(li);
    }

    list.appendChild(fragment);
}

function parseFeedXml(xmlText) {
    var xml;
    if (window.DOMParser) {
        var parser = new DOMParser();
        xml = parser.parseFromString(xmlText, "text/xml");
    } else {
        xml = new ActiveXObject("Microsoft.XMLDOM");
        xml.async = false;
        xml.loadXML(xmlText);
    }

    var articles = [];
    var feedTitle = "Feed";
    var i, item, entry, titleEl, linkEl, descEl, contentEl, pubDateEl;

    // Try RSS 2.0
    var channels = xml.getElementsByTagName("channel");
    if (channels.length > 0) {
        var channel = channels[0];
        var channelTitle = getFirstByTag(channel, "title");
        feedTitle = channelTitle ? getText(channelTitle) : "RSS Feed";

        var items = xml.getElementsByTagName("item");
        for (i = 0; i < items.length; i++) {
            item = items[i];
            titleEl = getFirstByTag(item, "title");
            linkEl = getFirstByTag(item, "link");
            descEl = getFirstByTag(item, "description");
            contentEl = getFirstByTag(item, "encoded");
            pubDateEl = getFirstByTag(item, "pubDate");
            var commentsEl = getFirstByTag(item, "comments");

            articles.push({
                index: i,
                title: titleEl ? getText(titleEl) : "Untitled",
                link: linkEl ? getText(linkEl) : "",
                comments: commentsEl ? getText(commentsEl) : "",
                description: descEl ? getText(descEl) : "",
                content: contentEl ? getText(contentEl) : "",
                pubDate: pubDateEl ? getText(pubDateEl) : ""
            });
        }
    }

    // Try Atom
    if (articles.length === 0) {
        var atomFeeds = xml.getElementsByTagName("feed");
        if (atomFeeds.length > 0) {
            var feedTitleEl = getFirstByTag(atomFeeds[0], "title");
            feedTitle = feedTitleEl ? getText(feedTitleEl) : "Atom Feed";
        }

        var entries = xml.getElementsByTagName("entry");
        for (i = 0; i < entries.length; i++) {
            entry = entries[i];
            titleEl = getFirstByTag(entry, "title");

            var linkHref = "";
            var links = entry.getElementsByTagName("link");
            for (var j = 0; j < links.length; j++) {
                var rel = links[j].getAttribute("rel");
                if (!rel || rel === "alternate") {
                    linkHref = links[j].getAttribute("href") || "";
                    break;
                }
            }

            descEl = getFirstByTag(entry, "summary");
            contentEl = getFirstByTag(entry, "content");
            pubDateEl = getFirstByTag(entry, "published");
            if (!pubDateEl) pubDateEl = getFirstByTag(entry, "updated");

            var commentsUrl = "";
            if (linkHref && linkHref.indexOf("reddit.com") >= 0) {
                commentsUrl = linkHref + "/.json";
            }

            articles.push({
                index: i,
                title: titleEl ? getText(titleEl) : "Untitled",
                link: linkHref,
                comments: commentsUrl,
                description: descEl ? getText(descEl) : "",
                content: contentEl ? getText(contentEl) : "",
                pubDate: pubDateEl ? getText(pubDateEl) : ""
            });
        }
    }

    return { title: feedTitle, articles: articles };
}

// Global function for HTML onclick handler
function loadFeed() {
    try {
        var urlInput = document.getElementById("feed-url");
        var url = urlInput.value;
        // Trim manually for ES3
        url = url.replace(/^\s+|\s+$/g, "");

        if (!url) {
            ViewManager.showError("input-error", "Please enter a feed URL");
            return;
        }

        // Save feed URL to browser URL for persistence on refresh
        if (window.history && window.history.replaceState) {
            window.history.replaceState(
                null,
                "",
                "?feed=" + encodeURIComponent(url)
            );
        } else {
            // Fallback for older browsers - use location.hash to avoid page reload
            window.location.hash = "feed=" + encodeURIComponent(url);
        }

        ViewManager.hideError("input-error");
        removeClass(document.getElementById("feed-loading"), "hidden");
        document.getElementById("article-list").innerHTML = "";
        resetArchiveState();
        ViewManager.showFeedView();

        if (AppConfig.USE_BACKEND) {
            BackendClient.fetchFeed(url, function(error, data) {
                addClass(document.getElementById("feed-loading"), "hidden");
                if (error) {
                    ViewManager.showInputView();
                    ViewManager.showError("input-error", "Error loading feed: " + error.message);
                    return;
                }
                if (!data.articles || data.articles.length === 0) {
                    ViewManager.showInputView();
                    ViewManager.showError("input-error", "Error loading feed: No articles found in feed");
                    return;
                }
                AppState.currentArticles = data.articles;
                AppState.lastLoadedFeedUrl = url;
                AppState.lastLoadedFeedTitle = data.title;
                setText(document.getElementById("feed-title"), data.title);
                document.title = data.title;
                SavedFeedsManager.updateFeedTitle(url, data.title);
                FeedRenderer.renderArticleList(data.articles);
                if (AuthState.isLoggedIn() && SavedFeedsManager.isFeedArchiveEnabled(url)) {
                    removeClass(document.getElementById("show-archive-btn"), "hidden");
                }
            });
            return;
        }

        fetchUrl(url, function (error, xmlText) {
            if (error) {
                ViewManager.showInputView();
                ViewManager.showError("input-error", "Error loading feed: " + error.message);
                addClass(document.getElementById("feed-loading"), "hidden");
                return;
            }

            try {
                var parsed = parseFeedXml(xmlText);
                if (parsed.articles.length === 0) {
                    throw new Error("No articles found in feed");
                }
                AppState.currentArticles = parsed.articles;
                AppState.lastLoadedFeedUrl = url;
                AppState.lastLoadedFeedTitle = parsed.title;
                setText(document.getElementById("feed-title"), parsed.title);
                document.title = parsed.title;
                SavedFeedsManager.updateFeedTitle(url, parsed.title);
                FeedRenderer.renderArticleList(parsed.articles);
                if (AuthState.isLoggedIn() && SavedFeedsManager.isFeedArchiveEnabled(url)) {
                    removeClass(document.getElementById("show-archive-btn"), "hidden");
                }
            } catch (e) {
                ViewManager.showInputView();
                ViewManager.showError("input-error", "Error loading feed: " + e.message);
            }

            addClass(document.getElementById("feed-loading"), "hidden");
        });
    } catch (e) {
        alert("loadFeed error: " + e.message);
    }
}

// Milliseconds since epoch for an article's pubDate; 0 when missing or unparseable.
function articleTimestamp(article) {
    if (!article.pubDate) { return 0; }
    var t = new Date(article.pubDate).getTime();
    return isNaN(t) ? 0 : t;
}

function loadCategoryFeeds(categoryFeeds, categoryName) {
    var total = categoryFeeds.length;
    var completed = 0;
    var results = [];  // indexed by feed position to preserve order
    var progress = document.getElementById("render-progress");

    for (var n = 0; n < total; n++) {
        results.push(null);
    }

    ViewManager.hideError("input-error");
    removeClass(document.getElementById("feed-loading"), "hidden");
    document.getElementById("article-list").innerHTML = "";
    ViewManager.showFeedView();
    setText(document.getElementById("feed-title"), categoryName);
    document.title = categoryName;
    setText(progress, "");

    function onFeedDone(index, articles) {
        completed++;
        results[index] = articles || [];
        setText(progress, "");

        if (completed === total) {
            addClass(document.getElementById("feed-loading"), "hidden");
            var allArticles = [];
            for (var i = 0; i < results.length; i++) {
                for (var j = 0; j < results[i].length; j++) {
                    allArticles.push(results[i][j]);
                }
            }
            if (allArticles.length === 0) {
                ViewManager.showInputView();
                ViewManager.showError("input-error", "No articles found in category feeds");
                return;
            }
            if (AppConfig.GROUP_SORT_BY_DATE) {
                allArticles.sort(function(a, b) {
                    return articleTimestamp(b) - articleTimestamp(a);
                });
                // Feed name moves into the title since there are no per-feed sections.
                for (var m = 0; m < allArticles.length; m++) {
                    allArticles[m].feedPrefix = allArticles[m].feedTitle || "";
                }
            }
            for (var k = 0; k < allArticles.length; k++) {
                allArticles[k].index = k;
            }
            AppState.currentArticles = allArticles;
            FeedRenderer.renderArticleList(allArticles);
        }
    }

    for (var i = 0; i < categoryFeeds.length; i++) {
        (function(feed, feedIndex) {
            if (AppConfig.USE_BACKEND) {
                BackendClient.fetchFeed(feed.url, function(error, data) {
                    if (error || !data || !data.articles) {
                        onFeedDone(feedIndex, null);
                    } else {
                        for (var k = 0; k < data.articles.length; k++) {
                            data.articles[k].feedTitle = data.title || feed.title;
                        }
                        onFeedDone(feedIndex, data.articles);
                    }
                });
            } else {
                fetchUrl(feed.url, function(error, xmlText) {
                    if (error) {
                        onFeedDone(feedIndex, null);
                        return;
                    }
                    try {
                        var parsed = parseFeedXml(xmlText);
                        for (var k = 0; k < parsed.articles.length; k++) {
                            parsed.articles[k].feedTitle = parsed.title || feed.title;
                        }
                        onFeedDone(feedIndex, parsed.articles.length > 0 ? parsed.articles : null);
                    } catch (e) {
                        onFeedDone(feedIndex, null);
                    }
                });
            }
        })(categoryFeeds[i], i);
    }
}
