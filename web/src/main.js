import { createApp, ref, computed } from 'vue/dist/vue.esm-bundler.js'
import './style.css'

createApp({
 setup() {
  const token=ref(''), signedIn=ref(false), apps=ref([]), overview=ref({stats:{events:0,devices:0,review:0},events:[]})
  const selected=ref(''), section=ref('overview'), error=ref(''), busy=ref(false), name=ref(''), packageName=ref(''), newKey=ref(''), query=ref(''), detail=ref(null), device=ref(null)
  const sectionNames={overview:'风险概览',events:'风险事件',devices:'设备画像',apps:'应用管理'}
  const signalNames={manufacturer:'制造商',model:'机型',sdkInt:'系统版本',securityPatch:'安全更新',buildFingerprint:'系统版本标识',debugger:'调试状态',emulatorHint:'模拟环境线索',vpn:'VPN 连接',transport:'网络连接',operator:'网络运营商',androidId:'应用设备标识',expectedPackage:'应用匹配',cell:'基站',cellSignal:'蜂窝信号',bssid:'无线接入点',location:'位置',installedApps:'已安装应用',bluetoothMac:'蓝牙地址',routerMac:'路由地址',attestation:'硬件证明'}
  const statusNames={permission_denied:'未授权',unavailable:'暂不可用',disabled:'未开启'}
  const reasons={debugger:'正在调试',emulator_hint:'模拟环境线索',vpn:'VPN 连接',package_mismatch:'应用不匹配',multiple_accounts_24h:'多账号关联',frequent_events_1m:'操作频繁'}
  const events=computed(()=>overview.value.events.filter(e=>[e.accountId,e.type,e.installationId].some(v=>v.includes(query.value))))
  async function api(path,method='GET',body) {
   const response=await fetch('/api/'+path,{method,headers:{Authorization:'Bearer '+token.value,'Content-Type':'application/json'},body:body?JSON.stringify(body):undefined})
   const data=await response.json();if(!response.ok) throw Error(data.error);return data
  }
  async function act(action){error.value='';busy.value=true;try{await action()}catch(e){error.value=e.message}finally{busy.value=false}}
  async function refresh(){apps.value=await api('apps');overview.value=await api('overview?appId='+encodeURIComponent(selected.value))}
  function login(){act(async()=>{await refresh();signedIn.value=true})}
  function reload(){act(refresh)}
  function create(){act(async()=>{const data=await api('apps','POST',{name:name.value,package:packageName.value});newKey.value=data.key;name.value='';packageName.value='';await refresh()})}
  function rotate(app){if(window.confirm('重置「'+app.name+'」的密钥？原密钥将立即失效。'))act(async()=>{const data=await api('apps/'+app.id+'/rotate-key','POST',{});newKey.value=data.key})}
  function logout(){token.value='';signedIn.value=false;newKey.value='';detail.value=null;device.value=null;apps.value=[];overview.value={stats:{events:0,devices:0,review:0},events:[]}}
  const appName=id=>apps.value.find(a=>a.id===id)?.name||id
  const formatTime=value=>new Date(value).toLocaleString('zh-CN',{hour12:false})
  const signalValue=s=>s.status==='available'?(s.value===true?'是':s.value===false?'否':String(s.value)):statusNames[s.status]
  return {token,signedIn,apps,overview,selected,section,error,busy,name,packageName,newKey,query,detail,device,sectionNames,signalNames,signalValue,reasons,events,login,reload,create,rotate,logout,appName,formatTime}
 },
 template:`
 <div v-if="!signedIn" class="login"><form @submit.prevent="login"><div class="mark">衡</div><h1>知衡</h1><p>设备风控工作台</p><label for="token">管理口令</label><input id="token" v-model="token" type="password" required autocomplete="current-password"><button :disabled="busy">进入工作台 →</button><p v-if="error" class="error" role="alert">{{error}}</p></form></div>
 <div v-else class="shell">
  <aside><a class="brand" href="/"> <span class="mark">衡</span> 知衡</a><span class="caption">工作空间</span><button :class="{active:section==='overview'}" @click="section='overview'" title="风险概览">◫ <span>风险概览</span></button><button :class="{active:section==='events'}" @click="section='events'" title="风险事件">☷ <span>风险事件</span></button><button :class="{active:section==='devices'}" @click="section='devices'" title="设备画像">▣ <span>设备画像</span></button><button :class="{active:section==='apps'}" @click="section='apps'" title="应用管理">◇ <span>应用管理</span></button><div class="aside-bottom"><span class="dot"></span> 本地工作台<button @click="logout">退出登录 ↗</button></div></aside>
  <main><header><span>{{sectionNames[section]}}</span><div class="toolbar"><select v-model="selected" @change="reload" aria-label="选择应用"><option value="">全部应用</option><option v-for="app in apps" :value="app.id">{{app.name}}</option></select><button class="quiet" @click="reload" :disabled="busy">↻ 刷新</button></div></header>
   <div v-if="error" class="error banner" role="alert">{{error}}</div>
   <div class="content"><div class="heading"><div><span class="eyebrow">DEVICE TRUST</span><h1>{{section==='apps'?'管理你的应用':section==='devices'?'查看设备近况':section==='events'?'每一次风险，都有迹可循':'掌握设备与业务动态'}}</h1><p>{{section==='apps'?'连接应用，管理接入凭据':section==='devices'?'设备信息与账号关联':section==='events'?'查看近期事件与风险线索':'过去 24 小时'}}</p></div><span class="badge">试运行</span></div>
   <template v-if="section==='apps'">
    <section class="panel"><h2>创建应用</h2><form class="create" @submit.prevent="create"><input v-model="name" placeholder="应用名称" aria-label="应用名称" required><input v-model="packageName" placeholder="应用包名" aria-label="应用包名" required><button :disabled="busy">＋ 创建应用</button></form></section>
    <section class="panel" v-if="newKey"><div class="panel-title"><h2>保存应用密钥</h2><button class="quiet" @click="newKey=''">关闭 ×</button></div><p>仅展示一次，请保存到业务服务端。</p><code class="secret">{{newKey}}</code></section>
    <section class="app-grid"><article class="panel app-card" v-for="app in apps"><div class="app-icon">{{app.name.slice(0,1)}}</div><h2>{{app.name}}</h2><p>{{app.package}}</p><label>应用 ID</label><code>{{app.id}}</code><button class="quiet" @click="rotate(app)" :disabled="busy">重置密钥 ↗</button></article></section><div v-if="!apps.length" class="empty">创建第一个应用，开始接入</div>
   </template>
   <section class="panel" v-else-if="section==='devices'"><h2>近期设备</h2><div class="table-scroll"><table><thead><tr><th>设备</th><th>应用</th><th>关联账号</th><th>业务事件</th><th>最近出现</th><th></th></tr></thead><tbody><tr v-for="p in overview.profiles"><td>{{p.installationId}}</td><td>{{appName(p.appId)}}</td><td>{{p.accountCount}}</td><td>{{p.eventCount}}</td><td>{{formatTime(p.lastSeen)}}</td><td><button class="quiet" @click="device=p">详情 ↗</button></td></tr></tbody></table></div><div v-if="!overview.profiles.length" class="empty">暂无设备</div></section>
   <template v-else>
    <div class="stats" v-if="section==='overview'"><article><span>业务事件</span><strong>{{overview.stats.events.toLocaleString()}}</strong><small>过去 24 小时</small></article><article><span>活跃设备</span><strong>{{overview.stats.devices.toLocaleString()}}</strong><small>按应用统计的安装实例</small></article><article class="risk"><span>待关注事件</span><strong>{{overview.stats.review.toLocaleString()}}</strong><small>建议进一步核验</small></article></div>
    <section class="panel event-panel"><div class="panel-title"><h2>近期事件 <span class="count">{{events.length}}</span></h2><input v-model="query" placeholder="搜索账号、事件或设备" aria-label="搜索事件"></div><div class="table-scroll"><table><thead><tr><th>发生时间</th><th>应用 / 事件</th><th>账号</th><th>风险分</th><th>处理建议</th><th></th></tr></thead><tbody><tr v-for="e in events" :key="e.id"><td>{{formatTime(e.receivedAt)}}</td><td>{{appName(e.appId)}}<small>{{e.type}}</small></td><td>{{e.accountId}}</td><td><span :class="['score',e.decision]">{{e.score}}</span></td><td><span :class="['pill',e.decision]">{{e.decision==='review'?'关注':'观察'}}</span></td><td><button class="quiet" @click="detail=e">详情 ↗</button></td></tr></tbody></table></div><div v-if="!events.length" class="empty"><div class="empty-icon">◫</div><h3>暂无事件</h3><p>应用接入后，业务动态将在这里呈现</p></div></section>
   </template></div>
  </main>
  <div v-if="device" class="overlay" @click.self="device=null"><section class="drawer"><button class="close quiet" @click="device=null" aria-label="关闭画像">×</button><span class="eyebrow">DEVICE PROFILE</span><h2>设备画像</h2><dl><dt>应用</dt><dd>{{appName(device.appId)}}</dd><dt>设备</dt><dd>{{device.installationId}}</dd><dt>首次出现</dt><dd>{{formatTime(device.firstSeen)}}</dd><dt>最近出现</dt><dd>{{formatTime(device.lastSeen)}}</dd><dt>设备上报</dt><dd>{{device.reportCount}} 次</dd><dt>关联账号</dt><dd>{{device.accountCount}} 个</dd></dl><h3>最近设备信息</h3><dl><template v-for="(s,key) in device.latestReport.signals"><dt>{{signalNames[key]||key}}</dt><dd>{{signalValue(s)}}</dd></template></dl></section></div>
  <div v-if="detail" class="overlay" @click.self="detail=null"><section class="drawer"><button class="close quiet" @click="detail=null" aria-label="关闭详情">×</button><span class="eyebrow">EVENT DETAILS</span><h2>事件详情</h2><strong class="detail-score">{{detail.score}}<small> 风险分</small></strong><dl><dt>应用</dt><dd>{{appName(detail.appId)}}</dd><dt>账号</dt><dd>{{detail.accountId}}</dd><dt>事件</dt><dd>{{detail.type}}</dd><dt>设备</dt><dd>{{detail.installationId}}</dd><dt>业务 IP</dt><dd>{{detail.clientIp||'未提供'}}</dd><dt>时间</dt><dd>{{formatTime(detail.receivedAt)}}</dd></dl><h3>风险线索</h3><p v-if="!detail.reasons.length">暂未命中规则</p><p v-for="reason in detail.reasons" class="reason">{{reasons[reason]||reason}}</p></section></div>
 </div>`
}).mount('#app')
