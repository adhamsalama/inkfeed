// Event Handlers Setup (IIFE)
(function() {
    // Export init function
    window.initEventHandlers = function() {
        // MOBI download button handler
        var downloadMobiBtn = document.getElementById("download-mobi-btn");
        if (downloadMobiBtn) {
            var downloadMobiHandler = function () {
                if (
                    typeof AppState.currentArticleIndex === "undefined" ||
                    AppState.currentArticleIndex < 0
                ) {
                    alert("No article selected");
                    return;
                }

                var article = AppState.currentArticles[AppState.currentArticleIndex];
                var contentDiv = document.getElementById("article-content");
                var articleHtml = contentDiv.innerHTML;

                // If there's a comments URL, fetch and include comments
                if (article.comments) {
                    if (AppConfig.USE_BACKEND) {
                        // Backend re-fetches the article from its URL directly
                        MobiDownloader.generateAndDownloadMobi(article, articleHtml, "");
                    } else {
                        fetchUrl(article.comments, function (error, htmlText) {
                            var commentsHtml = "";
                            if (!error && htmlText) {
                                try {
                                    var doc;
                                    if (window.DOMParser) {
                                        doc = new DOMParser().parseFromString(
                                            htmlText,
                                            "text/html"
                                        );
                                    } else {
                                        doc = document.createElement("div");
                                        doc.innerHTML = htmlText;
                                    }
                                    var reader = new Readability(doc);
                                    var parsed = reader.parse();
                                    if (parsed && parsed.content) {
                                        commentsHtml = parsed.content;
                                    }
                                } catch (e) {
                                    // Ignore parsing errors, just skip comments
                                }
                            }
                            MobiDownloader.generateAndDownloadMobi(article, articleHtml, commentsHtml);
                        });
                    }
                } else {
                    MobiDownloader.generateAndDownloadMobi(article, articleHtml, "");
                }
            };

            if (downloadMobiBtn.addEventListener) {
                downloadMobiBtn.addEventListener("click", downloadMobiHandler, false);
            } else if (downloadMobiBtn.attachEvent) {
                downloadMobiBtn.attachEvent("onclick", downloadMobiHandler);
            }
        }

        // EPUB download button handler
        var downloadEpubBtn = document.getElementById("download-epub-btn");
        if (downloadEpubBtn) {
            var downloadEpubHandler = function () {
                if (
                    typeof AppState.currentArticleIndex === "undefined" ||
                    AppState.currentArticleIndex < 0
                ) {
                    alert("No article selected");
                    return;
                }

                var article = AppState.currentArticles[AppState.currentArticleIndex];
                var contentDiv = document.getElementById("article-content");
                var articleHtml = contentDiv.innerHTML;

                if (AppConfig.USE_BACKEND) {
                    var title = article.title || "Article";
                    var feedTitle = getText(document.getElementById("feed-title")) || "RSS Reader";
                    var filename = title.replace(/[^a-z0-9]/gi, "_") + ".epub";
                    BackendClient.downloadEpub(article.link, title, feedTitle, article.comments || "", filename);
                    return;
                }

                // If there's a comments URL, fetch and include comments
                if (article.comments) {
                    fetchUrl(article.comments, function (error, htmlText) {
                        var commentsHtml = "";
                        if (!error && htmlText) {
                            try {
                                var doc;
                                if (window.DOMParser) {
                                    doc = new DOMParser().parseFromString(
                                        htmlText,
                                        "text/html"
                                    );
                                } else {
                                    doc = document.createElement("div");
                                    doc.innerHTML = htmlText;
                                }
                                var reader = new Readability(doc);
                                var parsed = reader.parse();
                                if (parsed && parsed.content) {
                                    commentsHtml = parsed.content;
                                }
                            } catch (e) {
                                // Ignore parsing errors, just skip comments
                            }
                        }
                        EpubDownloader.generateAndDownloadEpub(article, articleHtml, commentsHtml);
                    });
                } else {
                    EpubDownloader.generateAndDownloadEpub(article, articleHtml, "");
                }
            };

            if (downloadEpubBtn.addEventListener) {
                downloadEpubBtn.addEventListener("click", downloadEpubHandler, false);
            } else if (downloadEpubBtn.attachEvent) {
                downloadEpubBtn.attachEvent("onclick", downloadEpubHandler);
            }
        }

        // View Comments button handler
        var viewCommentsBtn = document.getElementById("view-comments-btn");
        if (viewCommentsBtn) {
            if (viewCommentsBtn.addEventListener) {
                viewCommentsBtn.addEventListener("click", CommentsViewer.fetchComments, false);
            } else if (viewCommentsBtn.attachEvent) {
                viewCommentsBtn.attachEvent("onclick", CommentsViewer.fetchComments);
            }
        }

        // View Comments HTML button handler
        var viewCommentsHtmlBtn = document.getElementById("view-comments-html-btn");
        if (viewCommentsHtmlBtn) {
            if (viewCommentsHtmlBtn.addEventListener) {
                viewCommentsHtmlBtn.addEventListener("click", CommentsViewer.fetchCommentsHtml, false);
            } else if (viewCommentsHtmlBtn.attachEvent) {
                viewCommentsHtmlBtn.attachEvent("onclick", CommentsViewer.fetchCommentsHtml);
            }
        }

        // Wikipedia search input Enter key handler
        var wikiInput = document.getElementById("wikipedia-search-input");
        if (wikiInput) {
            if (wikiInput.addEventListener) {
                wikiInput.addEventListener("keypress", function(e) {
                    var key = e.key || e.keyCode;
                    if (key === "Enter" || key === 13) {
                        searchWikipedia();
                    }
                });
            } else if (wikiInput.attachEvent) {
                wikiInput.attachEvent("onkeypress", function(e) {
                    if (e.keyCode === 13) {
                        searchWikipedia();
                    }
                });
            }
        }

        // Archive search input Enter key handler
        var archiveSearchInput = document.getElementById("archive-search-input");
        if (archiveSearchInput) {
            if (archiveSearchInput.addEventListener) {
                archiveSearchInput.addEventListener("keypress", function(e) {
                    var key = e.key || e.keyCode;
                    if (key === "Enter" || key === 13) {
                        searchFeedArchive();
                    }
                });
            } else if (archiveSearchInput.attachEvent) {
                archiveSearchInput.attachEvent("onkeypress", function(e) {
                    if (e.keyCode === 13) {
                        searchFeedArchive();
                    }
                });
            }
        }

        // Feed input Enter key handler
        var feedInput = document.getElementById("feed-url");
        if (feedInput) {
            if (feedInput.addEventListener) {
                feedInput.addEventListener("keypress", function (e) {
                    var key = e.key || e.keyCode;
                    if (key === "Enter" || key === 13) {
                        loadFeed();
                    }
                });
            } else if (feedInput.attachEvent) {
                feedInput.attachEvent("onkeypress", function (e) {
                    if (e.keyCode === 13) {
                        loadFeed();
                    }
                });
            }
        }
    };
})();
