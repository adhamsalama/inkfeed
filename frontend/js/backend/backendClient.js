// Backend API client
// All calls go directly to the backend (no CORS proxy).

var BackendClient = {
    _get: function(path, callback) {
        var xhr = window.XMLHttpRequest ? new XMLHttpRequest() : new ActiveXObject("Microsoft.XMLHTTP");
        xhr.onreadystatechange = function() {
            if (xhr.readyState === 4) {
                if (xhr.status === 200) {
                    try {
                        var data = JSON.parse(xhr.responseText);
                        if (data.error) {
                            callback(new Error(data.error), null);
                        } else {
                            callback(null, data);
                        }
                    } catch (e) {
                        callback(e, null);
                    }
                } else {
                    callback(new Error("Backend error: " + xhr.status), null);
                }
            }
        };
        xhr.open("GET", AppConfig.BACKEND_URL + path, true);
        xhr.withCredentials = true;
        xhr.send(null);
    },

    fetchFeed: function(feedUrl, callback) {
        BackendClient._get("/feed?url=" + encodeURIComponent(feedUrl), callback);
    },

    fetchArticle: function(articleUrl, callback) {
        BackendClient._get("/article?url=" + encodeURIComponent(articleUrl), callback);
    },

    fetchComments: function(commentsUrl, callback) {
        BackendClient._get("/comments?url=" + encodeURIComponent(commentsUrl), callback);
    },

    sendEmail: function(articleUrl, to, format, author, commentsUrl, callback) {
        var xhr = window.XMLHttpRequest ? new XMLHttpRequest() : new ActiveXObject("Microsoft.XMLHTTP");
        xhr.onreadystatechange = function() {
            if (xhr.readyState === 4) {
                if (xhr.status === 200) {
                    callback(null);
                } else {
                    try {
                        var data = JSON.parse(xhr.responseText);
                        callback(new Error(data.error || "Send failed"));
                    } catch (e) {
                        callback(new Error("Backend error: " + xhr.status));
                    }
                }
            }
        };
        xhr.open("POST", AppConfig.BACKEND_URL + "/email", true);
        xhr.withCredentials = true;
        xhr.setRequestHeader("Content-Type", "application/json");
        xhr.send(JSON.stringify({ url: articleUrl, to: to, format: format, author: author, commentsUrl: commentsUrl || "", embedImages: format === "mobi" ? AppConfig.MOBI_EMBED_IMAGES : AppConfig.EPUB_EMBED_IMAGES }));
    },

    emailBulk: function(articleUrls, to, format, author, callback) {
        var xhr = window.XMLHttpRequest ? new XMLHttpRequest() : new ActiveXObject("Microsoft.XMLHTTP");
        xhr.onreadystatechange = function() {
            if (xhr.readyState === 4) {
                if (xhr.status === 200) {
                    callback(null);
                } else {
                    try {
                        var data = JSON.parse(xhr.responseText);
                        callback(new Error(data.error || "Send failed"));
                    } catch (e) {
                        callback(new Error("Backend error: " + xhr.status));
                    }
                }
            }
        };
        xhr.open("POST", AppConfig.BACKEND_URL + "/email", true);
        xhr.withCredentials = true;
        xhr.setRequestHeader("Content-Type", "application/json");
        xhr.send(JSON.stringify({ urls: articleUrls, to: to, format: format, author: author, embedImages: format === "mobi" ? AppConfig.MOBI_EMBED_IMAGES : AppConfig.EPUB_EMBED_IMAGES }));
    },

    fetchFeedArchive: function(feedUrl, limit, offset, callback) {
        BackendClient._get("/feed-archive?url=" + encodeURIComponent(feedUrl) + "&limit=" + limit + "&offset=" + offset, callback);
    },

    searchFeedArchive: function(feedUrls, query, limit, offset, callback) {
        var params = "";
        for (var i = 0; i < feedUrls.length; i++) {
            params += "url=" + encodeURIComponent(feedUrls[i]) + "&";
        }
        BackendClient._get("/feed-archive/search?" + params + "q=" + encodeURIComponent(query) + "&limit=" + limit + "&offset=" + offset, callback);
    },

    fetchRedditPost: function(redditJsonUrl, callback) {
        BackendClient._get("/reddit-post?url=" + encodeURIComponent(redditJsonUrl), callback);
    },

    decodeGoogleNewsUrl: function(googleNewsUrl, callback) {
        BackendClient._get("/decode-google-news?url=" + encodeURIComponent(googleNewsUrl), callback);
    },

    // Download a single article as plain text. The backend fetches and extracts the article.
    downloadText: function(articleUrl, filename) {
        var xhr = window.XMLHttpRequest ? new XMLHttpRequest() : new ActiveXObject("Microsoft.XMLHTTP");
        xhr.onreadystatechange = function() {
            if (xhr.readyState === 4) {
                if (xhr.status === 200) {
                    var blob = new Blob([xhr.responseText], { type: "text/plain;charset=utf-8" });
                    downloadBlob(blob, filename);
                } else {
                    alert("Backend text error: " + xhr.status);
                }
            }
        };
        xhr.open("GET", AppConfig.BACKEND_URL + "/text?url=" + encodeURIComponent(articleUrl), true);
        xhr.withCredentials = true;
        xhr.send(null);
    },

    // Download a single article as MOBI. The backend fetches and extracts the article.
    downloadMobi: function(articleUrl, title, author, commentsUrl, filename) {
        BackendClient._downloadMobiPost({ url: articleUrl, title: title, author: author, commentsUrl: commentsUrl || "", embedImages: AppConfig.MOBI_EMBED_IMAGES }, filename);
    },

    // Download multiple articles as a single MOBI. The backend fetches all URLs.
    downloadMobiBulk: function(articleUrls, title, author, filename) {
        BackendClient._downloadMobiPost({ urls: articleUrls, title: title, author: author, embedImages: AppConfig.MOBI_EMBED_IMAGES }, filename);
    },

    // Download a single article as EPUB. The backend fetches and extracts the article.
    downloadEpub: function(articleUrl, title, author, commentsUrl, filename) {
        BackendClient._downloadEpubPost({ url: articleUrl, title: title, author: author, commentsUrl: commentsUrl || "", embedImages: AppConfig.EPUB_EMBED_IMAGES }, filename);
    },

    // Download multiple articles as a single EPUB. The backend fetches all URLs.
    downloadEpubBulk: function(articleUrls, title, author, filename) {
        BackendClient._downloadEpubPost({ urls: articleUrls, title: title, author: author, embedImages: AppConfig.EPUB_EMBED_IMAGES }, filename);
    },

    _downloadEpubPost: function(body, filename) {
        var xhr = window.XMLHttpRequest ? new XMLHttpRequest() : null;
        if (!xhr || typeof xhr.responseType === "undefined") {
            alert("Your browser does not support binary downloads from the backend.");
            return;
        }
        xhr.responseType = "arraybuffer";
        xhr.onreadystatechange = function() {
            if (xhr.readyState === 4) {
                if (xhr.status === 200) {
                    var blob = new Blob([xhr.response], { type: "application/epub+zip" });
                    downloadBlob(blob, filename);
                } else {
                    alert("Backend EPUB error: " + xhr.status);
                }
            }
        };
        xhr.open("POST", AppConfig.BACKEND_URL + "/epub", true);
        xhr.withCredentials = true;
        xhr.setRequestHeader("Content-Type", "application/json");
        xhr.send(JSON.stringify(body));
    },

    // Internal: POST body to /mobi and trigger a binary download.
    // Requires arraybuffer + Blob support (IE10+, all modern browsers).
    _downloadMobiPost: function(body, filename) {
        var xhr = window.XMLHttpRequest ? new XMLHttpRequest() : null;
        if (!xhr || typeof xhr.responseType === "undefined") {
            alert("Your browser does not support binary downloads from the backend.");
            return;
        }
        xhr.responseType = "arraybuffer";
        xhr.onreadystatechange = function() {
            if (xhr.readyState === 4) {
                if (xhr.status === 200) {
                    var blob = new Blob([xhr.response], { type: "application/x-mobipocket-ebook" });
                    downloadBlob(blob, filename);
                } else {
                    alert("Backend MOBI error: " + xhr.status);
                }
            }
        };
        xhr.open("POST", AppConfig.BACKEND_URL + "/mobi", true);
        xhr.withCredentials = true;
        xhr.setRequestHeader("Content-Type", "application/json");
        xhr.send(JSON.stringify(body));
    }
};
