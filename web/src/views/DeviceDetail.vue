<script setup>
import { ref, computed, watch } from 'vue'
import { api } from '../api.js'
import { route, setQuery, href, go } from '../router.js'
import { appId, appName, revision, report, openEvent, changed } from '../store.js'
import { fullTime, dateTime, relative, region, gib, levelNames } from '../format.js'
import { reasonName, sourceNames, sceneNames, statusNames } from '../risk-display.js'
import ScoreRing from '../components/ScoreRing.vue'
import RiskScore from '../components/RiskScore.vue'
import LevelTag from '../components/LevelTag.vue'
import LineChart from '../components/LineChart.vue'
import StackedColumns from '../components/StackedColumns.vue'
import ResourcePanel from '../components/ResourcePanel.vue'
import Icon from '../components/Icon.vue'
import AllItems from '../components/AllItems.vue'

const packageNames = { android: '系统框架', 'com.android.settings': '系统设置', 'com.android.systemui': '系统界面', 'com.miui.securitycenter': '小米安全中心', 'com.miui.home': '小米桌面', 'com.xiaomi.market': '小米应用商店', 'com.huawei.hwid': '华为移动服务', 'com.huawei.appmarket': '华为应用市场', 'com.heytap.market': '欢太软件商店', 'com.oplus.safecenter': '系统安全中心', 'com.bbk.appstore': 'vivo 应用商店', 'com.bbk.launcher2': 'vivo 桌面', 'com.hihonor.appmarket': '荣耀应用市场' }
const sensorTypes = { 1: '加速度', 2: '磁场', 3: '方向', 4: '陀螺仪', 5: '光线', 8: '距离', 9: '重力', 10: '线性加速度', 11: '旋转矢量', 14: '未校准磁场', 15: '游戏旋转', 16: '未校准陀螺仪', 17: '显著运动', 18: '步行检测', 19: '计步器', 20: '地磁旋转', 22: '倾斜检测', 27: '设备方向', 29: '静止检测', 30: '运动检测', 35: '未校准加速度' }
const tabs = { risk: '风险分析', events: '业务事件', hardware: '设备参数', apps: '应用与签名', network: '网络与定位', history: '关联与轨迹', items: '全部条目', raw: '原始数据' }

const data = ref(null)
const busy = ref(false)
const tab = computed(() => (tabs[route.query.tab] ? route.query.tab : 'risk'))

async function load() {
  try {
    data.value = await api('devices/' + encodeURIComponent(route.id))
  } catch (e) {
    report(e)
  }
}
watch([() => route.id, revision], load, { immediate: true })

const p = computed(() => data.value?.profile)
const r = computed(() => p.value?.latestReport)
const observed = (report, key) => (report?.signals?.[key]?.status === 'available' ? report.signals[key].value : null)
const sig = key => observed(r.value, key)
const model = computed(() => sig('model') || '未识别机型')
const brand = computed(() => sig('manufacturer') || '品牌未提供')
const java = computed(() => sig('resourceJava'))
const native = computed(() => sig('resourceNative'))
const sensors = computed(() => sig('sensors') || [])
const packages = computed(() => sig('systemPackages') || [])
const application = computed(() => sig('application'))
const network = computed(() => sig('networkSnapshot'))
const gps = computed(() => sig('location'))
const cells = computed(() => sig('cell') || [])
const lan = computed(() => sig('lan'))
const lanProbe = computed(() => sig('lanProbe'))
const lanIp = computed(() => lan.value?.addresses?.[0]?.address || '')
const envFactors = computed(() => (p.value?.factors || []).filter(f => f.source === 'client' || f.source === 'resource'))
const bizFactors = computed(() => (p.value?.factors || []).filter(f => f.source !== 'client' && f.source !== 'resource'))
const completeness = computed(() => {
  const values = Object.values(r.value?.signals || {})
  return { available: values.filter(s => s.status === 'available').length, total: values.length }
})
const trendKeys = [{ key: 'events', label: '业务事件', color: 'var(--accent)' }]

