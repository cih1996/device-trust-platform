import { test } from 'node:test'
import assert from 'node:assert/strict'
import { signalValue } from './signals.js'

test('native failures retain the upstream message, not a clean device result', () => {
  assert.equal(signalValue({status:'error',reason:'java.lang.UnsatisfiedLinkError: missing toolChecker'}), 'java.lang.UnsatisfiedLinkError: missing toolChecker')
  assert.equal(signalValue({status:'available',value:false}), '否')
})
test('unavailable cell power is omitted and sample age remains visible', () => {
  assert.equal(signalValue({status:'available',value:[{technology:'NR',dbm:null,ageMillis:4500}]}), 'NR · 4 秒前')
})
test('cached location includes accuracy, age and mock observation', () => {
  assert.equal(signalValue({status:'available',value:{latitude:22.5,longitude:108.3,accuracyMeters:18,ageMillis:8000,mock:true}}), '22.5, 108.3 · ±18 米 · 8 秒前 · 模拟位置')
})
