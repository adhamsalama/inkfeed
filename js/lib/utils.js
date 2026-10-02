function escapeHtml(text) {
  if (!text) return "";
  var div = document.createElement("div");
  setText(div, text);
  return div.innerHTML;
}

function downloadBlob(blob, filename) {
  if (typeof URL !== "undefined" && URL.createObjectURL) {
    var url = URL.createObjectURL(blob);
    var a = document.createElement("a");
    a.href = url;
    a.download = filename;
    a.style.display = "none";
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
    return true;
  }
  return false;
}

// Helper: hide a section and remove btn-active from its toggle button
function deactivateToggle(sectionId, btnId) {
  var section = document.getElementById(sectionId);
  if (section) { addClass(section, "hidden"); }
  var btn = document.getElementById(btnId);
  if (btn) { removeClass(btn, "btn-active"); }
}

// Helper: close all toggle sections so only one can be open at a time
function closeAllToggleSections() {
  var pairs = [
    ["saved-feeds-section",     "saved-toggle-btn"],
    ["groups-section",          "groups-toggle-btn"],
    ["favorites-section",       "favorites-toggle-btn"],
    ["suggested-feeds-section", "suggested-toggle-btn"],
    ["wikipedia-section",       "wikipedia-toggle-btn"]
  ];
  for (var i = 0; i < pairs.length; i++) {
    deactivateToggle(pairs[i][0], pairs[i][1]);
  }
}

// Helper: add class to element
function addClass(el, className) {
  if (!el) return;
  var classes = el.className.split(" ");
  var found = false;
  for (var i = 0; i < classes.length; i++) {
    if (classes[i] === className) {
      found = true;
      break;
    }
  }
  if (!found) {
    classes.push(className);
  }
  el.className = classes.join(" ").replace(/^\s+|\s+$/g, "");
}

// Helper: remove class from element
function removeClass(el, className) {
  if (!el) return;
  var classes = el.className.split(" ");
  var newClasses = [];
  for (var i = 0; i < classes.length; i++) {
    if (classes[i] !== className && classes[i] !== "") {
      newClasses.push(classes[i]);
    }
  }
  el.className = newClasses.join(" ");
}

// Helper: get text content safely
function getText(el) {
  if (!el) return "";
  return el.textContent || el.innerText || "";
}

// Helper: convert an HTML fragment to plain text without building DOM nodes.
// Parsing via innerHTML would create <img> elements that start downloading
// (and run onerror handlers) even in a detached element. A textarea's content
// is never parsed as markup, so it only decodes entities like &amp;.
function htmlToText(html) {
  if (!html) return "";
  var stripped = String(html)
    .replace(/<(script|style)[^>]*>[\s\S]*?<\/\1\s*>/gi, " ")
    .replace(/<!--[\s\S]*?-->/g, " ")
    .replace(/<[^>]*>/g, " ");
  var textarea = document.createElement("textarea");
  textarea.innerHTML = stripped;
  return textarea.value.replace(/\s+/g, " ").replace(/^\s+|\s+$/g, "");
}

// Helper: set text content safely
function setText(el, text) {
  if (!el) return;
  if (typeof el.textContent !== "undefined") {
    el.textContent = text;
  } else {
    el.innerText = text;
  }
}

// Helper: get element by tag name from parent
function getFirstByTag(parent, tagName) {
  if (!parent) return null;
  var elements = parent.getElementsByTagName(tagName);
  return elements.length > 0 ? elements[0] : null;
}

// Helper: parse URL parameters
function getUrlParam(name) {
  var search = window.location.search;
  if (!search) return null;
  search = search.substring(1);
  var pairs = search.split("&");
  for (var i = 0; i < pairs.length; i++) {
    var pair = pairs[i].split("=");
    if (decodeURIComponent(pair[0]) === name) {
      return pair[1] ? decodeURIComponent(pair[1]) : "";
    }
  }
  return null;
}

// XMLHttpRequest fetch with callback
function fetchUrl(url, callback) {
  try {
    var useProxy = true;
    if (useProxy) {
      // url = "https://cors-anywhere.com/" + url;
      url = AppConfig.CORS_PROXY_URL + url;
    }
    var xhr;
    if (window.XMLHttpRequest) {
      xhr = new XMLHttpRequest();
    } else {
      xhr = new ActiveXObject("Microsoft.XMLHTTP");
    }
    xhr.onreadystatechange = function() {
      try {
        if (xhr.readyState === 4) {
          if (xhr.status === 200) {
            callback(null, xhr.responseText);
          } else {
            callback(new Error("Fetch failed: " + xhr.status), null);
          }
        }
      } catch (e) {
        alert("XHR callback error: " + e.message);
      }
    };
    xhr.open("GET", url, true);
    xhr.send(null);
  } catch (e) {
    alert("fetchUrl error: " + e.message);
    callback(e, null);
  }
}