const groups = computed(() => {
  const j = java.value, n = native.value, os = sig('osInfo'), mem = sig('memory'), storage = sig('storage'), screen = sig('display'), battery = sig('battery')
  return [
    { name: '机型与处理器', fields: [['品牌', j?.brand], ['制造商', j?.manufacturer], ['型号', j?.model], ['设备代号', j?.device], ['产品', j?.product], ['主板', j?.board], ['CPU 型号', n?.cpuHardware], ['硬件平台', n?.platform], ['系统架构', n?.machine], ['处理器架构', j?.supportedAbis?.join(' / ')], ['配置核心', n?.configuredCores], ['应用可用核心', j?.availableCores], ['SoC 厂商', j?.socManufacturer], ['SoC 型号', j?.socModel]] },
    { name: '内存与存储', fields: [['运行内存', mem ? gib(mem.totalBytes) : gib(j?.memoryBytes)], ['当前可用内存', gib(mem?.availableBytes)], ['数据分区容量', gib(storage?.totalBytes)], ['分区可用空间', gib(storage?.availableBytes)], ['内存紧张', mem?.lowMemory]] },
    { name: '系统与显示', fields: [['Android 版本', os?.release], ['API 版本', os?.sdk], ['安全更新', os?.patch], ['系统版本', os?.display], ['构建类型', os?.type], ['内核', n?.kernelRelease], ['启动程序', os?.bootloader], ['开机时长', os?.uptimeMillis != null ? Math.floor(os.uptimeMillis / 3600000) + ' 小时' : null], ['显示区域', screen ? screen.widthPixels + ' × ' + screen.heightPixels : null], ['显示密度', screen?.densityDpi != null ? screen.densityDpi + ' dpi' : null], ['字体缩放', screen?.fontScale], ['系统指纹', sig('buildFingerprint')]] },
    runtime.value,
    tooling.value,
    { name: '电源状态', fields: [['当前电量', battery && battery.scale > 0 && battery.level >= 0 ? Math.round((battery.level / battery.scale) * 100) + '%' : null], ['电池温度', battery?.temperatureTenthsC >= 0 ? battery.temperatureTenthsC / 10 + ' ℃' : null], ['电池电压', battery?.voltageMillivolts > 0 ? battery.voltageMillivolts + ' mV' : null], ['供电方式', battery ? { 0: '电池', 1: '交流电', 2: 'USB', 4: '无线充电' }[battery.plugged] || '其他' : null]] },
  ]
})
const listText = v => (Array.isArray(v) ? (v.length ? v.join('、') : '未发现') : null)
const runtime = computed(() => {
  const n = sig('integrityNative'), box = sig('appSandbox'), proxy = sig('httpProxy')
  return { name: '运行环境', fields: [
    ['调试器跟踪', n ? (n.tracerPid > 0 ? '进程 ' + n.tracerPid : '无') : signalStatus('integrityNative')],
    ['Hook 痕迹', listText(n?.hooks)],
    ['Hook 线程', listText(n?.hookThreads)],
    ['模拟器特征', listText(n?.emulator)],
    ['应用可调试', sig('appDebuggable')],
    ['USB 调试', sig('adbEnabled')],
    ['开发者选项', sig('developerOptions')],
    ['HTTP 代理', proxy ? proxy.host + ':' + proxy.port : proxy === false ? '无' : signalStatus('httpProxy')],
    ['已开启无障碍服务', sig('accessibilityServices')],
    ['用户空间', box ? (box.userId === 0 ? '主用户' : '用户 ' + box.userId) : null],
    ['数据目录', box ? box.dataDir + (box.ownDataDir ? '' : '（非本应用目录）') : null],
    ['启动状态', n?.bootState ? Object.entries(n.bootState).map(([k, v]) => k + ' = ' + v).join('\n') || '未读取到' : null],
    ['Root 属性', listText(n?.rootProperties)],
    ['Root 挂载', listText(n?.rootMounts)],
    ['SELinux 域', n?.selinuxContext],
    ['承载网卡', sig('activeInterface')],
    ['网卡', Array.isArray(n?.interfaces) ? n.interfaces.map(i => i.name + ' ' + i.family + ' ' + i.scope).join('\n') : null],
    ['SIM 运营商', sig('simOperator')],
    ['注册网络', sig('operator')],
    ['漫游', sig('networkRoaming')],
  ] }
})
const tooling = computed(() => {
  const n = sig('integrityNative'), apps = sig('riskApps'), a = sig('accessibility'), auto = sig('automation'), hooks = sig('javaHooks'), ports = n?.ports
  const touches = auto?.touches
  return { name: '工具与自动化', fields: [
    ['风险应用', apps ? listText(apps.installed) : signalStatus('riskApps')],
    ['无障碍', a ? (a.enabled ? '已开启' : '未开启') : null],
    ['无障碍服务', a ? listText(a.services.map(s => s.package + (s.system ? '（系统）' : '') + (s.gestures ? ' 可模拟手势' : ''))) : null],
    ['点击来源', touches ? (touches.downs ? touches.downs + ' 次按下，' + touches.synthetic + ' 次为模拟点击' : '未记录') : null],
    ['Monkey 模式', auto?.monkey],
    ['Xposed 类', listText(hooks?.classes)],
    ['注入代码', listText(n?.injectedCode)],
    ['本地监听端口', ports ? (ports.count ? ports.loopback.join('、') + (ports.count > ports.loopback.length ? ' 等 ' + ports.count + ' 个' : '') : '无') : null],
    ['Frida 端口', listText(ports?.frida)],
    ['局域网可访问端口', listText(ports?.exposed)],
    ['端口扫描耗时', ports ? ports.scanMillis + ' ms' : null],
  ] }
})
const networkHistory = computed(() => {
  const rows = []
  for (const rep of data.value?.reports || []) {
    const last = rows[rows.length - 1]
    const transport = observed(rep, 'transport')
    if (!last || last.ip !== rep.observedIp || last.transport !== transport) rows.push({ time: rep.receivedAt, ip: rep.observedIp, transport, region: rep.resources?.ip })
  }
  return rows
})

