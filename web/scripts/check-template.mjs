import fs from 'node:fs'
import path from 'node:path'
import { parse, compileTemplate, compileScript } from '@vue/compiler-sfc'

// Compile every SFC so template and script errors fail fast without a full build.
const root = new URL('../src/', import.meta.url).pathname
const files = []
const walk = dir => fs.readdirSync(dir, { withFileTypes: true }).forEach(e => (e.isDirectory() ? walk(path.join(dir, e.name)) : e.name.endsWith('.vue') && files.push(path.join(dir, e.name))))
walk(root)
let failed = 0
for (const file of files) {
  const { descriptor, errors } = parse(fs.readFileSync(file, 'utf8'), { filename: file })
  const problems = [...errors]
  try {
    const script = compileScript(descriptor, { id: file, inlineTemplate: true })
    if (!script.content) problems.push('empty script')
  } catch (e) {
    problems.push(e.message)
  }
  if (descriptor.template && !descriptor.scriptSetup) problems.push(...compileTemplate({ source: descriptor.template.content, filename: file, id: file }).errors)
  if (problems.length) {
    failed++
    console.error(path.relative(root, file) + ':\n  ' + problems.map(String).join('\n  '))
  }
}
if (failed) process.exit(1)
console.log(files.length + ' Vue components compiled successfully')
