// Feed Renderer (saved feeds list and article list)

var FeedRenderer = {
    renderSavedFeeds: function() {
        var list = document.getElementById("saved-feeds-list");
        var feeds = SavedFeedsManager.getSavedFeeds();

        var groupsBtn = document.getElementById("groups-toggle-btn");
        if (groupsBtn) {
            if (feeds.length > 0) {
                removeClass(groupsBtn, "hidden");
            } else {
                addClass(groupsBtn, "hidden");
                removeClass(groupsBtn, "btn-active");
                addClass(document.getElementById("groups-section"), "hidden");
            }
        }

        list.innerHTML = "";

        if (feeds.length === 0) {
            var emptyMsg = document.createElement("li");
            emptyMsg.className = "saved-feed-item";
            setText(emptyMsg, "No saved feeds");
            list.appendChild(emptyMsg);
            return;
        }

        for (var i = 0; i < feeds.length; i++) {
            var feed = feeds[i];

            var li = document.createElement("li");
            li.className = "saved-feed-item";

            var link = document.createElement("a");
            link.className = "saved-feed-link";
            setText(link, feed.title || feed.url);
            link.title = feed.url;
            // Use closure to capture url
            (function (url) {
                link.onclick = function () {
                    SavedFeedsManager.loadSavedFeed(url);
                    return false;
                };
            })(feed.url);

            // Up arrow button
            var upBtn = document.createElement("button");
            upBtn.className = "secondary delete-feed-btn";
            setText(upBtn, "^");
            upBtn.title = "Move up";
            (function (index) {
                upBtn.onclick = function () {
                    SavedFeedsManager.moveFeed(index, -1);
                    return false;
                };
            })(i);

            // Down arrow button
            var downBtn = document.createElement("button");
            downBtn.className = "secondary delete-feed-btn";
            setText(downBtn, "v");
            downBtn.title = "Move down";
            (function (index) {
                downBtn.onclick = function () {
                    SavedFeedsManager.moveFeed(index, 1);
                    return false;
                };
            })(i);

            var deleteBtn = document.createElement("button");
            deleteBtn.className = "secondary delete-feed-btn";
            setText(deleteBtn, "X");
            deleteBtn.title = "Delete";
            // Use closure to capture url
            (function (url) {
                deleteBtn.onclick = function () {
                    SavedFeedsManager.deleteFeed(url);
                    return false;
                };
            })(feed.url);

            var groupBtn = document.createElement("button");
            groupBtn.className = "secondary delete-feed-btn";
            setText(groupBtn, "+");
            groupBtn.title = "Add to group";
            (function(url, item) {
                groupBtn.onclick = function() {
                    FeedGroupsManager.addSavedFeedToGroup(url, item);
                    return false;
                };
            })(feed.url, li);

            li.appendChild(link);
            li.appendChild(upBtn);
            li.appendChild(downBtn);
            li.appendChild(deleteBtn);
            li.appendChild(groupBtn);

            if (AppConfig.USE_BACKEND) {
                var archiveBtn = document.createElement("button");
                archiveBtn.className = "secondary delete-feed-btn" + (feed.archiveEnabled ? " btn-active" : "");
                setText(archiveBtn, "A");
                archiveBtn.title = feed.archiveEnabled ? "Disable archiving" : "Enable archiving";
                (function(url) {
                    archiveBtn.onclick = function() {
                        SavedFeedsManager.toggleFeedArchive(url);
                        return false;
                    };
                })(feed.url);
                li.appendChild(archiveBtn);
            }

            list.appendChild(li);
        }
    },

    renderFeedGroups: function() {
        var container = document.getElementById("groups-container");
        container.innerHTML = "";

        var groups = FeedGroupsManager.getGroups();
        var savedFeeds = SavedFeedsManager.getSavedFeeds();
        var savedTitles = {};
        for (var s = 0; s < savedFeeds.length; s++) {
            savedTitles[savedFeeds[s].url] = savedFeeds[s].title;
        }

        if (groups.length === 0) {
            var emptyMsg = document.createElement("p");
            setText(emptyMsg, "No groups yet. Click \"+\" on a saved feed to add it to a group.");
            container.appendChild(emptyMsg);
            return;
        }

        for (var i = 0; i < groups.length; i++) {
            var group = groups[i];

            var block = document.createElement("div");
            block.className = "suggested-feeds-section-block";

            var heading = document.createElement("h4");
            heading.className = "suggested-feeds-category";
            setText(heading, group.name);

            var loadAllBtn = document.createElement("button");
            loadAllBtn.className = "secondary load-all-btn";
            setText(loadAllBtn, "Load All");
            (function(feeds, name) {
                loadAllBtn.onclick = function() {
                    FeedGroupsManager.loadGroup(feeds, name);
                    return false;
                };
            })(group.feeds, group.name);

            var deleteGroupBtn = document.createElement("button");
            deleteGroupBtn.className = "secondary delete-feed-btn";
            setText(deleteGroupBtn, "X");
            deleteGroupBtn.title = "Delete group";
            (function(name) {
                deleteGroupBtn.onclick = function() {
                    FeedGroupsManager.deleteGroup(name);
                    return false;
                };
            })(group.name);

            heading.appendChild(loadAllBtn);
            heading.appendChild(deleteGroupBtn);
            block.appendChild(heading);

            var ul = document.createElement("ul");
            ul.className = "saved-feeds-list";

            for (var j = 0; j < group.feeds.length; j++) {
                var feed = group.feeds[j];

                var li = document.createElement("li");
                li.className = "saved-feed-item";

                var link = document.createElement("a");
                link.className = "saved-feed-link";
                setText(link, savedTitles[feed.url] || feed.title || feed.url);
                link.title = feed.url;
                (function(url) {
                    link.onclick = function() {
                        var feedInput = document.getElementById("feed-url");
                        feedInput.value = url;
                        deactivateToggle("groups-section", "groups-toggle-btn");
                        loadFeed();
                        return false;
                    };
                })(feed.url);

                var removeFeedBtn = document.createElement("button");
                removeFeedBtn.className = "secondary delete-feed-btn";
                setText(removeFeedBtn, "X");
                removeFeedBtn.title = "Remove from group";
                (function(groupName, url) {
                    removeFeedBtn.onclick = function() {
                        FeedGroupsManager.removeFeedFromGroup(groupName, url);
                        return false;
                    };
                })(group.name, feed.url);

                li.appendChild(link);
                li.appendChild(removeFeedBtn);
                ul.appendChild(li);
            }

            block.appendChild(ul);
            container.appendChild(block);
        }
    },

    renderSuggestedFeeds: function() {
        var container = document.getElementById("suggested-feeds-container");
        container.innerHTML = "";

        for (var i = 0; i < SUGGESTED_FEEDS.length; i++) {
            var section = SUGGESTED_FEEDS[i];

            var block = document.createElement("div");
            block.className = "suggested-feeds-section-block";

            var heading = document.createElement("h4");
            heading.className = "suggested-feeds-category";
            setText(heading, section.category);

            var loadAllBtn = document.createElement("button");
            loadAllBtn.className = "secondary load-all-btn";
            setText(loadAllBtn, "Load All");
            (function(feeds, name) {
                loadAllBtn.onclick = function() {
                    deactivateToggle("suggested-feeds-section", "suggested-toggle-btn");
                    loadCategoryFeeds(feeds, name);
                    return false;
                };
            })(section.feeds, section.category);

            heading.appendChild(loadAllBtn);
            block.appendChild(heading);

            var ul = document.createElement("ul");
            ul.className = "saved-feeds-list";

            for (var j = 0; j < section.feeds.length; j++) {
                var feed = section.feeds[j];

                var li = document.createElement("li");
                li.className = "saved-feed-item";

                var link = document.createElement("a");
                link.className = "saved-feed-link";
                setText(link, feed.title);
                link.title = feed.url;
                (function(url) {
                    link.onclick = function() {
                        var feedInput = document.getElementById("feed-url");
                        feedInput.value = url;
                        deactivateToggle("suggested-feeds-section", "suggested-toggle-btn");
                        loadFeed();
                        return false;
                    };
                })(feed.url);

                li.appendChild(link);
                ul.appendChild(li);
            }

            block.appendChild(ul);
            container.appendChild(block);
        }
    },

    renderFavorites: function() {
        var container = document.getElementById("favorites-container");
        container.innerHTML = "";

        var favs = FavoritesManager.getFavorites();

        if (favs.length === 0) {
            var emptyMsg = document.createElement("p");
            setText(emptyMsg, "No favorites yet.");
            container.appendChild(emptyMsg);
            return;
        }

        for (var i = 0; i < favs.length; i++) {
            var fav = favs[i];

            var block = document.createElement("div");
            block.className = "suggested-feeds-section-block";

            var ul = document.createElement("ul");
            ul.className = "saved-feeds-list";

            var li = document.createElement("li");
            li.className = "saved-feed-item";

            var link = document.createElement("a");
            link.className = "saved-feed-link";
            setText(link, fav.title || fav.url);
            link.href = "#";
            (function(f) {
                link.onclick = function() {
                    AppState.currentArticles = [{ link: f.url, title: f.title, pubDate: f.pubDate, description: "", content: "", comments: f.commentsUrl || "" }];
                    setText(document.getElementById("feed-title"), f.feedTitle || "");
                    ArticleViewer.openArticle(0);
                    return false;
                };
            })(fav);

            if (fav.feedTitle) {
                var sub = document.createElement("small");
                setText(sub, fav.feedTitle);
                sub.style.display = "block";
                link.appendChild(sub);
            }

            var deleteBtn = document.createElement("button");
            deleteBtn.className = "secondary delete-feed-btn";
            setText(deleteBtn, "X");
            deleteBtn.title = "Remove from favorites";
            (function(url) {
                deleteBtn.onclick = function() {
                    var favs = FavoritesManager.getFavorites();
                    for (var j = 0; j < favs.length; j++) {
                        if (favs[j].url === url) {
                            favs.splice(j, 1);
                            break;
                        }
                    }
                    FavoritesManager.saveFavorites(favs);
                    FavoritesManager.updateFavoriteBtn();
                    return false;
                };
            })(fav.url);

            li.appendChild(link);
            li.appendChild(deleteBtn);
            ul.appendChild(li);
            block.appendChild(ul);
            container.appendChild(block);
        }
    },

    renderArticleList: function(articles) {
        try {
            var list = document.getElementById("article-list");
            var progress = document.getElementById("render-progress");
            var total = articles.length;

            list.innerHTML = "";
            setText(progress, "");

            var fragment = document.createDocumentFragment();
            var lastFeedTitle = null;

            for (var i = 0; i < articles.length; i++) {
                var article = articles[i];

                if (!article.feedPrefix && article.feedTitle && article.feedTitle !== lastFeedTitle) {
                    var separator = document.createElement("li");
                    separator.className = "feed-separator";
                    setText(separator, article.feedTitle);
                    fragment.appendChild(separator);
                    lastFeedTitle = article.feedTitle;
                }

                var li = document.createElement("li");
                li.className = "article-item" + (article.link && AppState.readArticles.has(article.link) ? " article-read" : "");
                li.id = "article-" + i;
                (function (index) {
                    li.onclick = function () {
                        AppState.feedScrollBackId = "article-" + index;
                        ArticleViewer.openArticle(index);
                        return false;
                    };
                })(i);

                var titleRow = document.createElement("div");
                titleRow.className = "article-title-row";

                var title = document.createElement("span");
                title.className = "article-title";
                setText(title, article.feedPrefix ? article.feedPrefix + " \u2014 " + article.title : article.title);

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
                // Strip HTML and truncate
                var plainText = htmlToText(article.description);
                var truncated = plainText.substring(0, 200);
                if (plainText.length > 200) truncated += "...";
                setText(desc, truncated);

                li.appendChild(titleRow);
                if (getText(desc)) li.appendChild(desc);
                fragment.appendChild(li);

            }

            list.appendChild(fragment);
            setText(progress, "(" + total + ")");

            // Scroll to article if returning to feed after page refresh
            var scrollTarget = AppState.pendingScrollTarget;
            if (scrollTarget) {
                AppState.pendingScrollTarget = "";
                var targetEl = document.getElementById(scrollTarget);
                if (targetEl && targetEl.scrollIntoView) {
                    targetEl.scrollIntoView();
                }
            }
        } catch (e) {
            alert("renderArticleList error: " + e.message);
        }
    }
};
