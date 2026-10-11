function isHomeworkPage(path) {
  return /^\/mooc-ans\/mooc2\/work\/dowork\/?$/i.test(path);
}

const currentPath = location.pathname;
const isVideoPage = /^\/(?:mooc-ans\/)?mycourse\/studentstudy(?:$|[/.])/.test(currentPath) ||
  /^\/(?:mooc-ans\/)?knowledge\/cards(?:$|[/.])/.test(currentPath) ||
  /^\/ananas\/modules\/video\//.test(currentPath);
if (isVideoPage) installVideoFeature();
else if (isHomeworkPage(currentPath)) installHomeworkPaste();
