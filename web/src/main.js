import { createApp, ref, computed } from 'vue/dist/vue.esm-bundler.js'
import './style.css'
import template from './template.html?raw'
import { signalValue } from './signals.js'
import { sceneNames, actionNames, reviewNames, riskReasons, fieldNames, outcomeNames } from './risk-display.js'

createApp({
 setup() {
  const token=ref(''), signedIn=ref(false), apps=ref([]), overview=ref({stats:{events:0,devices:0,review:0},events:[]})
  const selected=ref(''), section=ref('overview'), error=ref(''), busy=ref(false), name=ref(''), packageName=ref(''), newKey=ref(''), query=ref(''), detail=ref(null), device=ref(null)
  const sectionNames={overview:'风险概览',events:'风险事件',devices:'设备画像',apps:'应用管理'}
  const signalNames={manufacturer:'制造商',model:'机型',sdkInt:'系统版本',securityPatch:'安全更新',buildFingerprint:'系统版本标识',debugger:'调试状态',emulatorHint:'模拟环境线索',vpn:'VPN 连接',transport:'网络连接',operator:'网络运营商',androidId:'应用设备标识',expectedPackage:'应用匹配',cell:'基站与信号',wifiRssi:'Wi-Fi 信号',bssid:'无线接入点',location:'位置',installedApps:'已安装应用',bluetoothMac:'蓝牙地址',routerMac:'路由地址',attestation:'硬件证明',rootBeerTestKeys:'测试版系统线索',rootBeerSuBinary:'提权程序线索',rootBeerMagiskBinary:'系统修改工具线索',rootBeerNativeSu:'原生提权程序线索',rootBeerVersion:'采集组件版本'}
  const reasons=riskReasons
  const scenario=ref(''),policyApp=ref(null),policyDraft=ref(null),reviewNote=ref('')
  const securityApp=ref(null),securityDraft=ref(null),signerText=ref(''),resolutionId=ref('')
  const workspaceMode=computed(()=>{const list=apps.value.filter(app=>!selected.value||app.id===selected.value);return list.some(app=>app.policy.mode==='enforce')?'执行模式':'观察模式'})
  const proofName=proof=>!proof?'待验证':(proof.level==='tee'||proof.level==='strongbox')?'硬件密钥已验证':'签名已验证'
  function gateState(event){const execution=overview.value.executions[event.id];if(execution)return execution.status==='succeeded'?'业务已完成':'执行失败';const resolution=overview.value.resolutions[event.id];if(resolution)return resolution.status==='approved'?'已批准':'已拒绝';if(event.mode!=='enforce')return '观察';return event.action==='hold'?'待审核':event.action==='verify'?'待核验':'可执行'}
  const policyFields=[['reviewScore','建议核验分数'],['holdScore','建议暂缓分数'],['registrationAccounts','设备注册账号数 · 10 分钟'],['ipRegistrations','网络注册账号数 · 10 分钟'],['campaignAccounts','活动关联账号数 · 24 小时'],['paymentMethods','账号支付工具数 · 24 小时'],['paymentAccounts','支付工具关联账号数 · 24 小时'],['paymentFailures','支付失败次数 · 1 小时']]
  const events=computed(()=>overview.value.events.filter(e=>(!scenario.value||e.scene===scenario.value)&&[e.accountId,e.type,e.installationId].some(v=>v.includes(query.value))))
  async function api(path,method='GET',body) {
   const response=await fetch('/api/'+path,{method,headers:{Authorization:'Bearer '+token.value,'Content-Type':'application/json'},body:body?JSON.stringify(body):undefined})
   const data=await response.json();if(!response.ok) throw Error(data.error);return data
  }
  async function act(action){error.value='';busy.value=true;try{await action()}catch(e){error.value=e.message}finally{busy.value=false}}
  async function refresh(){apps.value=await api('apps');overview.value=await api('overview?appId='+encodeURIComponent(selected.value))}
  function login(){act(async()=>{await refresh();signedIn.value=true})}
  function reload(){act(refresh)}
  function openPolicy(app){policyApp.value=app;policyDraft.value={...app.policy}}
  function openSecurity(app){securityApp.value=app;securityDraft.value={...app.security};signerText.value=app.security.signingDigests.join('\n')}
  function saveSecurity(){act(async()=>{await api('apps/'+securityApp.value.id+'/security','PUT',{...securityDraft.value,signingDigests:signerText.value.split(/\s+/).filter(Boolean)});securityApp.value=null;await refresh()})}
  function resolve(status){act(async()=>{await api('events/'+detail.value.id+'/resolve','POST',{resolutionId:resolutionId.value,status,note:reviewNote.value});await refresh()})}
  function revokeDevice(){if(window.confirm('停用此设备密钥？'))act(async()=>{await api('device-keys/revoke','POST',{appId:device.value.appId,keyId:device.value.latestReport.proof.keyId,reason:'operator'});device.value=null;await refresh()})}
  function savePolicy(){act(async()=>{await api('apps/'+policyApp.value.id+'/policy','PUT',policyDraft.value);policyApp.value=null;await refresh()})}
  function openEvent(event){detail.value=event;reviewNote.value=overview.value.feedback[event.id]?.note||'';resolutionId.value=crypto.randomUUID()}
  function review(label){act(async()=>{await api('events/'+detail.value.id+'/reviews','POST',{label,note:reviewNote.value});await refresh()})}
  function create(){act(async()=>{const data=await api('apps','POST',{name:name.value,package:packageName.value});newKey.value=data.key;name.value='';packageName.value='';await refresh()})}
  function rotate(app){if(window.confirm('重置「'+app.name+'」的密钥？原密钥将立即失效。'))act(async()=>{const data=await api('apps/'+app.id+'/rotate-key','POST',{});newKey.value=data.key})}
  function logout(){token.value='';signedIn.value=false;newKey.value='';detail.value=null;device.value=null;apps.value=[];overview.value={stats:{events:0,devices:0,review:0},events:[]}}
  const appName=id=>apps.value.find(a=>a.id===id)?.name||id
  const formatTime=value=>new Date(value).toLocaleString('zh-CN',{hour12:false})
  return {token,signedIn,apps,overview,selected,section,error,busy,name,packageName,newKey,query,detail,device,sectionNames,signalNames,signalValue,reasons,events,login,reload,create,rotate,logout,appName,formatTime,sceneNames,actionNames,reviewNames,fieldNames,outcomeNames,scenario,policyApp,policyDraft,policyFields,reviewNote,openPolicy,savePolicy,openEvent,review,securityApp,securityDraft,signerText,workspaceMode,proofName,gateState,openSecurity,saveSecurity,resolve,revokeDevice}
 },
 template
}).mount('#app')