function display(value) {
  if (value === null || value === undefined || value === '') return '未提供'
  if (typeof value === 'boolean') return value ? '是' : '否'
  if (typeof value === 'number' && !Number.isInteger(value)) return String(Number(value.toPrecision(6)))
  return String(value)
}
const purpose = v => ({ device_init: '首次登记', device_refresh: '启动更新', register: '注册场景', reward: '营销场景', payment: '支付场景' }[v] || '业务上报')
const proof = v => (!v ? '未签名' : v.level === 'tee' || v.level === 'strongbox' ? '硬件证明通过' : '签名已验证')
const attestIssue = { boot_unlocked: 'Bootloader 未锁定', app_mismatch: '包名或签名不符', revoked: '证明证书已吊销', unverified: '未通过', keystore_failed: '设备无法生成证明' }
const attestation = v => (!v ? '未签名' : v.level === 'tee' || v.level === 'strongbox' ? '通过（' + v.level.toUpperCase() + '）' : attestIssue[v.attestationIssue] || '未提交证明')
const signerCheck = computed(() => {
  const s = r.value?.signals?.expectedSigner
  if (!s) return '未配置应用证书'
  return s.status !== 'available' ? '未上报' : s.value ? '一致' : '不一致'
})
const linkMethod = v => ({ android_id: '同一开发者设备标识', boot_session: '同一次开机', new: '首次登记' }[v] || v)
const transport = v => ({ wifi: 'Wi-Fi', cellular: '移动网络', ethernet: '有线网络', other: '其他网络' }[v] || '暂不可用')
const signalStatus = key => {
  const s = r.value?.signals?.[key]
  return s ? { available: '已采集', disabled: '未开启', permission_denied: '未授权', unavailable: '暂不可用', error: '采集出错' }[s.status] || s.status : '未上报'
}
const signalReason = key => r.value?.signals?.[key]?.reason || signalStatus(key)
const keyState = computed(() => ({ revoked: { text: '密钥已停用', cls: 'bad' }, active: { text: '密钥有效', cls: 'good' }, legacy: { text: '未签名上报', cls: '' } }[p.value?.keyStatus] || { text: p.value?.keyStatus, cls: '' }))

