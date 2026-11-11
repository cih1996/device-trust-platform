import fs from 'node:fs'
import { compile } from '@vue/compiler-dom'
for(const name of ['template.html','device-workspace.html'])compile(fs.readFileSync(new URL('../src/'+name,import.meta.url),'utf8'))
console.log('Vue template compiled successfully')
