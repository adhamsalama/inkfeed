// Comments Viewer (Reddit JSON and HTML)

var CommentsViewer = {
    fetchComments: function() {
        try {
            var article = AppState.currentArticles[AppState.currentArticleIndex];
            if (!article || !article.comments) {
                return;
            }

            var commentsContent = document.getElementById("comments-content");
            var commentsLoading = document.getElementById("comments-loading");
            var articleContent = document.getElementById("article-content");
            var commentsHtmlContent = document.getElementById("comments-html-content");

            // Hide article and other comments view
            removeClass(articleContent, "visible");
            removeClass(commentsHtmlContent, "visible");

            // Show loading indicator
            removeClass(commentsLoading, "hidden");

            var isHnComments = article.comments.indexOf("news.ycombinator.com/item?id=") >= 0;

            if (isHnComments && !AppConfig.USE_BACKEND) {
                var hnId = "";
                var qIdx = article.comments.indexOf("?id=");
                if (qIdx >= 0) {
                    hnId = article.comments.substring(qIdx + 4);
                    var ampIdx = hnId.indexOf("&");
                    if (ampIdx >= 0) hnId = hnId.substring(0, ampIdx);
                    var hashIdx = hnId.indexOf("#");
                    if (hashIdx >= 0) hnId = hnId.substring(0, hashIdx);
                }
                if (!hnId) {
                    commentsContent.innerHTML = '<p class="error">Could not extract HN item ID from URL.</p>';
                    addClass(commentsContent, "visible");
                    addClass(commentsLoading, "hidden");
                    return;
                }

                var algoliaUrl = "https://hn.algolia.com/api/v1/items/" + hnId;
                var hnXhr;
                if (window.XMLHttpRequest) {
                    hnXhr = new XMLHttpRequest();
                } else {
                    hnXhr = new ActiveXObject("Microsoft.XMLHTTP");
                }
                hnXhr.onreadystatechange = function() {
                    if (hnXhr.readyState !== 4) return;
                    addClass(commentsLoading, "hidden");
                    if (hnXhr.status !== 200) {
                        commentsContent.innerHTML = '<p class="error">Error fetching HN comments: HTTP ' + hnXhr.status + '</p>';
                        addClass(commentsContent, "visible");
                        return;
                    }
                    try {
                        var data = JSON.parse(hnXhr.responseText);
                        var hnCounter = [0];

                        var renderHNComment = function(comment, depth, collapsed) {
                            if (!comment) return "";
                            var text = comment.text || "";
                            var author = comment.author || "[deleted]";
                            var createdAt = comment.created_at || "";
                            var children = comment.children || [];
                            var n = hnCounter[0]++;
                            var collapseId = "hn-c-" + n;
                            var dateStr = createdAt ? createdAt.substring(0, 10) : "";
                            var toggleIcon = collapsed ? "[+]" : "[&minus;]";
                            var bodyStyle = collapsed ? ' style="display:none"' : "";

                            var parts = [];
                            parts.push('<div class="hn-comment">');
                            parts.push('<div class="hn-comment-header">');
                            parts.push('<span id="' + collapseId + '-btn" class="hn-toggle" onclick="toggleHNComment(\'' + collapseId + '\')">' + toggleIcon + '</span> ');
                            parts.push('<strong class="hn-author">' + escapeHtml(author) + '</strong>');
                            if (dateStr) {
                                parts.push(' <span class="hn-date">' + escapeHtml(dateStr) + '</span>');
                            }
                            parts.push('</div>');
                            parts.push('<div id="' + collapseId + '" class="hn-comment-body"' + bodyStyle + '>');
                            if (text) {
                                parts.push('<div class="hn-comment-text">' + text + '</div>');
                            } else {
                                parts.push('<div class="hn-comment-text hn-deleted">[deleted]</div>');
                            }
                            for (var ci = 0; ci < children.length; ci++) {
                                parts.push(renderHNComment(children[ci], depth + 1, true));
                            }
                            parts.push('</div>');
                            parts.push('</div>');
                            return parts.join("");
                        };

                        var topChildren = data.children || [];
                        if (topChildren.length === 0) {
                            commentsContent.innerHTML = '<p>No comments yet.</p>';
                        } else {
                            var htmlParts = [];
                            var maxComments = Math.min(topChildren.length, AppConfig.MAX_TOP_LEVEL_COMMENTS);
                            for (var ci = 0; ci < maxComments; ci++) {
                                htmlParts.push(renderHNComment(topChildren[ci], 0, ci >= 10));
                            }
                            commentsContent.innerHTML = '<div class="comments-body hn-comments">' + htmlParts.join("") + '</div>';
                        }
                        addClass(commentsContent, "visible");
                    } catch (e) {
                        commentsContent.innerHTML = '<p class="error">Error parsing HN comments: ' + escapeHtml(e.message) + '</p>';
                        addClass(commentsContent, "visible");
                    }
                };
                hnXhr.open("GET", algoliaUrl, true);
                hnXhr.send(null);
                return;
            }

            var isLobstersComments = article.comments.indexOf("lobste.rs/s/") >= 0;

            if (isLobstersComments && !AppConfig.USE_BACKEND) {
                var getLobstersJsonUrl = function(url) {
                    var idx = url.indexOf("/s/");
                    if (idx < 0) { return null; }
                    var rest = url.substring(idx + 3);
                    var slashIdx = rest.indexOf("/");
                    var shortID = slashIdx >= 0 ? rest.substring(0, slashIdx) : rest;
                    var qIdx = shortID.indexOf("?");
                    if (qIdx >= 0) { shortID = shortID.substring(0, qIdx); }
                    var hIdx = shortID.indexOf("#");
                    if (hIdx >= 0) { shortID = shortID.substring(0, hIdx); }
                    if (!shortID) { return null; }
                    return "https://lobste.rs/s/" + shortID + ".json";
                };

                var buildLobstersTree = function(comments) {
                    var nodes = {};
                    for (var i = 0; i < comments.length; i++) {
                        nodes[comments[i].short_id] = { comment: comments[i], children: [] };
                    }
                    var roots = [];
                    for (var i = 0; i < comments.length; i++) {
                        var c = comments[i];
                        var node = nodes[c.short_id];
                        if (!c.parent_comment) {
                            roots.push(node);
                        } else if (nodes[c.parent_comment]) {
                            nodes[c.parent_comment].children.push(node);
                        } else {
                            roots.push(node);
                        }
                    }
                    return roots;
                };

                var lobCounter = [0];
                var renderLobstersComment = function(node, collapsed) {
                    var c = node.comment;
                    var n = lobCounter[0]++;
                    var collapseId = "lob-c-" + n;
                    var author = "[deleted]";
                    if (!c.is_deleted && !c.is_moderated && c.commenting_user) {
                        author = (typeof c.commenting_user === "string" ? c.commenting_user : c.commenting_user.username) || "[deleted]";
                    }
                    var dateStr = c.created_at ? c.created_at.substring(0, 10) : "";
                    var toggleIcon = collapsed ? "[+]" : "[&minus;]";
                    var bodyStyle = collapsed ? ' style="display:none"' : "";
                    var parts = [];
                    parts.push('<div class="hn-comment">');
                    parts.push('<div class="hn-comment-header">');
                    parts.push('<span id="' + collapseId + '-btn" class="hn-toggle" onclick="toggleLobstersComment(\'' + collapseId + '\')">' + toggleIcon + '</span> ');
                    parts.push('<strong class="hn-author">' + escapeHtml(author) + '</strong>');
                    if (dateStr) {
                        parts.push(' <span class="hn-date">' + escapeHtml(dateStr) + '</span>');
                    }
                    parts.push('</div>');
                    parts.push('<div id="' + collapseId + '" class="hn-comment-body"' + bodyStyle + '>');
                    if (c.is_deleted || c.is_moderated) {
                        parts.push('<div class="hn-comment-text hn-deleted">[deleted]</div>');
                    } else if (c.comment) {
                        parts.push('<div class="hn-comment-text">' + c.comment + '</div>');
                    }
                    for (var ci = 0; ci < node.children.length; ci++) {
                        parts.push(renderLobstersComment(node.children[ci], true));
                    }
                    parts.push('</div>');
                    parts.push('</div>');
                    return parts.join("");
                };

                var lobJsonUrl = getLobstersJsonUrl(article.comments);
                if (!lobJsonUrl) {
                    commentsContent.innerHTML = '<p class="error">Could not parse Lobste.rs story URL.</p>';
                    addClass(commentsContent, "visible");
                    addClass(commentsLoading, "hidden");
                    return;
                }
                fetchUrl(lobJsonUrl, function(error, responseText) {
                    addClass(commentsLoading, "hidden");
                    if (error) {
                        commentsContent.innerHTML = '<p class="error">Error fetching Lobste.rs comments: ' + escapeHtml(error.message) + '</p>';
                        addClass(commentsContent, "visible");
                        return;
                    }
                    try {
                        var data = JSON.parse(responseText);
                        var comments = data.comments || [];
                        if (comments.length === 0) {
                            commentsContent.innerHTML = '<p>No comments yet.</p>';
                            addClass(commentsContent, "visible");
                            return;
                        }
                        var roots = buildLobstersTree(comments);
                        var htmlParts = [];
                        var limit = Math.min(roots.length, AppConfig.MAX_TOP_LEVEL_COMMENTS);
                        for (var i = 0; i < limit; i++) {
                            htmlParts.push(renderLobstersComment(roots[i], i >= 10));
                        }
                        commentsContent.innerHTML = '<div class="comments-body hn-comments">' + htmlParts.join("") + '</div>';
                        addClass(commentsContent, "visible");
                    } catch (e) {
                        commentsContent.innerHTML = '<p class="error">Error parsing Lobste.rs comments: ' + escapeHtml(e.message) + '</p>';
                        addClass(commentsContent, "visible");
                    }
                });
                return;
            }

            var isRedditComments = article.comments.indexOf(".json") >= 0;

            // Client-side Reddit fetch+render, used directly or as backend fallback
            var fetchRedditClientSide = function() {
                fetchUrl(article.comments, function(error, responseText) {
                    addClass(commentsLoading, "hidden");
                    if (error) {
                        commentsContent.innerHTML = '<p class="error">Error fetching comments: ' + escapeHtml(error.message) + "</p>";
                        addClass(commentsContent, "visible");
                        return;
                    }
                    try {
                        var json = JSON.parse(responseText);
                        var redditCounter = [0];
                        var replyCount = {count: 0};

                        var renderRedditComment = function(commentData, depth, isTopLevel, collapsed) {
                            if (!commentData || !commentData.data) return "";
                            var data = commentData.data;
                            if (commentData.kind === "more") return "";
                            if (!isTopLevel) {
                                if (replyCount.count >= AppConfig.MAX_REPLIES_PER_COMMENT) return "";
                                replyCount.count++;
                            }
                            var n = redditCounter[0]++;
                            var collapseId = "rc-" + n;
                            var toggleIcon = collapsed ? "[+]" : "[&minus;]";
                            var bodyStyle = collapsed ? ' style="display:none"' : "";

                            var author = data.author || "[deleted]";
                            var parts = [];
                            parts.push('<div class="hn-comment">');
                            parts.push('<div class="hn-comment-header">');
                            parts.push('<span id="' + collapseId + '-btn" class="hn-toggle" onclick="toggleRedditComment(\'' + collapseId + '\')">' + toggleIcon + '</span> ');
                            parts.push('<strong class="hn-author">' + escapeHtml(author) + '</strong>');
                            if (data.created_utc) {
                                var date = new Date(data.created_utc * 1000);
                                parts.push(' <span class="hn-date">' + escapeHtml(date.toLocaleDateString()) + '</span>');
                            }
                            parts.push('</div>');
                            parts.push('<div id="' + collapseId + '" class="hn-comment-body"' + bodyStyle + '>');
                            if (data.body_html) {
                                var tempDiv = document.createElement("div");
                                tempDiv.innerHTML = data.body_html;
                                var textContent = getText(tempDiv);
                                textContent = textContent.replace(/\n/g, "<br>");
                                parts.push('<div class="hn-comment-text">' + textContent + '</div>');
                            }
                            if (data.replies && data.replies.data && data.replies.data.children) {
                                var replies = data.replies.data.children;
                                for (var ri = 0; ri < replies.length; ri++) {
                                    parts.push(renderRedditComment(replies[ri], depth + 1, false, true));
                                }
                            }
                            parts.push('</div>');
                            parts.push('</div>');
                            return parts.join("");
                        };

                        var htmlParts = [];
                        if (json.length > 1 && json[1].data && json[1].data.children) {
                            var comments = json[1].data.children;
                            var maxComments = Math.min(comments.length, AppConfig.MAX_TOP_LEVEL_COMMENTS);
                            for (var i = 0; i < maxComments; i++) {
                                replyCount.count = 0;
                                htmlParts.push(renderRedditComment(comments[i], 0, true, i >= 10));
                            }
                        }
                        if (htmlParts.length === 0) {
                            commentsContent.innerHTML = '<p class="error">No comments found.</p>';
                        } else {
                            commentsContent.innerHTML = '<div class="comments-body">' + htmlParts.join("") + '</div>';
                        }
                        addClass(commentsContent, "visible");
                    } catch (e) {
                        commentsContent.innerHTML = '<p class="error">Error parsing Reddit comments: ' + escapeHtml(e.message) + '</p>';
                        addClass(commentsContent, "visible");
                    }
                });
            };

            if (AppConfig.USE_BACKEND) {
                BackendClient.fetchComments(article.comments, function(error, data) {
                    if (error && isRedditComments) {
                        // Backend failed (e.g. Reddit rate limit) — fall back to client-side
                        fetchRedditClientSide();
                        return;
                    }
                    addClass(commentsLoading, "hidden");
                    if (error) {
                        commentsContent.innerHTML = '<p class="error">Error fetching comments: ' + escapeHtml(error.message) + "</p>";
                    } else {
                        commentsContent.innerHTML = '<div class="comments-body">' + data.html + "</div>";
                    }
                    addClass(commentsContent, "visible");
                });
                return;
            }

            if (isRedditComments) {
                fetchRedditClientSide();
                return;
            }

            fetchUrl(article.comments, function (error, responseText) {
                if (error) {
                    commentsContent.innerHTML =
                        '<p class="error">Error fetching comments: ' +
                        escapeHtml(error.message) +
                        "</p>";
                    addClass(commentsContent, "visible");
                    addClass(commentsLoading, "hidden");
                    return;
                }

                try {
                    // Readability fallback for non-Reddit/non-HN pages
                    var doc;
                    if (window.DOMParser) {
                        doc = new DOMParser().parseFromString(responseText, "text/html");
                    } else {
                        doc = document.createElement("div");
                        doc.innerHTML = responseText;
                    }
                    var reader = new Readability(doc);
                    var extractedContent = reader.parse();
                    if (extractedContent && extractedContent.content) {
                        commentsContent.innerHTML = '<div class="comments-body">' + extractedContent.content + "</div>";
                    } else {
                        commentsContent.innerHTML = '<p class="error">Could not parse comments from this page.</p>';
                    }
                    addClass(commentsContent, "visible");
                } catch (e) {
                    commentsContent.innerHTML = '<p class="error">Error parsing comments: ' + escapeHtml(e.message) + "</p>";
                    addClass(commentsContent, "visible");
                }

                addClass(commentsLoading, "hidden");
            });
        } catch (e) {
            alert("fetchComments error: " + e.message);
        }
    },

    fetchCommentsHtml: function() {
        try {
            var article = AppState.currentArticles[AppState.currentArticleIndex];
            if (!article || !article.comments) {
                return;
            }

            var commentsHtmlContent = document.getElementById(
                "comments-html-content"
            );
            var commentsLoading = document.getElementById("comments-loading");
            var articleContent = document.getElementById("article-content");
            var commentsContent = document.getElementById("comments-content");

            // Hide article and other comments view
            removeClass(articleContent, "visible");
            removeClass(commentsContent, "visible");

            removeClass(commentsLoading, "hidden");

            if (AppConfig.USE_BACKEND) {
                BackendClient.fetchComments(article.comments, function(error, data) {
                    addClass(commentsLoading, "hidden");
                    if (error) {
                        commentsHtmlContent.innerHTML = "Error fetching comments: " + escapeHtml(error.message);
                    } else {
                        commentsHtmlContent.innerHTML = data.html;
                    }
                    addClass(commentsHtmlContent, "visible");
                });
                return;
            }

            fetchUrl(article.comments, function (error, htmlText) {
                if (error) {
                    commentsHtmlContent.innerHTML =
                        "Error fetching comments: " + escapeHtml(error.message);
                    addClass(commentsHtmlContent, "visible");
                    addClass(commentsLoading, "hidden");
                    return;
                }

                // Display raw HTML rendered
                commentsHtmlContent.innerHTML = htmlText;
                addClass(commentsHtmlContent, "visible");
                addClass(commentsLoading, "hidden");
            });
        } catch (e) {
            alert("fetchCommentsHtml error: " + e.message);
        }
    }
};

window.toggleRedditComment = function(collapseId) {
    var el = document.getElementById(collapseId);
    var btn = document.getElementById(collapseId + "-btn");
    if (!el) return;
    if (el.style.display === "none") {
        el.style.display = "";
        if (btn) btn.innerHTML = "[&minus;]";
    } else {
        el.style.display = "none";
        if (btn) btn.innerHTML = "[+]";
    }
};

window.toggleHNComment = function(collapseId) {
    var el = document.getElementById(collapseId);
    var btn = document.getElementById(collapseId + "-btn");
    if (!el) return;
    if (el.style.display === "none") {
        el.style.display = "";
        if (btn) btn.innerHTML = "[&minus;]";
    } else {
        el.style.display = "none";
        if (btn) btn.innerHTML = "[+]";
    }
};

window.toggleLobstersComment = function(collapseId) {
    var el = document.getElementById(collapseId);
    var btn = document.getElementById(collapseId + "-btn");
    if (!el) return;
    if (el.style.display === "none") {
        el.style.display = "";
        if (btn) btn.innerHTML = "[&minus;]";
    } else {
        el.style.display = "none";
        if (btn) btn.innerHTML = "[+]";
    }
};