function exportRecord() {
  const blob = new Blob([JSON.stringify(data.value, null, 2)], { type: 'application/json' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = 'DeviceTrust-' + (p.value.deviceId || p.value.id) + '.json'
  a.click()
  URL.revokeObjectURL(url)
}
async function revoke() {
  if (!window.confirm('停用此设备的签名密钥？停用后该设备的后续上报会被拒绝，需要重新登记。')) return
  busy.value = true
  try {
    await api('device-keys/revoke', 'POST', { appId: p.value.appId, keyId: r.value.proof.keyId, reason: 'operator' })
    changed()
  } catch (e) {
    report(e)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div v-if="p">
    <div class="crumbs"><a :href="href('devices', '', { app: appId })"><Icon name="back" />设备</a><span>/</span><span>{{ model }}</span></div>

    <section class="card identity" style="margin-bottom:16px">
      <div class="monogram">{{ brand.slice(0, 1) }}</div>
      <div style="min-width:0">
        <h1>{{ model }}</h1>
        <div class="meta">
          <span>{{ brand }} · {{ appName(p.appId) }}</span>
          <span class="mono">{{ p.deviceId || '未登记设备 ID' }}</span>
          <span v-if="lanIp" class="mono">内网 {{ lanIp }}</span>
        </div>
        <div class="meta">
          <span>首次出现 {{ fullTime(p.firstSeen) }}</span>
          <span :title="fullTime(p.lastSeen)">最近活跃 {{ relative(p.lastSeen) }}</span>
          <span :class="['tag', keyState.cls]">{{ keyState.text }}</span>
        </div>
      </div>
      <div class="ring-wrap">
        <ScoreRing :score="p.riskScore" />
        <div class="ring-text">
          <LevelTag :level="p.riskLevel" />
          <small>环境评分 {{ p.envScore }} · 业务评分 {{ p.businessScore }}</small>
          <span class="row-gap" style="margin-top:4px">
            <button class="small" @click="exportRecord"><Icon name="download" />导出</button>
            <button v-if="r.proof" class="small danger" :disabled="busy || p.keyStatus === 'revoked'" @click="revoke">停用密钥</button>
          </span>
        </div>
      </div>
    </section>

    <div class="grid g-kpi" style="margin-bottom:16px">
      <div class="card tile"><div class="label">关联账号</div><div class="value">{{ p.accountCount }}</div></div>
      <div class="card tile"><div class="label">业务事件</div><div class="value">{{ p.eventCount }}</div></div>
      <div class="card tile"><div class="label">7 天高风险事件</div><div class="value">{{ p.highEvents }}</div></div>
      <div class="card tile"><div class="label">设备上报</div><div class="value">{{ p.reportCount }}</div></div>
      <div class="card tile"><div class="label">使用过的 IP</div><div class="value">{{ data.ips.length }}</div></div>
      <div class="card tile"><div class="label">已采集项目</div><div class="value">{{ completeness.available }}<small>/ {{ completeness.total }}</small></div></div>
    </div>

    <nav class="tabs" aria-label="设备档案分区">
      <button v-for="(label, key) in tabs" :key="key" :class="{ on: tab === key }" @click="setQuery({ tab: key === 'risk' ? '' : key })">{{ label }}<span v-if="key === 'events'" class="muted"> {{ data.events.length }}</span></button>
    </nav>

    <template v-if="tab === 'risk'">
      <div class="grid g-2" style="margin-bottom:16px">
        <section class="card">
          <div class="card-head"><div><h2>评分构成</h2><span class="sub">设备评分 {{ p.riskScore }} = max(环境 {{ p.envScore }}, 业务 {{ p.businessScore }})</span></div></div>
          <div class="card-body">
            <div class="section-title">环境线索 <small>{{ envFactors.length ? '最近一次上报' : '' }}</small></div>
            <div v-for="(f, i) in envFactors" :key="'e' + i" class="factor">
              <span>{{ reasonName(f.code) }}</span><span class="pts">+{{ f.points }}</span>
              <small>{{ sourceNames[f.source] }}<template v-if="f.threshold"> · 观测 {{ f.observed }} / 阈值 {{ f.threshold }}</template></small>
            </div>
            <p v-if="!envFactors.length" class="muted" style="margin:0 0 12px">最近一次上报没有命中环境线索</p>
            <div class="section-title" style="margin-top:12px">业务线索 <small>7 天内最高分事件</small></div>
            <div v-for="(f, i) in bizFactors" :key="'b' + i" class="factor">
              <span>{{ reasonName(f.code) }}</span><span class="pts">+{{ f.points }}</span>
              <small>{{ sourceNames[f.source] || f.source }}<template v-if="f.threshold"> · 观测 {{ f.observed }} / 阈值 {{ f.threshold }}</template><template v-if="f.windowSeconds"> · {{ f.windowSeconds >= 3600 ? f.windowSeconds / 3600 + ' 小时' : f.windowSeconds / 60 + ' 分钟' }}窗口</template></small>
            </div>
            <p v-if="!bizFactors.length" class="muted" style="margin:0">7 天内没有命中业务规则</p>
          </div>
        </section>
        <section class="card">
          <div class="card-head"><div><h2>环境验真</h2><span class="sub">Java / NDK 参数、系统签名与网络位置比对</span></div></div>
          <div class="card-body"><ResourcePanel :assessment="r.resources" /></div>
        </section>
      </div>
      <div class="grid g-2">
        <section class="card">
          <div class="card-head"><div><h2>近 30 天最高风险分</h2><span class="sub">每天业务事件的最高分，无事件留空</span></div></div>
          <div class="card-body"><LineChart :points="data.trend" field="maxScore" empty-as="events" label="最高风险分" :max="100" step="day" color="var(--high)" /></div>
        </section>
        <section class="card">
          <div class="card-head"><div><h2>近 30 天业务事件</h2><span class="sub">每天事件数</span></div></div>
          <div class="card-body"><StackedColumns :points="data.trend" :keys="trendKeys" step="day" :height="160" /></div>
        </section>
      </div>
    </template>

    <section v-if="tab === 'events'" class="card">
      <div class="card-head">
        <div><h2>业务事件</h2><span class="sub">最近 50 条</span></div>
        <a :href="href('events', '', { app: appId, profileId: p.id })">在事件列表中查看 →</a>
      </div>
      <div class="table-wrap">
        <table>
          <thead><tr><th>时间</th><th>账号 / 场景</th><th>风险评分</th><th>命中线索</th><th>IP</th><th>处置</th></tr></thead>
          <tbody>
            <tr v-for="e in data.events" :key="e.id" class="click" @click="openEvent(e.id)">
              <td>{{ dateTime(e.receivedAt) }}</td>
              <td>{{ e.accountId }}<span class="sub">{{ sceneNames[e.scene] || e.type }}</span></td>
              <td><RiskScore :score="e.score" /></td>
              <td><span class="row-gap"><span v-for="h in e.hits.slice(0, 3)" :key="h.code" class="tag">{{ reasonName(h.code) }}</span><span v-if="!e.hits.length" class="muted">—</span></span></td>
              <td class="mono">{{ e.clientIp || '—' }}</td>
              <td><span :class="['tag', { warn: e.status === 'pending' }]">{{ statusNames[e.status] }}</span></td>
            </tr>
          </tbody>
        </table>
        <div v-if="!data.events.length" class="empty">该设备还没有业务事件</div>
      </div>
    </section>

    <template v-if="tab === 'hardware'">
      <div class="grid g-2" style="margin-bottom:16px">
        <section class="card" v-for="g in groups" :key="g.name">
          <div class="card-head"><h2>{{ g.name }}</h2></div>
          <div class="card-body"><dl class="kv"><template v-for="[k, v] in g.fields" :key="k"><dt>{{ k }}</dt><dd>{{ display(v) }}</dd></template></dl></div>
        </section>
      </div>
      <section class="card">
        <div class="card-head"><h2>传感器</h2><span class="sub">{{ sensors.length }} 项</span></div>
        <div class="table-wrap" v-if="sensors.length">
          <table>
            <thead><tr><th>名称</th><th>厂商</th><th>类型</th><th>版本</th><th>量程</th><th>精度</th></tr></thead>
            <tbody><tr v-for="(s, i) in sensors" :key="i"><td>{{ s.name }}</td><td>{{ s.vendor }}</td><td>{{ sensorTypes[s.type] || '厂商扩展' }}</td><td>{{ display(s.version) }}</td><td>{{ display(s.maxRange) }}</td><td>{{ display(s.resolution) }}</td></tr></tbody>
          </table>
        </div>
        <div v-else class="card-body muted">{{ signalReason('sensors') }}</div>
      </section>
    </template>

    <template v-if="tab === 'apps'">
      <section class="card" style="margin-bottom:16px">
        <div class="card-head"><h2>接入应用</h2></div>
        <div class="card-body">
          <dl v-if="application" class="kv">
            <dt>应用包名</dt><dd>{{ application.package }}</dd>
            <dt>应用版本</dt><dd>{{ application.versionName }} · {{ application.versionCode }}</dd>
            <dt>安装来源</dt><dd>{{ application.installer || '来源未提供' }}</dd>
            <dt>签名 SHA-256</dt><dd><code v-for="d in application.signers" :key="d" class="digest">{{ d }}</code></dd>
            <dt>签名校验</dt><dd>{{ signerCheck }}</dd>
            <dt>硬件证明</dt><dd :title="r.proof?.attestationError">{{ attestation(r.proof) }}</dd>
          </dl>
          <p v-else class="muted" style="margin:0">本次上报没有应用版本与签名信息</p>
        </div>
      </section>
      <section class="card">
        <div class="card-head"><h2>指定系统应用</h2><span class="sub">{{ signalStatus('systemPackages') }}</span></div>
        <div class="table-wrap">
          <table>
            <thead><tr><th>应用</th><th>状态</th><th>版本</th><th>类型</th><th>签名证书</th></tr></thead>
            <tbody>
              <tr v-for="item in packages" :key="item.package">
                <td><strong>{{ packageNames[item.package] || item.package }}</strong><span class="sub">{{ item.package }}</span></td>
                <td>{{ { visible: '已采集', not_visible_or_absent: '未检出', permission_denied: '未授权' }[item.status] || item.status }}</td>
                <td>{{ display(item.version) }}</td>
                <td>{{ item.status === 'visible' ? (item.system ? '系统应用' : '普通应用') : '—' }}</td>
                <td><details v-if="item.signers?.length"><summary>{{ item.signers.length }} 个证书</summary><code v-for="d in item.signers" :key="d" class="digest">{{ d }}</code></details><span v-else class="muted">未提供</span></td>
              </tr>
            </tbody>
          </table>
          <div v-if="!packages.length" class="empty">{{ signalReason('systemPackages') }}</div>
        </div>
      </section>
    </template>

    <template v-if="tab === 'network'">
      <div class="grid g-2" style="margin-bottom:16px">
        <section class="card">
          <div class="card-head"><h2>网络环境</h2></div>
          <div class="card-body">
            <dl class="kv">
              <dt>服务端观测 IP</dt><dd class="mono">{{ r.observedIp }}</dd>
              <dt>IP 归属</dt><dd>{{ region(r.resources?.ip) || '未匹配' }}</dd>
              <dt>网络服务商</dt><dd>{{ r.resources?.ip?.isp || '未匹配' }}</dd>
              <dt>上网方式</dt><dd>{{ transport(sig('transport')) }}</dd>
              <dt>网络已验证</dt><dd>{{ display(network?.validated) }}</dd>
              <dt>VPN 连接</dt><dd>{{ display(network?.vpn ?? sig('vpn')) }}</dd>
              <dt>移动网络运营号</dt><dd>{{ display(sig('operator')) }}</dd>
              <dt>Wi-Fi 信号</dt><dd>{{ sig('wifiRssi') != null ? sig('wifiRssi') + ' dBm' : signalStatus('wifiRssi') }}</dd>
              <dt>Wi-Fi 接入点</dt><dd>{{ display(sig('bssid')) }}</dd>
            </dl>
          </div>
        </section>
        <section class="card">
          <div class="card-head"><h2>定位快照</h2><span class="tag">{{ signalStatus('location') }}</span></div>
          <div class="card-body">
            <dl v-if="gps" class="kv">
              <dt>经纬度</dt><dd>{{ gps.latitude }}, {{ gps.longitude }}</dd>
              <dt>精度</dt><dd>{{ gps.accuracyMeters }} 米</dd>
              <dt>样本年龄</dt><dd>{{ Math.floor(gps.ageMillis / 1000) }} 秒</dd>
              <dt>坐标系</dt><dd>{{ gps.coordinateSystem }}</dd>
              <dt>提供方</dt><dd>{{ gps.provider }}</dd>
              <dt>模拟标记</dt><dd>{{ display(gps.mock) }}</dd>
            </dl>
            <p v-else class="muted" style="margin:0">{{ signalReason('location') }}</p>
          </div>
        </section>
      </div>
      <div class="grid g-2" style="margin-bottom:16px">
        <section class="card">
          <div class="card-head"><h2>局域网</h2><span class="tag">{{ signalStatus('lan') }}</span></div>
          <div class="card-body">
            <dl v-if="lan" class="kv">
              <dt>内网地址</dt><dd class="mono">{{ lan.addresses.map(a => a.address + '/' + a.prefix).join('\n') || '未分配' }}</dd>
              <dt>网关</dt><dd class="mono">{{ display(lan.gateway) }}</dd>
              <dt>DNS</dt><dd class="mono">{{ lan.dns.join('、') || '未提供' }}</dd>
              <dt>DHCP 服务器</dt><dd class="mono">{{ display(lan.dhcpServer) }}</dd>
              <dt>网卡</dt><dd class="mono">{{ lan.interface }} · 系统标记为{{ transport(lan.transport) }}</dd>
              <dt>同网探测</dt><dd>{{ lanProbe ? lanProbe.targets + ' 台，可达 ' + lanProbe.reachable.length + ' 台，开放 ADB ' + lanProbe.adb.length + ' 台，网关' + (lanProbe.gateway == null ? '未探测' : lanProbe.gateway ? '可达' : '不可达') + '，' + lanProbe.millis + ' ms' : signalStatus('lanProbe') }}</dd>
            </dl>
            <p v-else class="muted" style="margin:0">{{ signalReason('lan') }}</p>
          </div>
        </section>
        <section class="card">
          <div class="card-head"><h2>同局域网设备</h2><span class="sub">24 小时内 {{ data.lan.length }} 台</span></div>
          <div class="table-wrap">
            <table v-if="data.lan.length">
              <thead><tr><th>机型</th><th>应用</th><th>内网 IP</th><th>最近上报</th></tr></thead>
              <tbody><tr v-for="d in data.lan" :key="d.id" class="click" @click="go('devices', d.id, { app: d.appId })"><td>{{ d.model || '未识别机型' }}</td><td>{{ appName(d.appId) }}</td><td class="mono">{{ d.lanIp || '—' }}</td><td :title="fullTime(d.lastSeen)">{{ relative(d.lastSeen) }}</td></tr></tbody>
            </table>
            <div v-else class="empty">没有其他设备与它处于同一局域网</div>
          </div>
        </section>
      </div>
      <section class="card">
        <div class="card-head"><h2>基站信息</h2><span class="tag">{{ signalStatus('cell') }}</span></div>
        <div class="table-wrap" v-if="cells.length">
          <table>
            <thead><tr><th>制式</th><th>MCC / MNC</th><th>区域码</th><th>Cell ID</th><th>驻网</th><th>信号</th><th>样本年龄</th></tr></thead>
            <tbody><tr v-for="(c, i) in cells" :key="i"><td>{{ c.technology }}</td><td>{{ c.mcc }} / {{ c.mnc }}</td><td>{{ display(c.areaCode) }}</td><td>{{ display(c.cellId) }}</td><td>{{ display(c.registered) }}</td><td>{{ c.dbm != null ? c.dbm + ' dBm' : '未提供' }}</td><td>{{ c.ageMillis != null ? Math.floor(c.ageMillis / 1000) + ' 秒' : '未提供' }}</td></tr></tbody>
          </table>
        </div>
        <div v-else class="card-body muted">{{ signalReason('cell') }}</div>
      </section>
    </template>

    <template v-if="tab === 'history'">
      <section class="card" style="margin-bottom:16px">
        <div class="card-head"><h2>同一手机的其他应用</h2><span class="sub">{{ data.linked.length }} 个</span></div>
        <div class="table-wrap">
          <table v-if="data.linked.length">
            <thead><tr><th>应用</th><th>设备 ID</th><th>关联依据</th><th class="right">风险分</th><th>最近活跃</th></tr></thead>
            <tbody><tr v-for="d in data.linked" :key="d.id" class="click" @click="go('devices', d.id, { app: appId })"><td>{{ d.appName || d.appId }}</td><td class="mono">{{ d.deviceId || '未登记' }}</td><td>{{ linkMethod(d.method) }}</td><td class="right"><LevelTag :level="d.riskLevel" /> <span class="num">{{ d.riskScore }}</span></td><td :title="fullTime(d.lastSeen)">{{ relative(d.lastSeen) }}</td></tr></tbody>
          </table>
          <div v-else class="empty">没有在其他应用中发现这台手机</div>
        </div>
      </section>
      <div class="grid g-2" style="margin-bottom:16px">
        <section class="card">
          <div class="card-head"><h2>关联账号</h2><span class="sub">{{ data.accounts.length }} 个</span></div>
          <div class="table-wrap">
            <table v-if="data.accounts.length">
              <thead><tr><th>账号</th><th class="right">事件数</th><th>最近出现</th></tr></thead>
              <tbody><tr v-for="a in data.accounts" :key="a.accountId" class="click" @click="setQuery({ tab: 'events' })"><td>{{ a.accountId }}</td><td class="right num">{{ a.events }}</td><td :title="fullTime(a.lastSeen)">{{ relative(a.lastSeen) }}</td></tr></tbody>
            </table>
            <div v-else class="empty">尚未关联业务账号</div>
          </div>
        </section>
        <section class="card">
          <div class="card-head"><h2>使用过的 IP</h2><span class="sub">设备上报与业务请求</span></div>
          <div class="table-wrap">
            <table v-if="data.ips.length">
              <thead><tr><th>IP</th><th>来源</th><th>归属</th><th class="right">次数</th><th>最近</th></tr></thead>
              <tbody><tr v-for="ip in data.ips" :key="ip.source + ip.ip"><td class="mono">{{ ip.ip }}</td><td><span class="tag">{{ ip.source === 'device' ? '设备上报' : '业务请求' }}</span></td><td>{{ region(ip.region) || '未匹配' }}</td><td class="right num">{{ ip.count }}</td><td>{{ relative(ip.lastSeen) }}</td></tr></tbody>
            </table>
            <div v-else class="empty">没有 IP 记录</div>
          </div>
        </section>
      </div>
      <div class="grid g-2">
        <section class="card">
          <div class="card-head"><h2>最近上报</h2><span class="sub">最近 20 次</span></div>
          <div class="card-body timeline">
            <div v-for="rep in data.reports" :key="rep.id" class="item"><span class="pin"></span><div><strong>{{ purpose(rep.purpose) }}</strong> <small>{{ fullTime(rep.receivedAt) }}</small><div class="muted" style="font-size:13px">{{ rep.observedIp }} · {{ proof(rep.proof) }}</div></div></div>
          </div>
        </section>
        <section class="card">
          <div class="card-head"><h2>网络变化</h2></div>
          <div class="card-body timeline">
            <div v-for="(n, i) in networkHistory" :key="i" class="item"><span class="pin"></span><div><strong class="mono">{{ n.ip }}</strong> <small>{{ fullTime(n.time) }}</small><div class="muted" style="font-size:13px">{{ transport(n.transport) }} · {{ region(n.region) || '归属未提供' }} {{ n.region?.isp }}</div></div></div>
          </div>
        </section>
      </div>
    </template>

    <AllItems v-if="tab === 'items'" :report="r" :factors="p.factors || []" />

    <section v-if="tab === 'raw'" class="card">
      <div class="card-head"><h2>最近一次上报原始数据</h2><button class="small" @click="exportRecord"><Icon name="download" />导出完整档案</button></div>
      <div class="card-body"><pre class="raw">{{ JSON.stringify(r, null, 2) }}</pre></div>
    </section>
  </div>
  <div v-else class="empty">加载设备档案…</div>
</template>
