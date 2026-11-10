import fs from 'node:fs'
import { compile } from '@vue/compiler-dom'
compile(fs.readFileSync(new URL('../src/template.html',import.meta.url),'utf8'))
console.log('Vue template compiled successfully')
