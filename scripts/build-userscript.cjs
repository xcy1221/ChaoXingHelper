const fs = require('node:fs');
const path = require('node:path');
const root = path.resolve(__dirname, '..');
const read = file => fs.readFileSync(path.join(root, 'apps/userscript/src', file), 'utf8').trimEnd();
const modules = ['features/video.js', 'features/homework-paste.js', 'entry.js'].map(read);
fs.writeFileSync(path.join(root, 'chaoxing-helper.user.js'),
  read('metadata.txt') + '\n\n(() => {\n\'use strict\';\n\n' + modules.join('\n\n') + '\n})();\n');
console.log('Built chaoxing-helper.user.js');
