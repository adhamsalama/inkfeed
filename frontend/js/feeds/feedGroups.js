// Feed Groups Manager — user-created named groups of saved feeds
(function() {
    var GROUPS_KEY = "feedGroups";

    var FeedGroupsManager = {
        getGroups: function() {
            try {
                var data = localStorage.getItem(GROUPS_KEY);
                return data ? JSON.parse(data) : [];
            } catch (e) {
                return [];
            }
        },

        saveGroups: function(groups) {
            localStorage.setItem(GROUPS_KEY, JSON.stringify(groups));
            var groupsSection = document.getElementById("groups-section");
            if (groupsSection && groupsSection.className.indexOf("hidden") < 0) {
                FeedRenderer.renderFeedGroups();
            }
            PreferencesSync.pushFeedGroups();
        },

        addFeedToGroup: function(groupName, url, title) {
            var groups = FeedGroupsManager.getGroups();
            var group = null;
            for (var i = 0; i < groups.length; i++) {
                if (groups[i].name === groupName) {
                    group = groups[i];
                    break;
                }
            }
            if (!group) {
                group = { name: groupName, feeds: [] };
                groups.push(group);
            }
            // Skip duplicate URLs
            for (var j = 0; j < group.feeds.length; j++) {
                if (group.feeds[j].url === url) {
                    return;
                }
            }
            group.feeds.push({ url: url, title: title });
            FeedGroupsManager.saveGroups(groups);
        },

        removeFeedFromGroup: function(groupName, url) {
            var groups = FeedGroupsManager.getGroups();
            for (var i = 0; i < groups.length; i++) {
                if (groups[i].name === groupName) {
                    var feeds = groups[i].feeds;
                    var newFeeds = [];
                    for (var j = 0; j < feeds.length; j++) {
                        if (feeds[j].url !== url) {
                            newFeeds.push(feeds[j]);
                        }
                    }
                    groups[i].feeds = newFeeds;
                    // Delete group if empty
                    if (newFeeds.length === 0) {
                        groups.splice(i, 1);
                    }
                    break;
                }
            }
            FeedGroupsManager.saveGroups(groups);
        },

        deleteGroup: function(name) {
            var groups = FeedGroupsManager.getGroups();
            var newGroups = [];
            for (var i = 0; i < groups.length; i++) {
                if (groups[i].name !== name) {
                    newGroups.push(groups[i]);
                }
            }
            FeedGroupsManager.saveGroups(newGroups);
        },

        addSavedFeedToGroup: function(url, li) {
            // Remove any existing inline input for this item
            var existing = li.querySelector ? li.querySelector(".group-name-row") : null;
            if (existing) {
                li.removeChild(existing);
                return;
            }

            var savedFeeds = SavedFeedsManager.getSavedFeeds();
            var title = url;
            for (var i = 0; i < savedFeeds.length; i++) {
                if (savedFeeds[i].url === url) {
                    title = savedFeeds[i].title || url;
                    break;
                }
            }

            var row = document.createElement("div");
            row.className = "group-name-row";
            row.style.marginTop = "4px";

            var input = document.createElement("input");
            input.type = "text";
            input.placeholder = "Group name";
            input.style.width = "60%";

            var addBtn = document.createElement("button");
            addBtn.className = "secondary";
            setText(addBtn, "Add");

            var cancelBtn = document.createElement("button");
            cancelBtn.className = "secondary";
            setText(cancelBtn, "Cancel");

            addBtn.onclick = function() {
                var groupName = input.value.replace(/^\s+|\s+$/g, "");
                if (groupName) {
                    FeedGroupsManager.addFeedToGroup(groupName, url, title);
                }
                li.removeChild(row);
                return false;
            };

            cancelBtn.onclick = function() {
                li.removeChild(row);
                return false;
            };

            row.appendChild(input);
            row.appendChild(addBtn);
            row.appendChild(cancelBtn);
            li.appendChild(row);
            input.focus();
        },

        loadGroup: function(feeds, name) {
            deactivateToggle("groups-section", "groups-toggle-btn");
            loadCategoryFeeds(feeds, name);
            setArchiveSearchFeeds(feeds);
        },

        toggleFeedGroups: function() {
            var section = document.getElementById("groups-section");
            var btn = document.getElementById("groups-toggle-btn");
            if (section.className.indexOf("hidden") >= 0) {
                closeAllToggleSections();
                removeClass(section, "hidden");
                FeedRenderer.renderFeedGroups();
                if (btn) { addClass(btn, "btn-active"); }
            } else {
                deactivateToggle("groups-section", "groups-toggle-btn");
            }
        }
    };

    window.FeedGroupsManager = FeedGroupsManager;
    window.toggleFeedGroups = function() { FeedGroupsManager.toggleFeedGroups(); };
})();
